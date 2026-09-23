package composition

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	billinghttp "ideaboard/api/internal/billing/interfaces/http"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/principal"
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
	"ideaboard/api/internal/runtime/adapters/webdist"
	configinfra "ideaboard/api/internal/runtime/infrastructure/config"
	httpinfra "ideaboard/api/internal/runtime/infrastructure/http"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	"ideaboard/api/internal/runtime/ports"
	searchapplication "ideaboard/api/internal/searchcatalog/application"
	searchinfra "ideaboard/api/internal/searchcatalog/infrastructure"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	searchports "ideaboard/api/internal/searchcatalog/ports"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	taskhttp "ideaboard/api/internal/tasking/interfaces/http/task"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

func newAppRouter(
	cfg *config.Config,
	repos appRepositories,
	services appDomainServices,
	taskServices appTaskServices,
	readyChecker ports.HealthChecker,
	redMetrics *redmetrics.Set,
) (*gin.Engine, error) {
	taskHandler := taskhttp.NewHandler(taskhttp.HandlerDeps{
		Commands: taskServices.taskService, AdminQueries: taskServices.taskService, UserQueries: taskServices.taskService,
	})
	taskStatsHandler := statisticshttp.NewHandler(taskServices.taskStatsService, taskServices.monitor)
	billingHandler := billinghttp.NewHandler(services.billingCatalog, services.billingCatalog)
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
	searchProviderConfig := searchinfra.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayRepo := aigateway.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayHandler := aigateway.NewHandler(aiGatewayRepo)
	aiGatewayHandler.SetObservedService(aigateway.NewObservedService(
		aigateway.NewObservedRepository(repos.db),
		aiGatewayRepo,
		newBillingSKUCatalogAdapter(services.billingCatalog),
	))
	libraryMaterializer := librarymaterialization.NewService(repos.db, services.storageService, repos.mediaAssetRegistry)
	libraryService := library.NewService(repos.libraryRepo, libraryMaterializer)
	libraryHandler := library.NewHandler(libraryService)
	visionService := searchapplication.NewVisionSearchService(visionModelAdapter{repo: aiGatewayRepo}, visionGatewayAdapter{repo: aiGatewayRepo, metrics: redMetrics})
	visionHandler := searchhttp.NewVisionHandler(visionService, searchhttp.NewVisionInputReader(libraryService))
	visionHandler.SetBillingService(searchapplication.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, visionService))

	router := gin.New()
	// ClientIP feeds per-IP rate limiting. Trust only the configured reverse
	// proxies so X-Forwarded-For cannot be spoofed by direct clients.
	// Production behind Nginx/Cloudflare must set APP_TRUSTED_PROXIES.
	if err := router.SetTrustedProxies(cfg.App.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid trusted proxies %v: %w", cfg.App.TrustedProxies, err)
	}
	// Panic recovery, access logging and request IDs are registered in
	// registerGlobalMiddleware so every route shares the error envelope.
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth:                authDeps,
		PublicSearchHandler: searchhttp.NewSearchHandler(searchapplication.NewSearchService(searchProviderConfig, []searchports.SearchProviderAdapter{searchinfra.NewUnsplashAdapter(), searchinfra.NewPexelsAdapter()})),
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:            auth.NewAuthHandler(services.userService),
			TaskHandler:            taskHandler,
			BoardHandler:           boards.NewHandler(services.boardService, cfg.PublicOrigins),
			BillingHandler:         billingHandler,
			BillingRecords:         billingrecords.NewHandler(services.billingRecords),
			CreditUserHandler:      creditUserHandler,
			UsageHandler:           usage.NewHandler(services.usageService),
			RewardUserHandler:      rewardUserHandler,
			PaymentUserHandler:     paymentUserHandler,
			UserEventStreamHandler: userEventStreamHandler,
			MediaAssetRegistry:     repos.mediaAssetRegistry,
			MediaStorageService:    services.storageService,
			Imgproxy:               imgproxyTierConfig(cfg),
			Uploads:                uploadDeps,
			UploadDownloads: httpapi.UploadDownloadDeps{
				Reader:     services.storageService,
				ReadAccess: uploadReadAccess,
			},
			VisionHandler:  visionHandler,
			LibraryHandler: libraryHandler,
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
			TaskHandler:                 taskHandler,
			TaskStatsHandler:            taskStatsHandler,
			StorageHandler:              storagehttp.NewHandler(services.storageService),
			FeaturePromptHandler:        prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			StorageService:              services.storageService,
			UploadSettings:              services.runtimeSettings,
			RewardAdminCommandHandler:   rewardAdminCommandHandler,
			RewardAdminReadHandler:      rewardAdminReadHandler,
			PaymentAdminHandler:         paymentAdminHandler,
			SearchCatalogHandler:        searchhttp.NewHandler(searchProviderConfig, searchinfra.NewProviderTestAdapters()...),
			AIGatewayHandler:            aiGatewayHandler,
			AdminBrandLogoHandler: adminbranding.NewHandler(repos.adminBrandLogoRepo, func(c *gin.Context) bool {
				_, ok := principal.RequireAdmin(c)
				return ok
			}),
		},
		PaymentWebhook: paymentWebhookHandler,
		Internal:       httpapi.InternalRouteDeps{UserHandler: identityhttp.NewInternalHandler(repos.userRepo, cfg.Auth.InternalAPIKey)},
		StaticUploads: httpapi.StaticUploadDeps{Auth: authDeps,
			BoardService: services.boardService,
			ReadAccess:   uploadReadAccess,
		},
		CORSMiddleware: httpinfra.BuildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
		RateLimit:      services.runtimeSettings,
		Branding:       services.runtimeSettings,
		REDMetrics:     redMetrics,
		ReadyChecker:   readyChecker,
	}, httpapi.FrontendRouteOptions{
		WarnMissingIndex: configinfra.IsProductionRuntime(cfg.App.Env),
	})

	return router, nil
}

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies, frontendOptions httpapi.FrontendRouteOptions) {
	frontendFS, err := webdist.DistFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend dist", "err", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS, frontendOptions)
}

