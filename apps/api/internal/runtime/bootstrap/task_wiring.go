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
	"ideaboard/api/internal/media/derivatives"
	"ideaboard/api/internal/media/preparation"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
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
	userEvents       *events.UserEventHub
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
	runtimeTaskMediaStore := storage.NewRuntimeTaskMediaStore(services.storageService, repos.mediaAssetRegistry)
	generationMediaPreparer := preparation.NewService(services.storageService, repos.mediaAssetRegistry)
	submitMediaValidator := taskmedia.NewSubmitMediaValidator(cfg.PublicOrigins, taskMediaAuthorizer)
	userEventHub := events.NewUserEventHub()
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := newTaskStatusEventPublisherWithResolver(ctx, cfg.Events, taskEventHub, repos.taskRepo, userEventHub)
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
	mediaDerivativeEvents := derivatives.NewUserEventPublisher(userEventHub)
	mediaDerivativeService := derivatives.NewService(dbs.db, services.storageService, mediaDerivativeEvents)
	if err := configureUserEventFanout(ctx, eventBus, userEventHub, services.paymentService.EventHub()); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize user event fanout: %w", err)
	}
	mediaDerivativePeriodicJobs := derivatives.PeriodicJobs()
	visionBillingService := searchcatalog.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, nil)
	visionBillingPeriodicJobs := searchcatalog.VisionBillingPeriodicJobs()
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(reclamationPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(mediaDerivativePeriodicJobs)+len(visionBillingPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, reclamationPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	periodicJobs = append(periodicJobs, mediaDerivativePeriodicJobs...)
	periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
	riverClient, err := worker.NewRiverClient(dbs.riverPool, riverTaskWorker, worker.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: workerConcurrency,
		AdditionalQueues: map[string]river.QueueConfig{
			worker.CreditReconciliationQueue:    {MaxWorkers: 1},
			reclamation.QueueName:               {MaxWorkers: 1},
			worker.PaymentReconciliationQueue:   {MaxWorkers: 1},
			worker.PaymentCheckoutShutdownQueue: {MaxWorkers: 1},
			derivatives.QueueName:               {MaxWorkers: 2},
			searchcatalog.VisionBillingQueue:    {MaxWorkers: 1},
		},
		RegisterWorkers: func(workers *river.Workers) {
			river.AddWorker(workers, worker.NewCreditReconciliationWorker(services.creditService))
			river.AddWorker(workers, worker.NewCreditReconciliationScheduleWorker(services.creditService))
			river.AddWorker(workers, reclamation.NewReclaimWorker(dbs.db, services.storageService))
			river.AddWorker(workers, reclamation.NewScheduleWorker(dbs.db))
			river.AddWorker(workers, worker.NewPaymentReconciliationWorker(services.paymentService))
			river.AddWorker(workers, worker.NewPaymentCheckoutShutdownWorker(services.paymentService))
			river.AddWorker(workers, worker.NewPaymentCheckoutShutdownScheduleWorker(services.paymentService))
			river.AddWorker(workers, derivatives.NewWorker(mediaDerivativeService))
			river.AddWorker(workers, derivatives.NewOutboxDispatcher(dbs.db, services.storageService))
			river.AddWorker(workers, searchcatalog.NewVisionBillingReconciliationWorker(visionBillingService))
		},
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	repos.mediaAssetRegistry.SetDerivativeEnqueuer(func(ctx context.Context, derivativeID int64) error {
		_, err := riverClient.Insert(ctx, derivatives.JobArgs{DerivativeID: derivativeID}, &river.InsertOpts{
			Queue: derivatives.QueueName, MaxAttempts: 5, Priority: 4,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
		return err
	})
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
			userEvents:       userEventHub,
		}, appWorkerRuntime{
			riverClient: riverClient,
			eventBus:    eventBus,
			queueName:   taskqueue.RiverQueueDefault,
			concurrency: workerConcurrency,
		}, nil
}

func configureUserEventFanout(ctx context.Context, bus *events.NATSJetStreamEventBus, userHub *events.UserEventHub, paymentHub *payments.EventHub) error {
	if userHub == nil || paymentHub == nil {
		return fmt.Errorf("user event fanout requires user and payment hubs")
	}
	paymentHub.SetUserEventHub(userHub)
	if bus != nil {
		userHub.SetRemotePublisher(func(event events.UserEvent) {
			publishCtx, cancel := context.WithTimeout(context.Background(), events.DefaultPublishTimeout)
			defer cancel()
			if err := bus.PublishUserEvent(publishCtx, event); err != nil {
				slog.Warn("publish user event failed", "type", event.Type, "user_id", event.UserID, "err", err)
			}
		})
	}
	if bus == nil {
		return nil
	}
	return bus.SubscribeUserEvents(ctx, func(_ context.Context, event events.UserEvent) error {
		userHub.PublishRemote(event)
		return nil
	})
}

type taskEventHubPublisher struct {
	hub      *taskservice.EventHub
	userHub  *events.UserEventHub
	resolver interface {
		GetByTaskID(context.Context, string) (*tasks.Task, error)
	}
}

func (p taskEventHubPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	if p.hub == nil {
		return nil
	}
	event = events.NormalizeTaskStatusEvent(event)
	taskEvent := taskservice.TaskEvent{TaskID: event.TaskID, Status: event.Status}
	if p.resolver != nil {
		if task, err := p.resolver.GetByTaskID(ctx, event.TaskID); err == nil && task != nil {
			p.hub.Publish(event.TaskID, taskEvent)
			if p.userHub != nil {
				userEvent, eventErr := events.NewUserEvent(task.UserID, events.EventTypeTaskUpdated, taskEvent)
				if eventErr != nil {
					return eventErr
				}
				userEvent.OccurredAt = event.OccurredAt
				p.userHub.PublishRemote(userEvent)
			}
			return nil
		}
	}
	p.hub.Publish(event.TaskID, taskEvent)
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
	return newTaskStatusEventPublisherWithResolver(ctx, cfg, hub, nil)
}

func newTaskStatusEventPublisherWithResolver(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
	resolver interface {
		GetByTaskID(context.Context, string) (*tasks.Task, error)
	},
	userHubs ...*events.UserEventHub,
) (events.TaskStatusPublisher, *events.NATSJetStreamEventBus, error) {
	var userHub *events.UserEventHub
	if len(userHubs) > 0 {
		userHub = userHubs[0]
	}
	localPublisher := taskEventHubPublisher{hub: hub, userHub: userHub, resolver: resolver}
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
	remotePublisher := taskEventHubPublisher{hub: hub, userHub: userHub, resolver: resolver}
	if err := bus.SubscribeTaskStatus(ctx, func(ctx context.Context, event events.TaskStatusEvent) error {
		return remotePublisher.PublishTaskStatus(ctx, event)
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
