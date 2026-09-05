package bootstrap

import (
	"log/slog"

	accounthttp "ideaboard/api/internal/accountpool/accounts/interfaces/http"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users/application/administration"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/librarymaterialization"
	storageauth "ideaboard/api/internal/media/storage/application/authorization"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/runtime/httpapi"
	"ideaboard/api/internal/runtime/webdist"
	"ideaboard/api/internal/searchcatalog"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	taskhttp "ideaboard/api/internal/tasking/interfaces/http/task"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func newAppRouter(
	cfg *config.Config,
	repos appRepositories,
	services appDomainServices,
	taskServices appTaskServices,
) *gin.Engine {
	accountHandler := accounthttp.NewHandler(taskServices.accountService)
	taskHandler := taskhttp.NewHandler(taskhttp.HandlerDeps{
		Commands: taskServices.taskService, AdminQueries: taskServices.taskService, UserQueries: taskServices.taskService,
	})
	taskStatsHandler := statisticshttp.NewHandler(taskServices.taskStatsService, taskServices.accountService)
	billingHandler := billing.NewHandler(services.billingCatalog, services.billingCatalog)
	paymentUserHandler := paymenthttp.NewUserCheckoutHandler(services.paymentService, cfg.PublicOrigins)
	userEventStreamHandler := httpapi.NewUserEventStreamHandler(taskServices.userEvents)
	paymentAdminHandler := paymenthttp.NewAdminHandler(services.paymentService, cfg.PublicOrigins)
	paymentWebhookHandler := paymenthttp.NewWebhookHandler(services.paymentService)
	creditUserResolver := creditsUserResolver{repo: repos.userRepo}
	creditUserHandler := credithttp.NewUserHandler(services.creditService, creditUserResolver)
	creditAdminCommandHandler := credithttp.NewAdminCommandHandler(services.creditService, creditUserResolver)
	creditAdminReadHandler := credithttp.NewAdminReadHandler(services.creditService, creditUserResolver)
	creditReconciliationHandler := credithttp.NewReconciliationHandler(services.creditService, creditUserResolver)
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
	uploadReadAccess := storageauth.NewReadableUploadAccess(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	searchProviderConfig := searchcatalog.NewConfigRepository(repos.systemConfigRepo)
	libraryMaterializer := librarymaterialization.NewService(repos.db, services.storageService, repos.mediaAssetRegistry)
	libraryService := library.NewService(repos.libraryRepo, libraryMaterializer)
	libraryHandler := library.NewHandler(libraryService)
	searchMetrics, metricsErr := searchcatalog.NewPrometheusMetrics(prometheus.DefaultRegisterer)
	if metricsErr != nil {
		slog.Warn("search metrics unavailable", "err", metricsErr)
	}
	visionService := searchcatalog.NewVisionSearchService(searchProviderConfig, searchMetrics)
	visionHandler := searchcatalog.NewVisionHandler(visionService, searchcatalog.NewVisionInputReader(libraryService))
	visionHandler.SetBillingService(searchcatalog.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, visionService))

	router := gin.New()
	router.Use(gin.Recovery())
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth:                authDeps,
		PublicSearchHandler: searchcatalog.NewSearchHandler(searchcatalog.NewSearchService(searchProviderConfig, searchMetrics)),
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:            auth.NewAuthHandler(services.userService),
			TaskHandler:            taskHandler,
			BoardHandler:           boards.NewBoardHandler(services.boardService, cfg.PublicOrigins),
			BillingHandler:         billingHandler,
			BillingRecords:         billingrecords.NewHandler(services.billingRecords),
			CreditUserHandler:      creditUserHandler,
			UsageHandler:           usage.NewHandler(services.usageService),
			RewardUserHandler:      rewardUserHandler,
			PaymentUserHandler:     paymentUserHandler,
			UserEventStreamHandler: userEventStreamHandler,
			MediaAssetRegistry:     repos.mediaAssetRegistry,
			MediaStorageService:    services.storageService,
			Uploads:                uploadDeps,
			UploadDownloads: httpapi.UploadDownloadDeps{
				Reader:     services.storageService,
				ReadAccess: uploadReadAccess,
			},
			VisionHandler:        visionHandler,
			SearchMetricsHandler: searchcatalog.NewMetricsHandler(searchMetrics),
			LibraryHandler:       libraryHandler,
		},
		AdminRoutes: httpapi.AdminRouteDeps{
			AdminHandler: identityhttp.NewAdminHandler(
				services.userService,
				services.runtimeSettings,
				administration.NewIdentityAdminCommands(services.userService, services.authServiceClient),
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
			StorageHandler:              storagehttp.NewStorageHandler(services.storageService),
			FeaturePromptHandler:        prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			StorageService:              services.storageService,
			UploadSettings:              services.runtimeSettings,
			RewardAdminCommandHandler:   rewardAdminCommandHandler,
			RewardAdminReadHandler:      rewardAdminReadHandler,
			PaymentAdminHandler:         paymentAdminHandler,
			SearchCatalogHandler:        searchcatalog.NewHandler(searchProviderConfig),
			AdminBrandLogoHandler:       adminbranding.NewHandler(repos.adminBrandLogoRepo),
		},
		PaymentWebhook: paymentWebhookHandler,
		Internal:       httpapi.InternalRouteDeps{UserHandler: identityhttp.NewInternalHandler(repos.userRepo, cfg.Auth.InternalAPIKey)},
		StaticUploads: httpapi.StaticUploadDeps{
			Auth:         authDeps,
			BoardService: services.boardService,
			ReadAccess:   uploadReadAccess,
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
