package composition

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/preparation"
	storageauth "ideaboard/api/internal/media/storage/application/authorization"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/application/orchestration"
	"ideaboard/api/internal/runtime/domain"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	workerretention "ideaboard/api/internal/runtime/interfaces/worker/retention"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	visionbilling "ideaboard/api/internal/runtime/interfaces/worker/visionbilling"
	runtimeports "ideaboard/api/internal/runtime/ports"
	searchapplication "ideaboard/api/internal/searchcatalog/application"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	taskretention "ideaboard/api/internal/tasking/application/retention"
	taskstats "ideaboard/api/internal/tasking/application/statistics"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskmedia "ideaboard/api/internal/tasking/ports/media"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type appTaskServices struct {
	taskService      *taskservice.Service
	taskStatsService *taskstats.Service
	userEvents       *events.UserEventHub
	monitor          *taskqueue.RiverQueue
}

type appWorkerRuntime struct {
	riverClient *river.Client[pgx.Tx]
	eventBus    *events.NATSJetStreamEventBus
	queueName   string
	concurrency int
	// queueMetrics polls River depth/lag gauges for every known queue
	// (see taskqueue.RiverRetryMatrix); nil keeps the runtime
	// uninstrumented (tests).
	queueMetrics *redmetrics.MultiRefresher
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
	if err := r.riverClient.Start(ctx); err != nil {
		return err
	}
	if r.queueMetrics == nil {
		return nil
	}
	return r.queueMetrics.Start(ctx)
}

