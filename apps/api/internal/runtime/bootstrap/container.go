package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/assets"
	"ideaboard/api/internal/auth"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/runtime/httpapi"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/taskservice"
	"ideaboard/api/internal/taskstats"
	"ideaboard/api/internal/taskstore"
	"ideaboard/api/internal/tasksubmission"
	"ideaboard/api/internal/users"
	"ideaboard/api/web"

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
	eventBus         *events.NATSJetStreamEventBus
	workerQueueName  string
	workerConcurrent int
}

func newAppContainer(cfg *config.Config) (*appContainer, error) {
	// encryption
	if err := secrets.LoadEncryptionKey(); err != nil {
		return nil, fmt.Errorf("failed to load secrets encryption key: %w", err)
	}

	// database + River
	authServiceClient := auth.NewServiceClient(cfg.Auth)

	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := migrations.Up(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}
	riverPool, err := worker.NewRiverPgxPool(context.Background(), database.PostgresDSN(cfg.Database))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect River database pool: %w", err)
	}
	if err := worker.MigrateRiver(context.Background(), riverPool); err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate River schema: %w", err)
	}
	// repositories
	postgresAssigner := taskqueue.NewPostgresTaskAssigner(db)
	postgresRuntimeStore := taskqueue.NewPostgresTaskRuntimeStore(db)

	userRepo := users.NewUserRepository(db)
	if err := userRepo.EnsureStorageUIDs(context.Background()); err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to ensure user storage uid: %w", err)
	}
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	channelRepo := channels.NewChannelRepository(db)
	systemConfigRepo := settings.NewSystemConfigRepository(db)
	featurePromptRepo := prompts.NewFeaturePromptRepository(db)
	storageProfileRepo := storage.NewStorageProfileRepository(db)
	lineDraftModeRepo := assets.NewLineDraftModeRepository(db)
	boardRepo := boards.NewBoardRepository(db)

	storageService := storage.NewStorageService(storageProfileRepo)
	if err := storageService.EnsureDefaultProfiles(context.Background()); err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}
	userService := users.NewUserService(userRepo, storageService)
	boardService := boards.NewBoardService(boardRepo, userRepo, storageService)
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := newTaskStatusEventPublisher(context.Background(), cfg.Events, taskEventHub)
	if err != nil {
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize task event bus: %w", err)
	}
	// services
	riverQueue := taskqueue.NewRiverQueue(nil, db, postgresRuntimeStore, cfg.Queue.MaxRetry)
	accountService := accounts.NewAccountService(accounts.AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       riverQueue,
		AccountLoadStore: postgresRuntimeStore,
		ConcurrencyStore: postgresRuntimeStore,
		CancelStore:      postgresRuntimeStore,
		MonitorStore:     riverQueue,
	}, accounts.AccountServiceOptions{
		TaskEvents: taskEvents,
	})
	riverTaskWorker := worker.NewRiverTaskWorker(
		accountRepo,
		channelRepo,
		taskRepo,
		riverQueue,
		postgresAssigner,
		postgresRuntimeStore,
		storageService,
		taskEvents,
	)
	riverTaskWorker.SetNoCapacityRetryDelay(time.Duration(cfg.Queue.NoCapacityRetryDelaySeconds) * time.Second)
	riverClient, err := worker.NewRiverClient(riverPool, riverTaskWorker, worker.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: cfg.Queue.WorkerConcurrency,
		JobTimeout:  time.Duration(cfg.Queue.TaskTimeoutSeconds) * time.Second,
		MaxRetry:    cfg.Queue.MaxRetry,
	})
	if err != nil {
		_ = eventBus.Close()
		riverPool.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize River client: %w", err)
	}
	riverQueue.SetClient(riverClient)
	submissionStore := tasksubmission.NewStore(riverPool, riverClient, taskqueue.RiverQueueDefault, cfg.Queue.MaxRetry)

	taskService := taskservice.NewService(taskservice.ServiceDeps{
		AccountRepo:       accountRepo,
		TaskRepo:          taskRepo,
		SubmissionStore:   submissionStore,
		CancelStore:       postgresRuntimeStore,
		FeaturePrompts:    featurePromptRepo,
		UploadBoards:      uploadBoardAccessResolver{service: boardService},
		UploadOwners:      uploadOwnerStorageUIDResolver{repo: userRepo},
		TaskOutputCleaner: storageService,
	}, taskservice.ServiceOptions{
		EventHub:   taskEventHub,
		TaskEvents: taskEvents,
	})
	taskStatsService := taskstats.NewService(taskRepo)
	accountHandler := accounts.NewAccountHandler(accountService)
	taskHandler := taskservice.NewHandler(taskService)
	taskStatsHandler := taskstats.NewHandler(taskStatsService, accountService)

	authDeps := httpapi.AuthDeps{
		AuthServiceClient: authServiceClient,
		UserRepo:          userRepo,
	}
	uploadDeps := httpapi.UploadDeps{
		StorageService: storageService,
		UserRepo:       userRepo,
		BoardService:   boardService,
	}
	router := gin.New()
	router.Use(gin.Recovery())
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		UserRoutes: httpapi.UserRouteDeps{
			AuthHandler:      auth.NewAuthHandler(userService),
			TaskHandler:      taskHandler,
			BoardHandler:     boards.NewBoardHandler(boardService),
			LineDraftHandler: assets.NewLineDraftHandler(lineDraftModeRepo),
			Uploads:          uploadDeps,
		},
		AdminRoutes: httpapi.AdminRouteDeps{
			AdminHandler:         users.NewAdminHandler(userService, systemConfigRepo, authServiceClient),
			AccountHandler:       accountHandler,
			TaskHandler:          taskHandler,
			TaskStatsHandler:     taskStatsHandler,
			ChannelHandler:       channels.NewChannelHandler(channelRepo, cfg.App.Env),
			StorageHandler:       storage.NewStorageHandler(storageService),
			LineDraftHandler:     assets.NewLineDraftHandler(lineDraftModeRepo),
			FeaturePromptHandler: prompts.NewFeaturePromptHandler(featurePromptRepo),
			StorageService:       storageService,
		},
		StaticUploads: httpapi.StaticUploadDeps{
			Auth:         authDeps,
			BoardService: boardService,
		},
		CORSMiddleware: buildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
	}, httpapi.FrontendRouteOptions{
		WarnMissingIndex: isProductionRuntime(cfg.App.Env),
	})

	return &appContainer{
		router:           router,
		db:               db,
		riverPool:        riverPool,
		riverClient:      riverClient,
		eventBus:         eventBus,
		workerQueueName:  taskqueue.RiverQueueDefault,
		workerConcurrent: cfg.Queue.WorkerConcurrency,
	}, nil
}

