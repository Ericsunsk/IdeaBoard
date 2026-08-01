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
	storageService := storage.NewStorageService(repos.storageProfileRepo, runtimeSettings)
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return appDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	welcomeIdentityKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(welcomeIdentityKey) != 32 {
		return appDomainServices{}, fmt.Errorf("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	creditService := credits.NewService(appPool)
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
		paymentService: payments.NewService(payments.ServiceDependencies{
			Repository: repos.paymentRepo, ConfigurationGateway: stripeAdapter,
			CheckoutGateway: stripeAdapter, WebhookVerifier: stripeAdapter,
			Wallets: creditService, Events: payments.NewEventHub(),
		}),
		userLifecycle: userLifecycle,
	}, nil
}