// visionGatewayAdapter routes vision search through the central AI gateway
// connection (openrouter route). Composition owns this glue so the
// searchcatalog context never imports the runtime transport.
type visionGatewayAdapter struct {
	repo    *aigateway.ConfigRepository
	metrics *redmetrics.Set
}

func (a visionGatewayAdapter) Do(ctx context.Context, req searchports.VisionGatewayRequest) (searchports.VisionGatewayResponse, error) {
	if a.repo == nil {
		return searchports.VisionGatewayResponse{}, fmt.Errorf("ai gateway config repository is unavailable")
	}
	resolved, err := a.repo.Resolve(ctx)
	if err != nil {
		return searchports.VisionGatewayResponse{}, err
	}
	client, err := infrastructure.NewGatewayClient(infrastructure.GatewayClientConfig{
		BaseURL: resolved.BaseURL,
		Token:   resolved.Token,
		// Vision rides the standard outbound client through the gateway
		// seam; its own target keeps vision timeout/retry behavior
		// separately observable from image calls in logs/metrics.
		Target:  "vision-search",
		Metrics: a.metrics,
	})
	if err != nil {
		return searchports.VisionGatewayResponse{}, err
	}
	resp, err := client.Do(ctx, ports.GatewayRequest{
		Endpoint: ports.GatewayEndpoint{
			Kind:     ports.GatewayEndpointProviderNative,
			Provider: "openrouter",
			Path:     "api/v1/chat/completions",
		},
		Model:   req.Model,
		Feature: "vision-search",
		Body:    req.Body,
		Cache:   ports.GatewayCachePolicy{Mode: ports.GatewayCacheSkip},
		Timeout: req.Timeout,
	})
	if err != nil {
		return searchports.VisionGatewayResponse{}, err
	}
	return searchports.VisionGatewayResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
	}, nil
}

// visionModelAdapter resolves the vision search model from the central
// AI gateway mappings (fixed "vision" key). Composition owns this glue so
// the searchcatalog context never imports the gateway config package.
type visionModelAdapter struct {
	repo *aigateway.ConfigRepository
}

func (a visionModelAdapter) VisionConfig(ctx context.Context) (searchports.VisionProviderConfig, error) {
	if a.repo == nil {
		return searchports.VisionProviderConfig{}, fmt.Errorf("ai gateway config repository is unavailable")
	}
	model, err := a.repo.ResolveVisionModel(ctx)
	if err != nil {
		return searchports.VisionProviderConfig{}, err
	}
	return searchports.VisionProviderConfig{Model: model}, nil
}
