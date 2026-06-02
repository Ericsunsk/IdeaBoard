package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/db/migrations"
	"api/internal/imageproc"
	"api/internal/platform/config"
	"api/internal/platform/database"
	"api/internal/platform/settings"
	"api/internal/prompts"
	"api/internal/runtime/httpapi"
	"api/internal/runtime/taskqueue"
	"api/internal/runtime/worker"
	"api/internal/storage"
	"api/internal/users"
	"api/web"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type appContainer struct {
	router *gin.Engine

	db               *sql.DB
	riverPool        *pgxpool.Pool
	riverClient      *river.Client[pgx.Tx]
	workerQueueName  string
	workerConcurrent int
}

func newAppContainer(cfg *config.Config) (*appContainer, error) {
	authServiceClient := auth.NewServiceClient(cfg.Auth)

	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to database: %w", err)
	}
	if err := migrations.Up(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("Failed to migrate database schema: %w", err)
	}
	riverPool, err := worker.NewRiverPgxPool(context.Background(), database.PostgresDSN(cfg.Database))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("Failed to connect River database pool: %w", err)
	}
	if err := worker.MigrateRiver(context.Background(), riverPool); err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("Failed to migrate River schema: %w", err)
	}
	postgresAssigner := taskqueue.NewPostgresTaskAssigner(db)
	postgresRuntimeStore := taskqueue.NewPostgresTaskRuntimeStore(db)

	userRepo := users.NewUserRepository(db)
	if err := userRepo.EnsureStorageUIDs(context.Background()); err != nil {
		riverPool.Close()
		_ = db.Close()
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
	boardService := boards.NewBoardService(boardRepo, userRepo, storageService)
	taskEventHub := accounts.NewTaskEventHub()
	riverQueue := taskqueue.NewRiverQueue(nil, db, postgresRuntimeStore, cfg.Queue.MaxRetry)
	accountService := accounts.NewAccountService(accounts.AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		Queue:            riverQueue,
		QueueStats:       riverQueue,
		AccountLoadStore: postgresRuntimeStore,
		ConcurrencyStore: postgresRuntimeStore,
		CancelStore:      postgresRuntimeStore,
		MonitorStore:     riverQueue,
		SystemConfigRepo: systemConfigRepo,
		UploadBoards:     uploadBoardAccessResolver{service: boardService},
		UploadOwners:     uploadOwnerStorageUIDResolver{repo: userRepo},
	}, accounts.AccountServiceOptions{
		TaskEventHub: taskEventHub,
	})

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
			VectorizeHandler: imageproc.NewVectorizeHandler(taskRepo, storageService, imageproc.VectorizeDeps{
				UploadBoards: uploadBoardAccessResolver{service: boardService},
				UploadOwners: uploadOwnerStorageUIDResolver{repo: userRepo},
			}),
			Uploads: uploadDeps,
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

	riverTaskWorker := worker.NewRiverTaskWorker(
		accountRepo,
		channelRepo,
		taskRepo,
		riverQueue,
		postgresAssigner,
		postgresRuntimeStore,
		storageService,
		taskEventHub,
	)
	riverTaskWorker.SetNoCapacityRetryDelay(time.Duration(cfg.Queue.NoCapacityRetryDelaySeconds) * time.Second)
	riverClient, err := worker.NewRiverClient(riverPool, riverTaskWorker, worker.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: cfg.Queue.WorkerConcurrency,
		JobTimeout:  time.Duration(cfg.Queue.TaskTimeoutSeconds) * time.Second,
		MaxRetry:    cfg.Queue.MaxRetry,
	})
	if err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("Failed to initialize River client: %w", err)
	}
	riverQueue.SetClient(riverClient)

	return &appContainer{
		router:           router,
		db:               db,
		riverPool:        riverPool,
		riverClient:      riverClient,
		workerQueueName:  taskqueue.RiverQueueDefault,
		workerConcurrent: cfg.Queue.WorkerConcurrency,
	}, nil
}

func (c *appContainer) close() error {
	if c == nil {
		return nil
	}
	if c.riverPool != nil {
		c.riverPool.Close()
	}
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies) {
	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS)
}

type uploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r uploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.UserID, nil
}

type uploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r uploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}
