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
	"ideaboard/api/internal/media/preparation"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/tasking/taskbilling"
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

type riverStartupJobCanceller struct {
	client *river.Client[pgx.Tx]
}

func (c riverStartupJobCanceller) CancelJob(ctx context.Context, jobID int64) error {
	if c.client == nil || jobID <= 0 {
		return nil
	}
	_, err := c.client.JobCancel(ctx, jobID)
	return err
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
	workerConcurrency := cfg.Worker.EffectiveConcurrency()

	taskBillingService := taskbilling.NewService(services.billingCatalog, services.creditService)
	postgresAssigner := taskqueue.NewPostgresTaskAssigner(dbs.db)
	postgresRuntimeStore := taskqueue.NewPostgresTaskRuntimeStore(dbs.db)
	taskMediaAuthorizer := storage.NewTaskMediaAuthorizer(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	runtimeTaskMediaStore := storage.NewRuntimeTaskMediaStore(services.storageService)
	generationMediaPreparer := preparation.NewService(services.storageService, repos.mediaAssetRegistry)
	submitMediaValidator := taskmedia.NewSubmitMediaValidator(cfg.PublicOrigins, taskMediaAuthorizer)
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := newTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub)
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}

	riverQueue := taskqueue.NewRiverQueue(nil, dbs.db, postgresRuntimeStore, taskqueue.RiverMaxAttemptsDefault)
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
		AccountRepo:        repos.accountRepo,
		TaskRepo:           repos.taskRepo,
		Enqueuer:           riverQueue,
		Assigner:           postgresAssigner,
		CancelStore:        postgresRuntimeStore,
		MediaReader:        runtimeTaskMediaStore,
		ResultMaterializer: runtimeTaskMediaStore,
		TaskEvents:         taskEvents,
		PublicOrigins:      cfg.PublicOrigins,
	})
	creditPeriodicJobs := worker.CreditReconciliationPeriodicJobs()
	reclamationPeriodicJobs := reclamation.PeriodicJobs()
	paymentPeriodicJobs := worker.PaymentReconciliationPeriodicJobs()
	checkoutShutdownPeriodicJobs := worker.PaymentCheckoutShutdownPeriodicJobs()
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(reclamationPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, reclamationPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	riverClient, err := worker.NewRiverClient(dbs.riverPool, riverTaskWorker, worker.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: workerConcurrency,
		AdditionalQueues: map[string]river.QueueConfig{
			worker.CreditReconciliationQueue:    {MaxWorkers: 1},
			reclamation.QueueName:               {MaxWorkers: 1},
			worker.PaymentReconciliationQueue:   {MaxWorkers: 1},
			worker.PaymentCheckoutShutdownQueue: {MaxWorkers: 1},
		},
		RegisterWorkers: func(workers *river.Workers) {
			river.AddWorker(workers, worker.NewCreditReconciliationWorker(services.creditService))
			river.AddWorker(workers, worker.NewCreditReconciliationScheduleWorker(services.creditService))
			river.AddWorker(workers, reclamation.NewReclaimWorker(dbs.db, services.storageService))
			river.AddWorker(workers, reclamation.NewScheduleWorker(dbs.db))
			river.AddWorker(workers, worker.NewPaymentReconciliationWorker(services.paymentService))
			river.AddWorker(workers, worker.NewPaymentCheckoutShutdownWorker(services.paymentService))
			river.AddWorker(workers, worker.NewPaymentCheckoutShutdownScheduleWorker(services.paymentService))
		},
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	if _, err := recoverRunningTasksAfterStartup(ctx, repos.taskRepo, riverTaskWorker, riverStartupJobCanceller{client: riverClient}); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to recover running tasks after startup: %w", err)
	}
	riverQueue.SetClient(riverClient)

	submissionStore := tasksubmission.NewStore(dbs.riverPool, repos.taskRepo, riverClient, taskqueue.RiverQueueDefault, taskqueue.RiverMaxAttemptsDefault, taskBillingService)
	taskService := taskservice.NewService(taskservice.ServiceDeps{
		TaskIntake: taskservice.NewTaskIntake(taskservice.TaskIntakeDeps{
			Accounts:      repos.accountRepo,
			Prompts:       repos.featurePromptRepo,
			Media:         submitMediaValidator,
			MediaPreparer: generationMediaPreparer,
		}),
		TaskRepo:            repos.taskRepo,
		SubmissionStore:     submissionStore,
		CancelStore:         postgresRuntimeStore,
		TaskBilling:         taskBillingService,
		AdminBillingHistory: repos.taskRepo,
		AdminTaskTrace:      repos.taskRepo,
		AdminTaskListRead:   repos.taskRepo,
		BillableItems:       services.billableItemRegistry,
		UserResolver:        taskUserResolver{repo: repos.userRepo},
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
			concurrency: workerConcurrency,
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
	slog.Info("NATS JetStream task events enabled")
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
