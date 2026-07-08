package bootstrap

import (
	"log/slog"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/runtime/httpapi"
	"ideaboard/api/internal/runtime/webdist"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"

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

	authDeps := httpapi.AuthDeps{
		AuthServiceClient: services.authServiceClient,
		UserRepo:          repos.userRepo,
	}
	uploadDeps := httpapi.UploadDeps{
		StorageService: services.storageService,
		UserRepo:       repos.userRepo,
		BoardService:   services.boardService,
		UploadSettings: services.runtimeSettings,
	}

	router := gin.New()
	router.Use(gin.Recovery())
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:      auth.NewAuthHandler(services.userService),
			TaskHandler:      taskHandler,
			BoardHandler:     boards.NewBoardHandler(services.boardService, services.runtimeSettings),
			LineDraftHandler: assets.NewLineDraftHandler(repos.lineDraftModeRepo),
			Uploads:          uploadDeps,
		},
		AdminRoutes: httpapi.AdminRouteDeps{
			AdminHandler:            users.NewAdminHandler(services.userService, services.runtimeSettings, services.authServiceClient),
			AccountHandler:          accountHandler,
			TaskHandler:             taskHandler,
			TaskStatsHandler:        taskStatsHandler,
			ProviderEndpointHandler: providerendpoints.NewProviderEndpointHandler(repos.channelRepo, cfg.App.Env),
			StorageHandler:          storage.NewStorageHandler(services.storageService),
			LineDraftHandler:        assets.NewLineDraftHandler(repos.lineDraftModeRepo),
			FeaturePromptHandler:    prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			StorageService:          services.storageService,
			UploadSettings:          services.runtimeSettings,
		},
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