func (c *appContainer) close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	if c.eventBus != nil {
		closeErr = c.eventBus.Close()
	}
	if c.riverPool != nil {
		c.riverPool.Close()
	}
	if c.db != nil {
		if err := c.db.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies, frontendOptions httpapi.FrontendRouteOptions) {
	frontendFS, err := web.DistFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend dist", "err", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS, frontendOptions)
}

type taskEventHubPublisher struct {
	hub *taskservice.EventHub
}

func (p taskEventHubPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	if p.hub == nil {
		return nil
	}
	event = events.NormalizeTaskStatusEvent(event)
	p.hub.Publish(event.TaskID, taskservice.TaskEvent{TaskID: event.TaskID, Status: event.Status})
	return nil
}

type compositeTaskStatusPublisher struct {
	local  events.TaskStatusPublisher
	remote events.TaskStatusPublisher
}

func (p compositeTaskStatusPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	event = events.NormalizeTaskStatusEvent(event)
	var firstErr error
	if p.local != nil {
		if err := p.local.PublishTaskStatus(ctx, event); err != nil {
			firstErr = err
		}
	}
	if p.remote != nil {
		if err := p.remote.PublishTaskStatus(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func newTaskStatusEventPublisher(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
) (events.TaskStatusPublisher, *events.NATSJetStreamEventBus, error) {
	localPublisher := taskEventHubPublisher{hub: hub}
	if !cfg.Enabled {
		return localPublisher, nil, nil
	}
	if cfg.NATSURL == "" {
		return nil, nil, fmt.Errorf("NATS_EVENT_BUS_ENABLED=true requires NATS_URL")
	}

	bus, err := events.NewNATSJetStreamEventBus(ctx, events.NATSJetStreamConfig{
		URL:            cfg.NATSURL,
		StreamName:     cfg.NATSStreamName,
		SubjectPrefix:  cfg.NATSSubjectPrefix,
		ConsumerName:   cfg.NATSConsumerName,
		Retention:      time.Duration(cfg.NATSRetentionDays) * 24 * time.Hour,
		ConnectTimeout: time.Duration(cfg.NATSConnectTimeout) * time.Second,
		RequestTimeout: time.Duration(cfg.NATSRequestTimeout) * time.Second,
		DrainTimeout:   time.Duration(cfg.NATSDrainTimeout) * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := bus.SubscribeTaskStatus(ctx, func(ctx context.Context, event events.TaskStatusEvent) error {
		return localPublisher.PublishTaskStatus(ctx, event)
	}); err != nil {
		_ = bus.Close()
		return nil, nil, err
	}
	slog.Info("NATS JetStream task events enabled", "stream", cfg.NATSStreamName, "subject_prefix", cfg.NATSSubjectPrefix)
	return compositeTaskStatusPublisher{local: localPublisher, remote: bus}, bus, nil
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
