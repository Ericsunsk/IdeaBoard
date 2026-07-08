package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"
	"ideaboard/api/internal/tasking/tasksubmission"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type appTaskServices struct {
	accountService   *accounts.AccountService
	taskService      *taskservice.Service
	taskStatsService *taskstats.Service
}

type appWorkerRuntime struct {
	riverClient *river.Client[pgx.Tx]
	eventBus    *events.NATSJetStreamEventBus
	queueName   string
	concurrency int
}

func (r appWorkerRuntime) close() error {
	if r.eventBus == nil {
		return nil
	}
	return r.eventBus.Close()
}

func newAppTaskRuntime(
	ctx context.Context,
	cfg *config.Config,
	dbs appDatabases,
	repos appRepositories,
	services appDomainServices,
) (appTaskServices, appWorkerRuntime, error) {
	postgresAssigner := taskqueue.NewPostgresTaskAssigner(dbs.db)
	postgresRuntimeStore := taskqueue.NewPostgresTaskRuntimeStore(dbs.db)
	taskMediaAuthorizer := storage.NewTaskMediaAuthorizer(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	runtimeTaskMediaStore := storage.NewRuntimeTaskMediaStore(services.storageService)
	submitMediaValidator := taskmedia.NewSubmitMediaValidator(services.runtimeSettings, taskMediaAuthorizer)
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := newTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub)
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}

	riverQueue := taskqueue.NewRiverQueue(nil, dbs.db, postgresRuntimeStore, cfg.Queue.MaxRetry)
	accountService := accounts.NewAccountService(accounts.AccountServiceDeps{
		AccountRepo:      repos.accountRepo,
		TaskRepo:         repos.taskRepo,
		QueueStats:       riverQueue,
		AccountLoadStore: postgresRuntimeStore,
		ConcurrencyStore: postgresRuntimeStore,
		CancelStore:      postgresRuntimeStore,
		MonitorStore:     riverQueue,
	}, accounts.AccountServiceOptions{
		TaskEvents: taskEvents,
	})
	riverTaskWorker := worker.NewRiverTaskWorker(worker.RiverTaskWorkerDeps{
		AccountRepo:          repos.accountRepo,
		ProviderEndpointRepo: repos.providerEndpointRepo,
		TaskRepo:             repos.taskRepo,
		Enqueuer:             riverQueue,
		Assigner:             postgresAssigner,
		CancelStore:          postgresRuntimeStore,
		MediaReader:          runtimeTaskMediaStore,
		ResultMaterializer:   runtimeTaskMediaStore,
		TaskEvents:           taskEvents,
		PublicURLs:           services.runtimeSettings,
	})
	riverTaskWorker.SetNoCapacityRetryDelay(time.Duration(cfg.Queue.NoCapacityRetryDelaySeconds) * time.Second)
	riverClient, err := worker.NewRiverClient(dbs.riverPool, riverTaskWorker, worker.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: cfg.Queue.WorkerConcurrency,
		JobTimeout:  time.Duration(cfg.Queue.TaskTimeoutSeconds) * time.Second,
		MaxRetry:    cfg.Queue.MaxRetry,
	})
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	riverQueue.SetClient(riverClient)

	submissionStore := tasksubmission.NewStore(dbs.riverPool, repos.taskRepo, riverClient, taskqueue.RiverQueueDefault, cfg.Queue.MaxRetry)
	taskService := taskservice.NewService(taskservice.ServiceDeps{
		AccountRepo:       repos.accountRepo,
		TaskRepo:          repos.taskRepo,
		SubmissionStore:   submissionStore,
		CancelStore:       postgresRuntimeStore,
		FeaturePrompts:    repos.featurePromptRepo,
		MediaValidator:    submitMediaValidator,
		TaskOutputCleaner: runtimeTaskMediaStore,
	}, taskservice.ServiceOptions{
		EventHub:   taskEventHub,
		TaskEvents: taskEvents,
	})

	return appTaskServices{
			accountService:   accountService,
			taskService:      taskService,
			taskStatsService: taskstats.NewService(repos.taskRepo),
		}, appWorkerRuntime{
			riverClient: riverClient,
			eventBus:    eventBus,
			queueName:   taskqueue.RiverQueueDefault,
			concurrency: cfg.Queue.WorkerConcurrency,
		}, nil
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
	return board.OwnerUserID, nil
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
