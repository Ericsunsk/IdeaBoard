package composition

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/domain"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	visionbilling "ideaboard/api/internal/runtime/interfaces/worker/visionbilling"
	runtimeports "ideaboard/api/internal/runtime/ports"
	searchapplication "ideaboard/api/internal/searchcatalog/application"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type appTaskServices struct {
	// userEvents and the River queue monitor are owned locally by
	// newAppTaskRuntime (user-event fanout + queue RED metrics); HTTP
	// wiring only consumes the workspace settlement pieces below.
	// workspaceTaskQueue carries the River client for the v1 workspace
	// task surface (workspacetasks.Enqueuer). Built here where the
	// riverClient exists; consumed by http_wiring.go. Nil only when the
	// River client failed to build (startup already failed then).
	workspaceTaskQueue *workspaceTaskEnqueuer
	// The workspace settlement pieces below are built ONCE here and shared
	// with both the HTTP submit path (http_wiring.go) and the workspace
	// worker (#825). Sharing is load-bearing for the holder: it maps
	// hold_transaction_id -> userID process-locally at Hold time, so a
	// second holder instance would fail every worker Capture/Release with
	// "unknown hold".
	workspaceTaskStore  *taskpostgres.Store
	workspaceTaskHolder *workspaceTaskHolder
	workspaceTaskQuoter *workspaceTaskQuoter
	workspaceTaskMedia  *workspacemedia.Service
	workspaceTaskAssets *workspacemedia.TaskAssetAdapter
	// webhooks owns signed webhook subscriptions + River-driven
	// delivery (issue #833). Built here where the riverClient exists
	// so the same store instance serves HTTP views and the worker;
	// consumed by http_wiring.go.
	webhooks *events.WebhookStore
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

	// #809 hard cut: the fail-closed legacyTaskStore is deleted, and with
	// it every legacy-only task service (legacy River worker, retention
	// service, submission store, task/task-stats services, startup
	// recovery). The workspace settlement pieces below are the only task
	// path; the legacy River job kinds are unregistered from the client
	// (stray old-kind jobs fail loudly as unknown kinds, never silently).
	userEventHub := events.NewUserEventHub()
	taskEventHub := taskservice.NewEventHub()
	// The legacy task resolver is gone with the stub: the publisher
	// degrades gracefully (nil resolver skips enrichment and still
	// broadcasts bare task events to the local hubs and NATS).
	_, eventBus, err := eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub, nil, userEventHub)
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
	// #825 (#809 Phase 4b-01): the workspace settlement pieces are built
	// once here and shared with the HTTP submit path (see appTaskServices).
	// The enqueuer starts clientless: the settlement Service needs it at
	// construction, the River client needs the worker at registration, so
	// the client attaches after NewRiverClient below (SetClient).
	workspaceStore := taskpostgres.NewStore(dbs.db)
	workspaceHolder := newWorkspaceTaskHolder(services.creditService)
	workspaceQuoter := &workspaceTaskQuoter{catalog: services.billingCatalog}
	workspaceMedia := workspacemedia.NewService(dbs.db)
	workspaceAssets := &workspacemedia.TaskAssetAdapter{Store: workspaceMedia}
	workspaceEnqueuer := newWorkspaceTaskEnqueuer(nil, taskqueue.RiverQueueDefault, taskqueue.RiverMaxAttemptsDefault)
	workspaceSettler := workspacetasks.NewService(workspacetasks.ServiceDeps{
		Tasks: workspaceStore, Quoter: workspaceQuoter, Holder: workspaceHolder,
		Queue: workspaceEnqueuer, Assets: workspaceAssets,
	})
	workspaceMediaResolver := newWorkspaceTaskRuntimeMediaResolver(workspaceMedia, services.storageService)
	workspaceGateway := providers.NewExecutor(workspaceMediaResolver, nil, cfg.PublicOrigins, gatewayFactory, modelRoutes)
	// #827 (#809 Phase 4b-03): result renderer ingests provider outputs
	// into workspace-scoped media assets before the atomic Complete. Both
	// byte seams are production implementations: remote downloads retain
	// the outbound allowlist/timeout/size policy and derivatives are
	// best-effort bounded PNG renders.
	workspaceRenderer, err := NewWorkspaceTaskRenderer(WorkspaceTaskRendererDeps{
		Media:       workspaceMedia,
		Assets:      workspaceAssets,
		Objects:     services.storageService,
		Fetcher:     workspaceTaskResultBytesFetcher{},
		Derivatives: workspaceTaskDerivativeRenderer{},
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task renderer: %w", err)
	}
	// #828: post-Complete auto-materialize into the workspace library Quick
	// Saves folder. Managed task output refs are represented explicitly by
	// library.Service (workspace_media + media_asset_id), so no provider URL
	// is fabricated for internal media.
	workspaceLibraryStore := library.NewService(repos.db)
	workspaceAutoMaterializer, err := NewWorkspaceTaskAutoMaterializer(WorkspaceTaskAutoMaterializerDeps{
		Store:   workspaceLibraryStore,
		Quoter:  newLibraryMaterializeQuoter(services.billingCatalog),
		Billing: newLibraryMaterializeSettler(services.creditService),
		Saver:   newWorkspaceTaskLibrarySaver(workspaceLibraryStore),
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task auto-materializer: %w", err)
	}
	workspaceWorker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: workspaceStore, Settler: workspaceSettler, Assets: workspaceAssets,
		Provider: newWorkspaceGatewayProvider(aiGatewayRepo, workspaceGateway),
		Renderer: workspaceRenderer, Materializer: workspaceAutoMaterializer,
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task worker: %w", err)
	}
	creditPeriodicJobs := workerbilling.CreditReconciliationPeriodicJobs()
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
	// #833: the webhook store is built here (not in HTTP normalize) so
	// the River worker and the HTTP views share one instance. The
	// enqueuer attaches after NewRiverClient below (SetEnqueuer),
	// mirroring the workspaceEnqueuer SetClient seam.
	webhookStore := events.NewWebhookStore(events.NewHTTPSender(), nil)
	webhookStore.SetPersistence(events.NewPostgresWebhookPersistence(dbs.db))
	if err := webhookStore.LoadFromPersistence(ctx); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to load webhook subscriptions: %w", err)
	}
	// #809 hard cut: the legacy task-retention periodic jobs are dropped
	// with the retention service (no legacy store left to purge against).
	// Workspace task lifecycle retention is #809-close work.
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(visionBillingPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
	// #809 hard cut: the legacy River task worker (old job kinds) is
	// unregistered with the stub (nil worker below). Stray old-kind jobs
	// fail loudly as unknown kinds; the workspace_task_execute consumer
	// and all periodic workers ride this same client.
	riverClient, err := workerriver.NewRiverClient(dbs.riverPool, nil, workerriver.RiverServerOptions{
		QueueName:   taskqueue.RiverQueueDefault,
		Concurrency: workerConcurrency,
		AdditionalQueues: map[string]river.QueueConfig{
			workerbilling.CreditReconciliationQueue:    {MaxWorkers: 1},
			workerbilling.PaymentReconciliationQueue:   {MaxWorkers: 1},
			workerbilling.PaymentCheckoutShutdownQueue: {MaxWorkers: 1},
			visionbilling.VisionBillingQueue:           {MaxWorkers: 1},
			// #833: signed webhook deliveries ride their own queue with
			// the shared concurrency config (no new infra); serial
			// workers match the maintenance-queue precedent and the 1m
			// per-attempt timeout bounds head-of-line blocking.
			taskqueue.RiverQueueWebhookDeliver: {MaxWorkers: 1},
		},
		RegisterWorkers: func(workers *river.Workers) {
			river.AddWorker(workers, workerbilling.NewCreditReconciliationWorker(services.creditService))
			river.AddWorker(workers, workerbilling.NewCreditReconciliationScheduleWorker(services.creditService))
			river.AddWorker(workers, workerbilling.NewPaymentReconciliationWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownWorker(services.paymentService))
			river.AddWorker(workers, workerbilling.NewPaymentCheckoutShutdownScheduleWorker(services.paymentService))
			river.AddWorker(workers, visionbilling.NewVisionBillingReconciliationWorker(visionBillingService))
			// #825: the real workspace_task_execute consumer rides the
			// shared client/queue/concurrency (no new infra).
			river.AddWorker(workers, workspaceWorker)
			// #833: the webhook delivery consumer rides the same client.
			river.AddWorker(workers, events.NewWebhookDeliverWorker(webhookStore))
		},
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	// #809 hard cut: legacy startup recovery was a no-op against the
	// stub (no legacy running rows left to converge). Workspace recovery
	// is River redelivery + lease-guarded claim (#825).
	riverQueue.SetClient(riverClient)
	workspaceEnqueuer.SetClient(riverClient)
	webhookStore.SetEnqueuer(riverClient)
	if err := webhookStore.ReconcilePending(ctx); err != nil {
		_ = riverClient.Stop(ctx)
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to reconcile pending webhooks: %w", err)
	}

	return appTaskServices{
		// #821 (#809 Phase 4-04): v1 workspace task enqueue rides the
		// production River client (workspace_task_execute kind); #825
		// registers the real consumer on the same client above.
		workspaceTaskQueue:  workspaceEnqueuer,
		workspaceTaskStore:  workspaceStore,
		workspaceTaskHolder: workspaceHolder,
		workspaceTaskQuoter: workspaceQuoter,
		workspaceTaskMedia:  workspaceMedia,
		workspaceTaskAssets: workspaceAssets,
		webhooks:            webhookStore,
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
// The legacy task-retention queue left with the #809 hard cut.
func riverQueueStatsByQueue(monitor *taskqueue.RiverQueue) map[string]redmetrics.RiverQueueStats {
	queues := []string{
		taskqueue.RiverQueueDefault,
		workerbilling.CreditReconciliationQueue,
		workerbilling.PaymentReconciliationQueue,
		workerbilling.PaymentCheckoutShutdownQueue,
		visionbilling.VisionBillingQueue,
		taskqueue.RiverQueueWebhookDeliver,
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
