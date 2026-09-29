package composition

import (
	"context"
	"encoding/base64"
	"fmt"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/billingrecords"
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
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	"ideaboard/api/internal/payments/infrastructure/stripe"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/rewards"
	creditsadapter "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/runtime/adapters/billingregistry"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	"ideaboard/api/internal/usage"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type appDomainServices struct {
	dbPool               *pgxpool.Pool
	authServiceClient    *auth.ServiceClient
	runtimeSettings      *settings.RuntimeSettingsService
	storageService       *storage.Service
	userService          *administration.Service
	billingService       *billingpostgres.Store
	billingCatalog       *billingapplication.BillableItemCatalog
	billableItemRegistry *billing.BillableItemRegistry
	billingRecords       *billingrecords.Service
	creditService        *credits.Service
	usageService         *usage.Service
	rewardService        *rewards.Service
	paymentService       *payments.Service
	userLifecycle        *identityLifecycle
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories, appPool *pgxpool.Pool) (appDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	outbound.ConfigurePolicyProvider(runtimeSettings)
	storageService := storage.NewServiceWithLocalRoot(
		repos.storageProfileRepo,
		// Browser-visible upload references must resolve through the same-origin
		// gateway. API_BASE_URL remains reserved for webhooks and server-side
		// integrations; using it here would leak a separate API host into the SPA.
		cfg.PublicOrigins.Web,
		cfg.Storage.LocalRoot,
	)
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return appDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	welcomeIdentityKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(welcomeIdentityKey) != 32 {
		return appDomainServices{}, fmt.Errorf("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	riverProducer, err := workerriver.NewRiverProducer(appPool)
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize River producer: %w", err)
	}
	creditReconciliationDispatcher, err := workerbilling.NewCreditReconciliationDispatcher(riverProducer)
	if err != nil {
		return appDomainServices{}, err
	}
	creditService, err := credits.NewServiceWithReconciliation(appPool, creditReconciliationDispatcher)
	if err != nil {
		return appDomainServices{}, err
	}
	rewardKeys, err := rewards.ParseCodeKeyring(cfg.Rewards.CodeHMACKeys)
	if err != nil {
		return appDomainServices{}, err
	}
	rewardService := rewards.NewService(appPool, creditsadapter.New(creditService), rewardKeys)
	if err := rewardService.ValidateActiveCodeKeyVersions(ctx); err != nil {
		return appDomainServices{}, err
	}
	billingItems := billingregistry.WorkflowItems()
	billingItems = append(billingItems, billingregistry.VisionSearchItem())
	billableItemRegistry, err := billing.NewBillableItemRegistry(billingItems)
	if err != nil {
		return appDomainServices{}, fmt.Errorf("build billable item registry: %w", err)
	}
	billingService := billingpostgres.NewStore(appPool)
	billingCatalog := billingapplication.NewBillableItemCatalog(billableItemRegistry, billingService)
	userLifecycle := newIdentityLifecycle(appPool, identitycreditsadapter.New(creditService), welcomeIdentityKey)
	if err := userLifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		return appDomainServices{}, err
	}
	authServiceClient := auth.NewServiceClient(cfg.Auth)
	userService := administration.NewService(repos.userRepo, userLifecycle, creditService, storageService, authServiceClient)
	stripeAdapter := stripe.New()
	paymentCheckoutShutdownDispatcher, err := workerbilling.NewPaymentCheckoutShutdownDispatcher(riverProducer)
	if err != nil {
		return appDomainServices{}, err
	}
	paymentRuntime := payments.NewPaymentRuntime(paymentevents.NewHub(), nil)
	paymentTransitions, err := payments.NewPaymentOrderTransitions(repos.paymentRepo, paymentRuntime)
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize payment order transitions: %w", err)
	}
	paymentCheckout, err := payments.NewCheckoutWorkflow(payments.CheckoutWorkflowDependencies{
		Configuration: repos.paymentRepo, ConfigurationGateway: stripeAdapter, ShutdownEnqueuer: paymentCheckoutShutdownDispatcher,
		Store: repos.paymentRepo, CheckoutGateway: stripeAdapter, Wallets: creditService,
		Convergence: repos.paymentRepo, Transitions: paymentTransitions,
	})
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize payment Checkout workflow: %w", err)
	}
	paymentFacts, err := payments.NewStripeFactsWorkflow(payments.StripeFactsWorkflowDependencies{
		Configuration: repos.paymentRepo, Inbox: repos.paymentRepo, Convergence: repos.paymentRepo, Verifier: stripeAdapter, RefundGateway: stripeAdapter,
		DisputeGateway: stripeAdapter, CheckoutStore: repos.paymentRepo, CheckoutGateway: stripeAdapter, CheckoutRecovery: paymentCheckout, Transitions: paymentTransitions,
	})
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize Stripe facts workflow: %w", err)
	}
	paymentService, err := payments.NewService(paymentCheckout, paymentFacts)
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize payments service: %w", err)
	}
	return appDomainServices{
		dbPool:               appPool,
		authServiceClient:    authServiceClient,
		runtimeSettings:      runtimeSettings,
		storageService:       storageService,
		userService:          userService,
		billingService:       billingService,
		billingCatalog:       billingCatalog,
		billableItemRegistry: billableItemRegistry,
		billingRecords:       billingrecords.NewService(billingrecords.NewRepository(appPool)),
		creditService:        creditService,
		usageService:         usage.NewService(creditService, repos.taskBillingUsage, usage.ServiceOptions{}),
		rewardService:        rewardService,
		paymentService:       paymentService,
		userLifecycle:        userLifecycle,
	}, nil
}

// identityLifecycle composes the Identity application services used by runtime wiring.
type identityLifecycle struct {
	provisioning *provisioning.Service
	lifecycle    *lifecycle.Service
}

func newIdentityLifecycle(
	pool *pgxpool.Pool,
	credits identityports.CreditWalletLifecycleService,
	welcomeIdentityKey []byte,
) *identityLifecycle {
	return &identityLifecycle{
		provisioning: provisioning.NewService(pool, credits, welcomeIdentityKey),
		lifecycle:    lifecycle.NewService(pool, credits),
	}
}

func (s *identityLifecycle) VerifyWelcomeIdentityKey(ctx context.Context) error {
	return s.provisioning.VerifyWelcomeIdentityKey(ctx)
}

func (s *identityLifecycle) ProvisionIdentity(ctx context.Context, identity identityports.VerifiedIdentity) (*identityports.ProvisionResult, error) {
	return s.provisioning.ProvisionIdentity(ctx, identity)
}

func (s *identityLifecycle) InitializeExistingIdentities(ctx context.Context, identities []identityports.VerifiedIdentity) ([]identityports.ProvisionResult, error) {
	return s.provisioning.InitializeExistingIdentities(ctx, identities)
}

func (s *identityLifecycle) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	return s.lifecycle.SetUserStatus(ctx, userID, status)
}

func (s *identityLifecycle) DeleteUser(
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

type taskUserResolver struct {
	repo identityports.UserRepository
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

func (r taskUserResolver) ResolvePublicID(ctx context.Context, publicID string) (int64, error) {
	user, err := r.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

func (r taskUserResolver) ResolveUser(ctx context.Context, userID int64) (uuid.UUID, string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return uuid.Nil, "", err
	}
	return user.PublicID, user.DisplayName, nil
}
