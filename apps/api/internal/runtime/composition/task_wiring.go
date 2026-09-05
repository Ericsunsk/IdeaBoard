package composition

import (
	"context"
	"fmt"
	"log/slog"

	accountapp "ideaboard/api/internal/accountpool/accounts/application"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/users/ports"
	derivativeprocessing "ideaboard/api/internal/media/derivatives/application/processing"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	derivativeevents "ideaboard/api/internal/media/derivatives/infrastructure/events"
	derivativerendering "ideaboard/api/internal/media/derivatives/infrastructure/rendering"
	derivativeworker "ideaboard/api/internal/media/derivatives/infrastructure/worker"
	"ideaboard/api/internal/media/preparation"
	"ideaboard/api/internal/media/reclamation"
	storageauth "ideaboard/api/internal/media/storage/application/authorization"
	"ideaboard/api/internal/media/storage/application/integrity"
	storagetaskmedia "ideaboard/api/internal/media/storage/application/taskmedia"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/application/orchestration"
	"ideaboard/api/internal/runtime/domain"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	runtimeports "ideaboard/api/internal/runtime/ports"
	"ideaboard/api/internal/searchcatalog"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	taskstats "ideaboard/api/internal/tasking/application/statistics"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskmedia "ideaboard/api/internal/tasking/ports/media"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type appTaskServices struct {
	accountService   *accountapp.Service
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

func (r appWorkerRuntime) Name() string {
	return "river-worker"
}

func (r appWorkerRuntime) Kind() domain.ComponentKind {
	return domain.ComponentKindRiver
}

func (r appWorkerRuntime) Start(ctx context.Context) error {
	if r.riverClient == nil {
		return nil
	}
	slog.Info("River worker starting", "queue", r.queueName, "concurrency", r.concurrency)
	return r.riverClient.Start(ctx)
}

func (r appWorkerRuntime) Stop(ctx context.Context) error {
	if r.riverClient == nil {
		return nil
	}
	return r.riverClient.Stop(ctx)
}

func (r appWorkerRuntime) CheckHealth(context.Context) domain.ComponentStatus {
	return domain.ComponentStatus{Name: r.Name(), Kind: r.Kind(), Ready: r.riverClient != nil, Required: true}
}

var _ runtimeports.HealthChecker = appWorkerRuntime{}

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
	taskMediaAuthorizer := storageauth.NewTaskMediaAuthorizer(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	runtimeTaskMediaStore := storagetaskmedia.NewRuntimeTaskMediaStore(services.storageService, repos.mediaAssetRegistry)
	generationMediaPreparer := preparation.NewService(services.storageService, repos.mediaAssetRegistry)
	submitMediaValidator := taskmedia.NewSubmitMediaValidator(cfg.PublicOrigins, taskMediaAuthorizer, repos.mediaAssetRegistry)
	userEventHub := events.NewUserEventHub()
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub, repos.taskRepo, userEventHub)
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}

	riverQueue := taskqueue.NewRiverQueue(nil, dbs.db, postgresRuntimeStore, taskqueue.RiverMaxAttemptsDefault)
	accountService := accountapp.NewService(accountapp.ServiceDeps{
		AccountRepo:      repos.accountRepo,
		TaskRepo:         repos.taskRepo.Queries(),
		QueueStats:       riverQueue,
		AccountLoadStore: postgresRuntimeStore,
		ConcurrencyStore: postgresRuntimeStore,
		CancelStore:      postgresRuntimeStore,
		MonitorStore:     riverQueue,
	}, accountapp.ServiceOptions{
		TaskEvents: taskEvents,
	})
	riverTaskWorker := workerriver.NewRiverTaskWorker(workerriver.RiverTaskWorkerDeps{
		AccountRepo:        repos.accountRepo,
		TaskRepo:           repos.taskRepo.Runtime(),
		Enqueuer:           riverQueue,
		Assigner:           postgresAssigner,
		CancelStore:        postgresRuntimeStore,
		MediaReader:        runtimeTaskMediaStore,
		ResultMaterializer: runtimeTaskMediaStore,
		TaskEvents:         taskEvents,
		PublicOrigins:      cfg.PublicOrigins,
	})
	creditPeriodicJobs := workerbilling.CreditReconciliationPeriodicJobs()
	reclamationPeriodicJobs := reclamation.PeriodicJobs()
	paymentPeriodicJobs := workerbilling.PaymentReconciliationPeriodicJobs()
	checkoutShutdownPeriodicJobs := workerbilling.PaymentCheckoutShutdownPeriodicJobs()
	mediaDerivativeEvents := derivativeevents.NewUserEventPublisher(userEventHub)
	mediaDerivativeService := derivativeprocessing.NewService(
		dbs.db,
		services.storageService,
		integrity.NewIntegrityVerifier(services.storageService, integrity.IntegrityConfig{MaxBytes: derivativedomain.MaxOutputBytes + 1}),
		derivativerendering.NewRenderer(),
		mediaDerivativeEvents,
	)
	if err := eventinterfaces.ConfigureUserEventFanout(ctx, eventBus, userEventHub, services.paymentService); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize user event fanout: %w", err)
	}
	mediaDerivativePeriodicJobs := derivativeworker.PeriodicJobs()
	visionBillingService := searchcatalog.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, nil)
	visionBillingPeriodicJobs := searchcatalog.VisionBillingPeriodicJobs()
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(reclamationPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(mediaDerivativePeriodicJobs)+len(visionBillingPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, reclamationPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	periodicJobs = append(periodicJobs, mediaDerivativePeriodicJobs...)
	periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
	derivativeOutboxDispatcher := derivativeworker.NewOutboxDispatcher(
		dbs.db,
		services.storageService,
		integrity.NewIntegrityVerifier(services.storageService, integrity.IntegrityConfig{}),
	)
	derivativeOutboxDispatcher.SetReconciler(mediaDerivativeEvents)
	riverClient, err := workerriver.NewRiverClient(dbs.riverPool, riverTaskWorker, workerriver.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: workerConcurrency,
		AdditionalQueues: map[string]river.QueueConfig{
			workerbilling.CreditReconciliationQueue:    {MaxWorkers: 1},
			reclamation.QueueName:                      {MaxWorkers: 1},
			workerbilling.PaymentReconciliationQueue:   {MaxWorkers: 1},
			workerbilling.PaymentCheckoutShutdownQueue: {MaxWorkers: 1},
			derivativeworker.QueueName:                 {MaxWorkers: 2},
			searchcatalog.VisionBillingQueue:           {MaxWorkers: 1},
		},
		RegisterWorkers: func(workers *river.Workers) {
			river.AddWorker(workers, workerbilling.NewCreditReconciliationWorker(services.creditService))
			river.AddWorker(workers, workerbilling.NewCreditReconciliationScheduleWorker(services.creditService))
			river.AddWorker(workers, reclamation.NewReclaimWorker(dbs.db, services.storageService))
			river.AddWorker(workers, reclamation.NewScheduleWorker(dbs.db))
			river.AddWorker(workers, workerbilling.NewPaymentReconciliationWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownScheduleWorker(services.paymentService))
			river.AddWorker(workers, derivativeworker.NewWorker(mediaDerivativeService))
			river.AddWorker(workers, derivativeOutboxDispatcher)
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
		_, err := riverClient.Insert(ctx, derivativeworker.JobArgs{DerivativeID: derivativeID}, derivativeworker.DerivativeJobInsertOpts())
		return err
	})
	recoveryCoord := orchestration.NewTaskRecoveryCoordinator(repos.taskRepo, riverTaskWorker, riverStartupJobCanceller{client: riverClient})
	if _, err := recoveryCoord.RecoverRunningTasks(ctx); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to recover running tasks after startup: %w", err)
	}
	riverQueue.SetClient(riverClient)

	submissionStore := tasksubmission.NewStore(
		dbs.riverPool,
		repos.taskRepo.Submission(),
		taskqueue.NewTransactionalEnqueuer(riverClient, taskqueue.RiverQueueDefault, taskqueue.RiverMaxAttemptsDefault),
		taskBillingService,
	)
	taskService := taskservice.NewService(taskservice.ServiceDeps{
		TaskIntake: taskservice.NewTaskIntake(taskservice.TaskIntakeDeps{
			Accounts:       repos.accountRepo,
			Prompts:        repos.featurePromptRepo,
			Media:          submitMediaValidator,
			MediaReadiness: repos.mediaAssetRegistry,
			MediaPreparer:  generationMediaPreparer,
			MediaManifest:  repos.mediaAssetRegistry,
		}),
		TaskRepo:            repos.taskRepo.Queries(),
		SubmissionStore:     submissionStore,
		CancelStore:         postgresRuntimeStore,
		TaskBilling:         taskBillingService,
		AdminBillingHistory: repos.taskRepo.Billing(),
		AdminTaskTrace:      repos.taskRepo.Trace(),
		AdminTaskListRead:   repos.taskRepo.Trace(),
		BillableItems:       services.billableItemRegistry,
		UserResolver:        taskUserResolver{repo: repos.userRepo},
	}, taskservice.ServiceOptions{
		TaskEvents: taskEvents,
	})

	return appTaskServices{
			accountService:   accountService,
			taskService:      taskService,
			taskStatsService: taskstats.NewService(repos.taskRepo.Statistics()),
			userEvents:       userEventHub,
		}, appWorkerRuntime{
			riverClient: riverClient,
			eventBus:    eventBus,
			queueName:   taskqueue.RiverQueueDefault,
			concurrency: workerConcurrency,
		}, nil
}

type uploadBoardAccessResolver struct {
	service *boards.Service
}

func (r uploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.OwnerUserID, nil
}

type uploadOwnerStorageUIDResolver struct {
	repo ports.UserRepository
}

func (r uploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}