func (r appWorkerRuntime) Stop(ctx context.Context) error {
	var stopErr error
	if r.queueMetrics != nil {
		stopErr = r.queueMetrics.Stop(ctx)
	}
	if r.riverClient == nil {
		return stopErr
	}
	if err := r.riverClient.Stop(ctx); err != nil && stopErr == nil {
		stopErr = err
	}
	return stopErr
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
	redMetrics *redmetrics.Set,
) (appTaskServices, appWorkerRuntime, error) {
	workerConcurrency := cfg.Worker.EffectiveConcurrency()

	// Fail-closed legacy task persistence (#816 Phase 3b-07): the deleted
	// postgres adapter is replaced by this stub until the workspace-scoped
	// store lands in Phase 4a. Startup recovery below is a no-op; every
	// other legacy task path returns an explicit error instead of panicking
	// on a nil store.
	legacyTasks := &legacyTaskStore{}
	taskBillingService := taskbilling.NewService(services.billingCatalog, services.creditService)
	postgresRuntimeStore := taskqueue.NewPostgresTaskRuntimeStore(dbs.db)
	taskMediaAuthorizer := storageauth.NewTaskMediaAuthorizer(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	// Legacy storage taskmedia seam removed (#815): the worker surfaces
	// explicit storage-unavailable errors when media materialization is
	// attempted without a materializer.
	generationMediaPreparer := preparation.NewService(services.storageService, repos.mediaAssetRegistry)
	submitMediaValidator := taskmedia.NewSubmitMediaValidator(cfg.PublicOrigins, taskMediaAuthorizer, repos.mediaAssetRegistry)
	userEventHub := events.NewUserEventHub()
	taskEventHub := taskservice.NewEventHub()
	taskEvents, eventBus, err := eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub, legacyTasks, userEventHub)
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}

	riverQueue := taskqueue.NewRiverQueue(nil, dbs.db, taskqueue.RiverMaxAttemptsDefault)
	aiGatewayRepo := aigateway.NewConfigRepository(repos.systemConfigRepo)
	modelRoutes := aiGatewayModelRouteReader{repo: aiGatewayRepo}
	gatewayFactory := func(ctx context.Context) (runtimeports.GatewayClient, error) {
		resolved, err := aiGatewayRepo.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		return infrastructure.NewGatewayClient(infrastructure.GatewayClientConfig{
			BaseURL: resolved.BaseURL,
			Token:   resolved.Token,
			Metrics: redMetrics,
		})
	}
	riverTaskWorker := workerriver.NewRiverTaskWorker(workerriver.RiverTaskWorkerDeps{
		TaskRepo:           legacyTasks,
		Enqueuer:           riverQueue,
		CancelStore:        postgresRuntimeStore,
		MediaReader:        nil,
		ResultMaterializer: nil,
		TaskEvents:         taskEvents,
		PublicOrigins:      cfg.PublicOrigins,
		ModelResolver:      aiGatewayRepo,
		GatewayFactory:     gatewayFactory,
		ModelRoutes:        modelRoutes,
		REDMetrics:         redMetrics,
	})
	creditPeriodicJobs := workerbilling.CreditReconciliationPeriodicJobs()
	retentionService := taskretention.NewService(legacyTasks)
	retentionPeriodicJobs := workerretention.PeriodicJobs()
	paymentPeriodicJobs := workerbilling.PaymentReconciliationPeriodicJobs()
	checkoutShutdownPeriodicJobs := workerbilling.PaymentCheckoutShutdownPeriodicJobs()
	if err := eventinterfaces.ConfigureUserEventFanout(ctx, eventBus, userEventHub, services.paymentService); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize user event fanout: %w", err)
	}
	visionBillingService := searchapplication.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, nil)
	visionBillingPeriodicJobs := visionbilling.VisionBillingPeriodicJobs()
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(retentionPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(visionBillingPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, retentionPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
	riverClient, err := workerriver.NewRiverClient(dbs.riverPool, riverTaskWorker, workerriver.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: workerConcurrency,
		AdditionalQueues: map[string]river.QueueConfig{
			workerbilling.CreditReconciliationQueue:    {MaxWorkers: 1},
			workerretention.QueueName:                  {MaxWorkers: 1},
			workerbilling.PaymentReconciliationQueue:   {MaxWorkers: 1},
			workerbilling.PaymentCheckoutShutdownQueue: {MaxWorkers: 1},
			visionbilling.VisionBillingQueue:           {MaxWorkers: 1},
		},
		RegisterWorkers: func(workers *river.Workers) {
			river.AddWorker(workers, workerbilling.NewCreditReconciliationWorker(services.creditService))
			river.AddWorker(workers, workerbilling.NewCreditReconciliationScheduleWorker(services.creditService))
			river.AddWorker(workers, workerretention.NewTaskRetentionWorker(retentionService, cfg.Worker.EffectiveTaskRetentionDays()))
			river.AddWorker(workers, workerbilling.NewPaymentReconciliationWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownScheduleWorker(services.paymentService))
			river.AddWorker(workers, visionbilling.NewVisionBillingReconciliationWorker(visionBillingService))
		},
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	recoveryCoord := orchestration.NewTaskRecoveryCoordinator(legacyTasks, riverTaskWorker, riverStartupJobCanceller{client: riverClient})
	if _, err := recoveryCoord.RecoverRunningTasks(ctx); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to recover running tasks after startup: %w", err)
	}
	riverQueue.SetClient(riverClient)

	submissionStore := tasksubmission.NewStore(
		dbs.riverPool,
		legacyTasks,
		taskqueue.NewTransactionalEnqueuer(riverClient, taskqueue.RiverQueueDefault, taskqueue.RiverMaxAttemptsDefault),
		taskBillingService,
	)
	taskService := taskservice.NewService(taskservice.ServiceDeps{
		TaskIntake: taskservice.NewTaskIntake(taskservice.TaskIntakeDeps{
			Models:         aiGatewayRepo,
			Prompts:        repos.featurePromptRepo,
			Media:          submitMediaValidator,
			MediaReadiness: repos.mediaAssetRegistry,
			MediaPreparer:  generationMediaPreparer,
			MediaManifest:  repos.mediaAssetRegistry,
		}),
		TaskRepo:            legacyTasks,
		SubmissionStore:     submissionStore,
		CancelStore:         postgresRuntimeStore,
		TaskBilling:         taskBillingService,
		AdminBillingHistory: legacyTasks,
		AdminTaskTrace:      legacyTasks,
		AdminTaskListRead:   legacyTasks,
		BillableItems:       services.billableItemRegistry,
		UserResolver:        taskUserResolver{repo: repos.userRepo},
	}, taskservice.ServiceOptions{
		TaskEvents: taskEvents,
	})

	return appTaskServices{
		taskService:      taskService,
		taskStatsService: taskstats.NewService(legacyTasks),
		userEvents:       userEventHub,
	}, appWorkerRuntime{
		riverClient:  riverClient,
		eventBus:     eventBus,
		queueName:    taskqueue.RiverQueueDefault,
		concurrency:  workerConcurrency,
		queueMetrics: redmetrics.NewMultiRefresher(redMetrics, riverQueueStatsByQueue(riverQueue), redmetrics.DefaultRefreshInterval),
	}, nil
}

// riverQueueStatsByQueue fans one RiverQueue monitor out to every known
// queue so each queue gets its own depth/lag gauge series. Queue inventory
// must track taskqueue.RiverRetryMatrix; a missing queue loses its RED
// signals (depth/lag alerts in docs/runbooks/backend-metrics.md).
func riverQueueStatsByQueue(monitor *taskqueue.RiverQueue) map[string]redmetrics.RiverQueueStats {
	queues := []string{
		taskqueue.RiverQueueDefault,
		workerbilling.CreditReconciliationQueue,
		workerbilling.PaymentReconciliationQueue,
		workerbilling.PaymentCheckoutShutdownQueue,
		workerretention.QueueName,
		visionbilling.VisionBillingQueue,
	}
	stats := make(map[string]redmetrics.RiverQueueStats, len(queues))
	for _, name := range queues {
		stats[name] = func(ctx context.Context) (int64, float64, error) {
			return monitor.QueueStatsFor(ctx, name)
		}
	}
	return stats
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

// aiGatewayModelRouteReader adapts the admin model catalog to the worker's
// route-override seam. It lives in composition (the sole assembly root) so
// runtime never imports the aigateway business context.
type aiGatewayModelRouteReader struct {
	repo *aigateway.ConfigRepository
}

func (a aiGatewayModelRouteReader) LookupRouteOverride(ctx context.Context, modelID string) (string, string, bool, error) {
	items, err := a.repo.ListCatalog(ctx)
	if err != nil {
		return "", "", false, err
	}
	for _, item := range items {
		if item.ID == modelID {
			return item.CustomPath, item.CustomParams, true, nil
		}
	}
	return "", "", false, nil
}
