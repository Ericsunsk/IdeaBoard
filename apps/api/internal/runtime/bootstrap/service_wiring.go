package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/payments/stripeadapter"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/runtime/billingregistry"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/usage"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDomainServices struct {
	authServiceClient    *auth.ServiceClient
	runtimeSettings      *settings.RuntimeSettingsService
	storageService       *storage.StorageService
	userService          *users.UserService
	boardService         *boards.BoardService
	billingService       *billing.Service
	billingCatalog       *billing.BillableItemCatalog
	billableItemRegistry *billing.BillableItemRegistry
	billingRecords       *billingrecords.Service
	creditService        *credits.Service
	usageService         *usage.Service
	rewardService        *rewards.Service
	paymentService       *payments.Service
	userLifecycle        *users.LifecycleService
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories, appPool *pgxpool.Pool) (appDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	outbound.ConfigurePolicyProvider(runtimeSettings)
	storageService := storage.NewStorageService(repos.storageProfileRepo)
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
	rewardService := rewards.NewService(appPool, creditService, rewardKeys)
	if err := rewardService.ValidateActiveCodeKeyVersions(ctx); err != nil {
		return appDomainServices{}, err
	}
	billableItemRegistry, err := billing.NewBillableItemRegistry(billingregistry.WorkflowItems())
	if err != nil {
		return appDomainServices{}, fmt.Errorf("build billable item registry: %w", err)
	}
	billingService := billing.NewService(appPool)
	billingCatalog := billing.NewBillableItemCatalog(billableItemRegistry, billingService)
	userLifecycle := users.NewLifecycleService(appPool, creditService, welcomeIdentityKey)
	if err := userLifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		return appDomainServices{}, err
	}
	userService := users.NewUserService(repos.userRepo, userLifecycle, creditService, storageService)
	stripeAdapter := stripeadapter.New()
	paymentCheckoutShutdownDispatcher, err := worker.NewPaymentCheckoutShutdownDispatcher(riverProducer)
	if err != nil {
		return appDomainServices{}, err
	}
	paymentRuntime := payments.NewPaymentRuntime(payments.NewEventHub(), nil)
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
		authServiceClient:    auth.NewServiceClient(cfg.Auth),
		runtimeSettings:      runtimeSettings,
		storageService:       storageService,
		userService:          userService,
		boardService:         boards.NewBoardService(repos.boardRepo, repos.userRepo, storageService),
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
