package bootstrap

import (
	"log/slog"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/runtime/httpapi"
	"ideaboard/api/internal/runtime/webdist"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

func newAppRouter(
	cfg *config.Config,
	repos appRepositories,
	services appDomainServices,
	taskServices appTaskServices,
) *gin.Engine {
	accountHandler := accounts.NewAccountHandler(taskServices.accountService)
	taskHandler := taskservice.NewHandler(taskServices.taskService)
	taskStatsHandler := taskstats.NewHandler(taskServices.taskStatsService, taskServices.accountService)
	billingHandler := billing.NewHandler(services.billingCatalog, services.billingCatalog)
	paymentUserHandler := payments.NewUserCheckoutHandler(services.paymentService, cfg.PublicOrigins)
	paymentEventStreamHandler := payments.NewEventStreamHandler(services.paymentService.EventHub())
	paymentAdminHandler := payments.NewAdminHandler(services.paymentService, cfg.PublicOrigins)
	paymentWebhookHandler := payments.NewWebhookHandler(services.paymentService)
	creditUserResolver := creditsUserResolver{repo: repos.userRepo}
	creditUserHandler := credits.NewUserHandler(services.creditService, creditUserResolver)
	creditAdminCommandHandler := credits.NewAdminCommandHandler(services.creditService, creditUserResolver)
	creditAdminReadHandler := credits.NewAdminReadHandler(services.creditService, creditUserResolver)
	creditReconciliationHandler := credits.NewReconciliationHandler(services.creditService, creditUserResolver)
	rewardIdentityProjection := rewardIdentityProjector{repo: repos.userRepo}
	rewardUserHandler := rewards.NewUserHandler(services.rewardService)
	rewardAdminCommandHandler := rewards.NewAdminCommandHandler(services.rewardService)
	rewardAdminReadHandler := rewards.NewAdminReadHandler(services.rewardService, rewardIdentityProjection)

	authDeps := httpapi.AuthDeps{
		AuthServiceClient: services.authServiceClient,
		UserRepo:          repos.userRepo,
		UserLifecycle:     services.userLifecycle,
	}
	uploadDeps := httpapi.UploadDeps{
		StorageService: services.storageService,
		MediaAssets:    repos.mediaAssetRegistry,
		UserRepo:       repos.userRepo,
		BoardService:   services.boardService,
		UploadSettings: services.runtimeSettings,
	}

	router := gin.New()
	router.Use(gin.Recovery())
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:               auth.NewAuthHandler(services.userService),
			TaskHandler:               taskHandler,
			BoardHandler:              boards.NewBoardHandler(services.boardService, cfg.PublicOrigins),
			BillingHandler:            billingHandler,
			BillingRecords:            billingrecords.NewHandler(services.billingRecords),
			CreditUserHandler:         creditUserHandler,
			UsageHandler:              usage.NewHandler(services.usageService),
			RewardUserHandler:         rewardUserHandler,
			PaymentUserHandler:        paymentUserHandler,
			PaymentEventStreamHandler: paymentEventStreamHandler,
			Uploads:                   uploadDeps,
		},
		AdminRoutes: httpapi.AdminRouteDeps{
			AdminHandler: users.NewAdminHandler(
				services.userService,
				services.runtimeSettings,
				users.NewIdentityAdminCommands(services.userService, services.authServiceClient),
				services.authServiceClient,
			),
			MailConfigHandler:           auth.NewMailConfigAdminHandler(services.authServiceClient),
			SocialProviderHandler:       auth.NewSocialProviderAdminHandler(services.authServiceClient),
			BillingHandler:              billingHandler,
			CreditAdminCommandHandler:   creditAdminCommandHandler,
			CreditAdminReadHandler:      creditAdminReadHandler,
			CreditReconciliationHandler: creditReconciliationHandler,
			AccountHandler:              accountHandler,
			TaskHandler:                 taskHandler,
			TaskStatsHandler:            taskStatsHandler,
			ProviderEndpointHandler:     providerendpoints.NewProviderEndpointHandler(repos.providerEndpointRepo, cfg.App.Env),
			StorageHandler:              storage.NewStorageHandler(services.storageService),
			FeaturePromptHandler:        prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			StorageService:              services.storageService,
			UploadSettings:              services.runtimeSettings,
			RewardAdminCommandHandler:   rewardAdminCommandHandler,
			RewardAdminReadHandler:      rewardAdminReadHandler,
			PaymentAdminHandler:         paymentAdminHandler,
		},
		PaymentWebhook: paymentWebhookHandler,
		StaticUploads: httpapi.StaticUploadDeps{
			Auth:         authDeps,
			BoardService: services.boardService,
			ReadAccess: storage.NewReadableUploadAccess(
				uploadBoardAccessResolver{service: services.boardService},
				uploadOwnerStorageUIDResolver{repo: repos.userRepo},
			),
		},
		CORSMiddleware: buildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
		RateLimit:      services.runtimeSettings,
		Branding:       services.runtimeSettings,
	}, httpapi.FrontendRouteOptions{
		WarnMissingIndex: isProductionRuntime(cfg.App.Env),
	})

	return router
}

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies, frontendOptions httpapi.FrontendRouteOptions) {
	frontendFS, err := webdist.DistFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend dist", "err", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS, frontendOptions)
}
