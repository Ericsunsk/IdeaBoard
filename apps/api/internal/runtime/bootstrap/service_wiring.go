package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"

	providerendpointsapp "ideaboard/api/internal/accountpool/providerendpoints/application"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users/application/administration"
	identitycreditsadapter "ideaboard/api/internal/identity/users/infrastructure/credits"
	"ideaboard/api/internal/media/storage"
	storageintegrity "ideaboard/api/internal/media/storage/application/integrity"
	"ideaboard/api/internal/payments"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	"ideaboard/api/internal/payments/infrastructure/stripe"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/rewards"
	creditsadapter "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/runtime/billingregistry"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/usage"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDomainServices struct {
	dbPool                  *pgxpool.Pool
	authServiceClient       *auth.ServiceClient
	runtimeSettings         *settings.RuntimeSettingsService
	storageService          *storage.Service
	userService             *administration.Service
	boardService            *boards.Service
	billingService          *billing.Service
	billingCatalog          *billing.BillableItemCatalog
	billableItemRegistry    *billing.BillableItemRegistry
	billingRecords          *billingrecords.Service
	creditService           *credits.Service
	usageService            *usage.Service
	rewardService           *rewards.Service
	paymentService          *payments.Service
	userLifecycle           *identityLifecycle
	providerEndpointService *providerendpointsapp.Service
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories, appPool *pgxpool.Pool) (appDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	outbound.ConfigurePolicyProvider(runtimeSettings)
	storageService := storage.NewService(
		repos.storageProfileRepo,
		repos.mediaAssetRegistry,
		// Browser-visible upload references must resolve through the same-origin
		// gateway. API_BASE_URL remains reserved for webhooks and server-side
		// integrations; using it here would leak a separate API host into the SPA.
		cfg.PublicOrigins.Web,
	)
	repos.mediaAssetRegistry.SetIntegrityVerifier(storageintegrity.NewIntegrityVerifier(storageService, storageintegrity.IntegrityConfig{}))
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return appDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	welcomeIdentityKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(welcomeIdentityKey) != 32 {
		return appDomainServices{}, fmt.Errorf("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	riverProducer, err := worker.NewRiverProducer(appPool)
	if err != nil {
		return appDomainServices{}, fmt.Errorf("initialize River producer: %w", err)
	}
	creditReconciliationDispatcher, err := worker.NewCreditReconciliationDispatcher(riverProducer)
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
	billingService := billing.NewService(appPool)
	billingCatalog := billing.NewBillableItemCatalog(billableItemRegistry, billingService)
	userLifecycle := newIdentityLifecycle(appPool, identitycreditsadapter.New(creditService), welcomeIdentityKey)
	if err := userLifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		return appDomainServices{}, err
	}
	authServiceClient := auth.NewServiceClient(cfg.Auth)
	userService := administration.NewService(repos.userRepo, userLifecycle, creditService, storageService, authServiceClient)
	stripeAdapter := stripe.New()
	paymentCheckoutShutdownDispatcher, err := worker.NewPaymentCheckoutShutdownDispatcher(riverProducer)
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
		dbPool:                  appPool,
		authServiceClient:       authServiceClient,
		runtimeSettings:         runtimeSettings,
		storageService:          storageService,
		userService:             userService,
		boardService:            boards.NewService(repos.boardRepo, repos.userRepo, storageService),
		billingService:          billingService,
		billingCatalog:          billingCatalog,
		billableItemRegistry:    billableItemRegistry,
		billingRecords:          billingrecords.NewService(billingrecords.NewRepository(appPool)),
		creditService:           creditService,
		usageService:            usage.NewService(creditService, repos.taskBillingUsage, usage.ServiceOptions{}),
		rewardService:           rewardService,
		paymentService:          paymentService,
		userLifecycle:           userLifecycle,
		providerEndpointService: providerendpointsapp.NewService(repos.providerEndpointRepo, cfg.App.Env),
	}, nil
}
