package bootstrap

import (
	"context"
	"fmt"
	"log"
	"time"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/imageproc"
	"api/internal/platform/config"
	"api/internal/platform/database"
	"api/internal/platform/redis"
	"api/internal/platform/settings"
	"api/internal/prompts"
	"api/internal/runtime/httpapi"
	"api/internal/runtime/taskqueue"
	"api/internal/runtime/worker"
	"api/internal/storage"
	"api/internal/users"
	"api/web"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

type appContainer struct {
	router *gin.Engine

	asynqQueue       *taskqueue.AsynqQueue
	asynqServer      *asynq.Server
	asynqMux         *asynq.ServeMux
	workerQueueName  string
	workerConcurrent int
}

func newAppContainer(cfg *config.Config) (*appContainer, error) {
	authServiceClient := auth.NewServiceClient(cfg.Auth)

	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to database: %w", err)
	}
	if err := boards.CleanupDuplicateBoardNames(db); err != nil {
		return nil, fmt.Errorf("Failed to cleanup duplicate board names: %w", err)
	}
	if err := migrateModels(db); err != nil {
		return nil, fmt.Errorf("Failed to migrate database schema: %w", err)
	}

	rdb := redis.NewRedis(cfg.Redis)
	if err := rdb.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("[Main] Failed to connect to Redis: %w", err)
	}
	redisMgr := redis.NewRedisManager(rdb.GetClient())
	redisAsynqAssigner := taskqueue.NewRedisAsynqAssigner(redisMgr)
	asynqConnOpt := worker.BuildAsynqRedisConnOpt(cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.Auth)
	asynqQueue := taskqueue.NewAsynqQueue(
		asynqConnOpt,
		redisMgr,
		time.Duration(cfg.Queue.AsynqTaskTimeoutSeconds)*time.Second,
		cfg.Queue.AsynqMaxRetry,
	)

	userRepo := users.NewUserRepository(db)
	if err := userRepo.EnsureStorageUIDs(); err != nil {
		return nil, fmt.Errorf("Failed to ensure user storage uid: %w", err)
	}
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	channelRepo := channels.NewChannelRepository(db)
	systemConfigRepo := settings.NewSystemConfigRepository(db)
	lineDraftModeRepo := assets.NewLineDraftModeRepository(db)
	boardRepo := boards.NewBoardRepository(db)

	storageService := storage.NewStorageService(systemConfigRepo)
	userService := users.NewUserService(userRepo, storageService)
	taskEventHub := accounts.NewTaskEventHub()
	accountService := accounts.NewAccountServiceWithQueue(
		accountRepo,
		taskRepo,
		asynqQueue,
		asynqQueue,
		redisAsynqAssigner,
		redisAsynqAssigner,
		redisAsynqAssigner,
		asynqQueue,
		systemConfigRepo,
		storageService,
	)
	accountService.SetTaskEventHub(taskEventHub)
	boardService := boards.NewBoardService(boardRepo, userRepo, storageService)

	router := gin.Default()
	registerHTTPRoutes(router, cfg, routeDependencies{
		authHandler:          auth.NewAuthHandler(userService),
		adminHandler:         users.NewAdminHandler(userService, systemConfigRepo, authServiceClient),
		accountHandler:       accounts.NewAccountHandler(accountService),
		boardHandler:         boards.NewBoardHandler(boardService),
		boardService:         boardService,
		channelHandler:       channels.NewChannelHandler(channelRepo, cfg.App.Env),
		storageHandler:       storage.NewStorageHandler(storageService),
		lineDraftHandler:     assets.NewLineDraftHandler(lineDraftModeRepo),
		featurePromptHandler: prompts.NewFeaturePromptHandler(systemConfigRepo),
		vectorizeHandler:     imageproc.NewVectorizeHandler(taskRepo, storageService),
		storageService:       storageService,
		userRepo:             userRepo,
		authServiceClient:    authServiceClient,
	})

	asynqHandler := worker.NewAsynqTaskHandler(
		accountRepo,
		channelRepo,
		taskRepo,
		asynqQueue,
		redisAsynqAssigner,
		redisAsynqAssigner,
		storageService,
		taskEventHub,
	)
	asynqServer, asynqMux := worker.NewAsynqServer(asynqConnOpt, worker.AsynqServerOptions{
		QueueName:            taskqueue.AsynqQueueDefault,
		Concurrency:          cfg.Queue.WorkerConcurrency,
		NoCapacityRetryDelay: time.Duration(cfg.Queue.AsynqNoCapacityDelaySec) * time.Second,
	}, asynqHandler)

	return &appContainer{
		router:           router,
		asynqQueue:       asynqQueue,
		asynqServer:      asynqServer,
		asynqMux:         asynqMux,
		workerQueueName:  taskqueue.AsynqQueueDefault,
		workerConcurrent: cfg.Queue.WorkerConcurrency,
	}, nil
}

func (c *appContainer) close() error {
	if c == nil || c.asynqQueue == nil {
		return nil
	}
	return c.asynqQueue.Close()
}

type routeDependencies struct {
	authHandler          *auth.AuthHandler
	adminHandler         *users.AdminHandler
	accountHandler       *accounts.AccountHandler
	boardHandler         *boards.BoardHandler
	boardService         *boards.BoardService
	channelHandler       *channels.ChannelHandler
	storageHandler       *storage.StorageHandler
	lineDraftHandler     *assets.LineDraftHandler
	featurePromptHandler *prompts.FeaturePromptHandler
	vectorizeHandler     *imageproc.VectorizeHandler
	storageService       *storage.StorageService
	userRepo             *users.UserRepository
	authServiceClient    *auth.ServiceClient
}

func registerHTTPRoutes(r *gin.Engine, cfg *config.Config, deps routeDependencies) {
	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	httpapi.RegisterRoutes(r, httpapi.Dependencies{
		AuthHandler:          deps.authHandler,
		AdminHandler:         deps.adminHandler,
		AccountHandler:       deps.accountHandler,
		BoardHandler:         deps.boardHandler,
		BoardService:         deps.boardService,
		ChannelHandler:       deps.channelHandler,
		StorageHandler:       deps.storageHandler,
		LineDraftHandler:     deps.lineDraftHandler,
		FeaturePromptHandler: deps.featurePromptHandler,
		VectorizeHandler:     deps.vectorizeHandler,
		StorageService:       deps.storageService,
		UserRepo:             deps.userRepo,
		AuthServiceClient:    deps.authServiceClient,
		CORSMiddleware:       buildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
	}, frontendFS)
}
