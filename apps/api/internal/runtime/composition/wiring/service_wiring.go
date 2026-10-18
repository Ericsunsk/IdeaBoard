package wiring

import (
	"context"
	"encoding/base64"
	"fmt"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	billingrecordsapplication "ideaboard/api/internal/billingrecords/application"
	"ideaboard/api/internal/credits"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users/application/administration"
	"ideaboard/api/internal/identity/users/application/lifecycle"
	"ideaboard/api/internal/identity/users/application/provisioning"
	identitycreditsadapter "ideaboard/api/internal/identity/users/infrastructure/credits"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/payments/infrastructure/events"
	"ideaboard/api/internal/payments/infrastructure/stripe"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/rewards"
	rewardscredits "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/runtime/adapters/billingregistry"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	usageapplication "ideaboard/api/internal/usage/application"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type AppDomainServices struct {
	dbPool               *pgxpool.Pool
	authServiceClient    *auth.ServiceClient
	runtimeSettings      *settings.RuntimeSettingsService
	storageService       *storage.Service
	userService          *administration.Service
	billingService       *billingpostgres.Store
	billingCatalog       *billingapplication.BillableItemCatalog
	billableItemRegistry *billing.BillableItemRegistry
	billingRecords       *billingrecordsapplication.Service
	creditService        *credits.Service
	usageService         *usageapplication.Service
	rewardService        *rewards.Service
	paymentService       *payments.Service
	userLifecycle        *IdentityLifecycle
	riverProducer        *river.Client[pgx.Tx]
}

func NewAppDomainServices(ctx context.Context, cfg *config.Config, repos AppRepositories, appPool *pgxpool.Pool) (AppDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	safehttp.ConfigurePolicyProvider(runtimeSettings)
	storageService := storage.NewServiceWithLocalRoot(
		repos.storageProfileRepo,
		// Browser-visible upload references must resolve through the same-origin
		// gateway. API_BASE_URL remains reserved for webhooks and server-side
		// integrations; using it here would leak a separate API host into the SPA.
		cfg.PublicOrigins.Web,
		cfg.Storage.LocalRoot,
	)
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return AppDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	welcomeIdentityKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(welcomeIdentityKey) != 32 {
		return AppDomainServices{}, fmt.Errorf("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	riverProducer, err := workerriver.NewRiverProducer(appPool)
	if err != nil {
		return AppDomainServices{}, fmt.Errorf("initialize River producer: %w", err)
	}
	creditReconciliationDispatcher, err := workerbilling.NewCreditReconciliationDispatcher(riverProducer)
	if err != nil {
		return AppDomainServices{}, err
	}
	creditService, err := credits.NewServiceWithReconciliation(appPool, creditReconciliationDispatcher)
	if err != nil {
		return AppDomainServices{}, err
	}
	rewardKeys, err := rewards.ParseCodeKeyring(cfg.Rewards.CodeHMACKeys)
	if err != nil {
		return AppDomainServices{}, err
	}
	rewardService := rewards.NewService(appPool, rewardscredits.New(creditService), rewardKeys)
	if err := rewardService.ValidateActiveCodeKeyVersions(ctx); err != nil {
		return AppDomainServices{}, err
	}
	billingItems := billingregistry.WorkflowItems()
	billingItems = append(billingItems, billingregistry.VisionSearchItem())
	billableItemRegistry, err := billing.NewBillableItemRegistry(billingItems)
	if err != nil {
		return AppDomainServices{}, fmt.Errorf("build billable item registry: %w", err)
	}
	billingService := billingpostgres.NewStore(appPool)
	billingCatalog := billingapplication.NewBillableItemCatalog(billableItemRegistry, billingService)
	userLifecycle := NewIdentityLifecycle(appPool, identitycreditsadapter.New(creditService), welcomeIdentityKey)
	if err := userLifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		return AppDomainServices{}, err
	}
	authServiceClient := auth.NewServiceClient(cfg.Auth)
	userService := administration.NewService(repos.userRepo, userLifecycle, creditService, storageService, authServiceClient)
	stripeAdapter := stripe.New()
	paymentCheckoutShutdownDispatcher, err := workerbilling.NewPaymentCheckoutShutdownDispatcher(riverProducer)
	if err != nil {
		return AppDomainServices{}, err
	}
	paymentRuntime := payments.NewPaymentRuntime(events.NewHub(), nil)
	paymentTransitions, err := payments.NewPaymentOrderTransitions(repos.paymentRepo, paymentRuntime)
	if err != nil {
		return AppDomainServices{}, fmt.Errorf("initialize payment order transitions: %w", err)
	}
	paymentService, err := payments.NewService(payments.ServiceDependencies{
		Configuration: repos.paymentRepo, ConfigurationGateway: stripeAdapter, ShutdownEnqueuer: paymentCheckoutShutdownDispatcher,
		Store: repos.paymentRepo, CheckoutGateway: stripeAdapter, Wallets: creditService,
		Convergence: repos.paymentRepo, Inbox: repos.paymentRepo, Verifier: stripeAdapter, RefundGateway: stripeAdapter,
		DisputeGateway: stripeAdapter, Transitions: paymentTransitions,
	})
	if err != nil {
		return AppDomainServices{}, fmt.Errorf("initialize payments service: %w", err)
	}
	return AppDomainServices{
		dbPool:               appPool,
		authServiceClient:    authServiceClient,
		runtimeSettings:      runtimeSettings,
		storageService:       storageService,
		userService:          userService,
		billingService:       billingService,
		billingCatalog:       billingCatalog,
		billableItemRegistry: billableItemRegistry,
		billingRecords:       billingrecordsapplication.NewService(billingrecordsapplication.NewRepository(appPool)),
		creditService:        creditService,
		usageService:         usageapplication.NewService(creditService, repos.taskBillingUsage, usageapplication.ServiceOptions{}),
		rewardService:        rewardService,
		paymentService:       paymentService,
		userLifecycle:        userLifecycle,
		riverProducer:        riverProducer,
	}, nil
}

// IdentityLifecycle composes the Identity application services used by runtime wiring.
type IdentityLifecycle struct {
	provisioning *provisioning.Service
	lifecycle    *lifecycle.Service
}

func NewIdentityLifecycle(
	pool *pgxpool.Pool,
	credits identityports.CreditWalletLifecycleService,
	welcomeIdentityKey []byte,
) *IdentityLifecycle {
	return &IdentityLifecycle{
		provisioning: provisioning.NewService(pool, credits, welcomeIdentityKey),
		lifecycle:    lifecycle.NewService(pool, credits),
	}
}

func (s *IdentityLifecycle) VerifyWelcomeIdentityKey(ctx context.Context) error {
	return s.provisioning.VerifyWelcomeIdentityKey(ctx)
}

func (s *IdentityLifecycle) ProvisionIdentity(ctx context.Context, identity identityports.VerifiedIdentity) (*identityports.ProvisionResult, error) {
	return s.provisioning.ProvisionIdentity(ctx, identity)
}

func (s *IdentityLifecycle) InitializeExistingIdentities(ctx context.Context, identities []identityports.VerifiedIdentity) ([]identityports.ProvisionResult, error) {
	return s.provisioning.InitializeExistingIdentities(ctx, identities)
}

func (s *IdentityLifecycle) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	return s.lifecycle.SetUserStatus(ctx, userID, status)
}

func (s *IdentityLifecycle) DeleteUser(
	ctx context.Context,
	userID int64,
	authDeleter identityports.AuthUserDeleter,
	uploadCleaner identityports.UserUploadCleaner,
) error {
	return s.lifecycle.DeleteUser(ctx, userID, authDeleter, uploadCleaner)
}

// creditsUserResolver adapts the identity repository without making the
// credits package depend on the users lifecycle package.
type creditsUserResolver struct {
	repo identityports.UserRepository
}

func (r creditsUserResolver) ResolvePublicID(ctx context.Context, publicID string) (credithttp.UserIdentity, error) {
	user, err := r.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return credithttp.UserIdentity{}, err
	}
	return credithttp.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (r creditsUserResolver) ResolveID(ctx context.Context, id int64) (credithttp.UserIdentity, error) {
	user, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return credithttp.UserIdentity{}, err
	}
	return credithttp.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

type rewardIdentityProjector struct {
	repo identityports.UserRepository
}

func (r rewardIdentityProjector) ResolveIdentity(
	ctx context.Context,
	userID int64,
) (rewards.IdentityProjection, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return rewards.IdentityProjection{}, err
	}
	return rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (r rewardIdentityProjector) ResolveIdentities(
	ctx context.Context,
	userIDs []int64,
) (map[int64]rewards.IdentityProjection, error) {
	usersByID, err := r.repo.GetByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	identities := make(map[int64]rewards.IdentityProjection, len(usersByID))
	for userID, user := range usersByID {
		identities[userID] = rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}
	}
	return identities, nil
}
