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
	"api/internal/runtime/taskruntime"
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
	if err := userRepo.EnsureStorageUIDs(context.Background()); err != nil {
		return nil, fmt.Errorf("Failed to ensure user storage uid: %w", err)
	}
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	channelRepo := channels.NewChannelRepository(db)
	systemConfigRepo := settings.NewSystemConfigRepository(db)
	lineDraftModeRepo := assets.NewLineDraftModeRepository(db)
	boardRepo := boards.NewBoardRepository(db)

	reconcileRuntime := taskruntime.NewService(accountRepo, taskRepo, redisAsynqAssigner, redisAsynqAssigner)
	reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 30*time.Second)
	reconcileResult, err := reconcileRuntime.ReconcileRunningTaskLeases(reconcileCtx, taskruntime.RunningTaskLeaseReconcileOptions{
		AssignmentDrainDelay: 2 * time.Second,
	})
	reconcileCancel()
	if err != nil {
		return nil, fmt.Errorf("Failed to reconcile running task leases: %w", err)
	}
	log.Printf("[Main] Reconciled running task leases: running=%d rebuilt_leases=%d rebuilt_account_counts=%d",
		reconcileResult.RunningTasks,
		reconcileResult.RebuiltLeases,
		reconcileResult.AccountCountsRebuilt,
	)

	storageService := storage.NewStorageService(systemConfigRepo)
	userService := users.NewUserService(userRepo, storageService)
	taskEventHub := accounts.NewTaskEventHub()
	accountService := accounts.NewAccountService(accounts.AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		Queue:            asynqQueue,
		QueueStats:       asynqQueue,
		Assigner:         redisAsynqAssigner,
		ConcurrencyStore: redisAsynqAssigner,
		CancelStore:      redisAsynqAssigner,
		MonitorStore:     asynqQueue,
		SystemConfigRepo: systemConfigRepo,
	}, accounts.AccountServiceOptions{
		TaskEventHub: taskEventHub,
	})
	boardService := boards.NewBoardService(boardRepo, userRepo, storageService)

	authDeps := httpapi.AuthDeps{
		AuthServiceClient: authServiceClient,
		UserRepo:          userRepo,
	}
	uploadDeps := httpapi.UploadDeps{
		StorageService: storageService,
		UserRepo:       userRepo,
		BoardService:   boardService,
	}
	router := gin.Default()
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:      auth.NewAuthHandler(userService),
			AccountHandler:   accounts.NewAccountHandler(accountService),
			BoardHandler:     boards.NewBoardHandler(boardService),
			LineDraftHandler: assets.NewLineDraftHandler(lineDraftModeRepo),
			VectorizeHandler: imageproc.NewVectorizeHandler(taskRepo, storageService),
			Uploads:          uploadDeps,
		},
		AdminRoutes: httpapi.AdminRouteDeps{
			AdminHandler:         users.NewAdminHandler(userService, systemConfigRepo, authServiceClient),
			AccountHandler:       accounts.NewAccountHandler(accountService),
			ChannelHandler:       channels.NewChannelHandler(channelRepo, cfg.App.Env),
			StorageHandler:       storage.NewStorageHandler(storageService),
			LineDraftHandler:     assets.NewLineDraftHandler(lineDraftModeRepo),
			FeaturePromptHandler: prompts.NewFeaturePromptHandler(systemConfigRepo),
			StorageService:       storageService,
		},
		StaticUploads: httpapi.StaticUploadDeps{
			Auth:         authDeps,
			BoardService: boardService,
		},
		CORSMiddleware: buildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
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

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies) {
	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS)
}
