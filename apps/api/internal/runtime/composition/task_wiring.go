package composition

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	identityreconciliation "ideaboard/api/internal/identity/users/application/reconciliation"
	"ideaboard/api/internal/identity/users/ports"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
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
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
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
	workspaceTaskQueue *taskadapters.WorkspaceTaskEnqueuer
	// The workspace settlement pieces below are built ONCE here and shared
	// with both the HTTP submit path (http_wiring.go) and the workspace
	// worker (#825). Sharing is load-bearing for the holder: it maps
	// hold_transaction_id -> userID process-locally at Hold time, so a
	// second holder instance would fail every worker Capture/Release with
	// "unknown hold".
	workspaceTaskStore  *taskpostgres.Store
	workspaceTaskHolder *taskadapters.WorkspaceTaskHolder
	workspaceTaskQuoter *taskadapters.WorkspaceTaskQuoter
	workspaceTaskMedia  *workspacemedia.Service
	workspaceTaskAssets *workspacemedia.TaskAssetAdapter
	workspaceTaskRouter *routing.RoutingResolver
	// webhooks owns signed webhook subscriptions + River-driven
	// delivery (issue #833). Built here where the riverClient exists
	// so the same store instance serves HTTP views and the worker;
	// consumed by http_wiring.go.
	webhooks *events.WebhookStore
	// workspaceEvents is the shared per-workspace envelope hot log (SSE
	// replay window + live fanout). It is built HERE, not lazily inside
	// the v1 handler normalization, so the Control Plane's admin-cancel
	// adapter appends to the SAME window the SSE endpoints read: a second
	// process-local log would make operator cancels invisible to live
	// subscribers and to replay.
	workspaceEvents *events.WorkspaceEventLog
	// workspaceEventBus is the shared workspace-envelope NATS bus. It is
	// exposed to the Control Plane adapter so admin transitions fan out across
	// instances through the same bus as user/worker transitions.
	workspaceEventBus *events.NATSJetStreamEventBus
	// taskStatusPublisher is the ONE publisher wired to the task status
	// channel (local hubs + NATS JetStream), built by
	// eventinterfaces.NewTaskStatusEventPublisher above. The Control Plane
	// admin-cancel adapter consumes this same instance. Building a second
	// publisher for the control path would open a second NATS connection
	// and a second hub broadcast, re-introducing the double delivery this
	// wiring exists to prevent.
	taskStatusPublisher events.TaskStatusPublisher
}

// settlerEventDelivery is the shared workspace envelope fanout used by the
// worker bridge. The task status channel remains a separate compatibility
// stream; workspace events are what the v1 SSE and webhook surfaces consume.
type settlerEventDelivery struct {
	log      *events.WorkspaceEventLog
	bus      *events.NATSJetStreamEventBus
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
	//
	// The returned publisher is the ONLY way task.status_changed reaches
	// the hubs; it used to be dropped here, so nothing ever called
	// PublishTaskStatus and the whole event channel was dead. It is wired
	// into the worker settler bridge below. The v1 HTTP submit/cancel edges
	// deliberately do NOT route through it: they mint workspace envelopes
	// from their own per-request Service, so no transition is emitted
	// twice.
	taskStatusPublisher, eventBus, err := eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub, nil, userEventHub)
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}
	// These are created before the worker bridge so worker settlements and
	// HTTP/control transitions share the same workspace envelope log and
	// delivery infrastructure.
	workspaceEvents := events.NewWorkspaceEventLog()
	webhookStore := events.NewWebhookStore(events.NewHTTPSender(), nil)
	webhookStore.SetPersistence(events.NewPostgresWebhookPersistence(dbs.db))

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
	workspaceHolder := taskadapters.NewWorkspaceTaskHolder(services.creditService)
	workspaceQuoter := &taskadapters.WorkspaceTaskQuoter{Catalog: services.billingCatalog}
	workspaceMedia := workspacemedia.NewService(dbs.db)
	workspaceAssets := &workspacemedia.TaskAssetAdapter{Store: workspaceMedia}
	workspaceEnqueuer := taskadapters.NewWorkspaceTaskEnqueuer(nil, taskqueue.RiverQueueDefault, taskqueue.RiverMaxAttemptsDefault)
	workspaceRouter := routing.NewRoutingResolver(catalog.NewRepository(dbs.db))
	workspaceSettler := workspacetasks.NewService(workspacetasks.ServiceDeps{
		TaskRead: workspaceStore, TaskLifecycle: workspaceStore,
		TaskBilling: workspaceStore,
		Quoter:      workspaceQuoter, Holder: workspaceHolder,
		Queue: workspaceEnqueuer, Assets: workspaceAssets, Resolver: workspaceRouter,
	})
	// The worker settles terminal transitions through the bridge, not the
	// bare Service: the Service only STAGES status events in a per-call
	// buffer, and the worker had no drain point, so task.completed /
	// task.failed / task.pending were never published. The bridge flushes
	// only after the settlement call returns, which is after its
	// transaction committed.
	workspaceSettlerBridge := newSettlerEventBridge(workspaceSettler, taskStatusPublisher, settlerEventDelivery{
		log: workspaceEvents, bus: eventBus, webhooks: webhookStore,
	})
	workspaceMediaResolver := taskruntime.NewWorkspaceTaskRuntimeMediaResolver(workspaceMedia, services.storageService)
	// Live dispatch finalize seam: provider image bytes land directly in
	// workspace media assets (never legacy user storage), so Generate
	// yields workspace asset refs for the renderer + atomic Complete.
	// Credentials still resolve per execution inside gatewayFactory; a
	// missing gateway configuration fails tasks deterministically before
	// this materializer ever runs.
	workspaceProviderMaterializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media: workspaceMedia, Objects: services.storageService,
		Fetcher:     taskruntime.WorkspaceTaskResultBytesFetcher{},
		Derivatives: taskruntime.WorkspaceTaskDerivativeRenderer{},
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace provider result materializer: %w", err)
	}
	workspaceGateway := providers.NewExecutor(workspaceMediaResolver, workspaceProviderMaterializer, cfg.PublicOrigins, gatewayFactory, modelRoutes)
	// #827 (#809 Phase 4b-03): result renderer ingests provider outputs
	// into workspace-scoped media assets before the atomic Complete. Both
	// byte seams are production implementations: remote downloads retain
	// the outbound allowlist/timeout/size policy and derivatives are
	// best-effort bounded PNG renders.
	workspaceRenderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media:       workspaceMedia,
		Assets:      workspaceAssets,
		Objects:     services.storageService,
		Fetcher:     taskruntime.WorkspaceTaskResultBytesFetcher{},
		Derivatives: taskruntime.WorkspaceTaskDerivativeRenderer{},
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task renderer: %w", err)
	}
	// #828: post-Complete auto-materialize into the workspace library Quick
	// Saves folder. The trigger is unbilled (permanent free tier): the
	// saver ensures the Quick Saves asset and the trigger records the
	// durable job directly, with no quote, hold, capture, or release.
	workspaceLibraryStore := librarypostgres.NewService(repos.db)
	workspaceAutoMaterializer, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: workspaceLibraryStore,
		Saver: taskruntime.NewWorkspaceTaskLibrarySaver(workspaceLibraryStore),
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task auto-materializer: %w", err)
	}
	workspaceWorker, err := taskworker.NewWorkspaceTaskWorker(taskworker.WorkspaceTaskWorkerDeps{
		Store: workspaceStore, Settler: workspaceSettlerBridge, Assets: workspaceAssets,
		Provider: taskworker.NewWorkspaceGatewayProvider(&workspaceGatewayCallerAdapter{executor: workspaceGateway}),
		Renderer: workspaceRenderer, Materializer: workspaceAutoMaterializer,
		// The postgres store is the durable attempt-fact recorder
		// (retry_layer/gateway_retry_count, #848): it also gates the
		// single bounded model fallback, which requires its persisted
		// model_fallback row.
		Attempts: workspaceStore,
		// #852: bounded model-fallback sends land in
		// ideaboard_model_fallback_total with the fallback-send
		// outcome. A nil set keeps local/test runtimes
		// uninstrumented (the worker guards the seam).
		Metrics: redMetrics,
	})
	if err != nil {
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task worker: %w", err)
	}
	creditPeriodicJobs := workerbilling.CreditReconciliationPeriodicJobs()
	paymentPeriodicJobs := workerbilling.PaymentReconciliationPeriodicJobs()
	checkoutShutdownPeriodicJobs := workerbilling.PaymentCheckoutShutdownPeriodicJobs()
	// Final-timeout settlement convergence: a worker SIGKILLed mid-attempt
	// whose River job is later discarded leaves a task running with a Credit
	// Hold nothing will ever release (the Credits stale-hold reconciler only
	// records a finding, it does not converge). The periodic sweep converges
	// those rows through the SAME settler bridge the worker uses below (task ->
	// failed + hold Release + settlement projection + a real task.failed
	// event), which is why it shares workspaceSettlerBridge instead of
	// building a second settler.
	settlementSweepPeriodicJobs := taskworker.TaskSettlementSweepPeriodicJobs()
	settlementSweepWorker := taskworker.NewTaskSettlementSweepWorker(workspaceStore, workspaceSettlerBridge)
	// Identity admin reconciliation (dry-run alerting only): a daily scan
	// pages the local users table and compares each row against the Auth
	// read-only batch endpoint, persisting one finding per drift. The local
	// row always wins future repairs; this phase never pushes anything.
	// AutoRemediate stays false (fail-closed); opening automatic repair
	// needs a dedicated opt-in switch plus repair-path tests.
	identityAdminReconciler := identityreconciliation.NewReconciler(identityreconciliation.ReconcilerDeps{
		DB:    dbs.db,
		Users: repos.userRepo,
		Auth:  services.authServiceClient,
	})
	identityAdminDispatcher, err := identityreconciliation.NewRunDispatcher(services.riverProducer)
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize identity admin reconciliation dispatcher: %w", err)
	}
	identityPeriodicJobs := identityreconciliation.PeriodicJobs()
	if err := eventinterfaces.ConfigureUserEventFanout(ctx, eventBus, userEventHub, services.paymentService); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to initialize user event fanout: %w", err)
	}
	visionBillingService := searchapplication.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, nil)
	visionBillingPeriodicJobs := visionbilling.VisionBillingPeriodicJobs()
	// #833: the webhook store was built above so the River worker and the HTTP
	// views share one instance. The enqueuer attaches after NewRiverClient
	// below (SetEnqueuer), mirroring the workspaceEnqueuer SetClient seam.
	if err := webhookStore.LoadFromPersistence(ctx); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return appTaskServices{}, appWorkerRuntime{}, fmt.Errorf("failed to load webhook subscriptions: %w", err)
	}
	// #809 hard cut: the legacy task-retention periodic jobs are dropped
	// with the retention service (no legacy store left to purge against).
	// Workspace task lifecycle retention is #809-close work.
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(visionBillingPeriodicJobs)+len(settlementSweepPeriodicJobs)+len(identityPeriodicJobs))
	periodicJobs = append(periodicJobs, creditPeriodicJobs...)
	periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
	periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
	periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
	periodicJobs = append(periodicJobs, settlementSweepPeriodicJobs...)
	periodicJobs = append(periodicJobs, identityPeriodicJobs...)
	// #809 hard cut: the legacy River task worker (old job kinds) is
	// gone (see workerriver.NewRiverClient). Stray old-kind jobs
	// fail loudly as unknown kinds; the workspace_task_execute consumer
	// and all periodic workers ride this same client.
	riverClient, err := workerriver.NewRiverClient(dbs.riverPool, workerriver.RiverServerOptions{
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
			// Final-timeout settlement sweep: one serial maintenance worker on
			// the shared client (same precedent as the queues above). Serial is
			// deliberate: the sweep converges abandoned holds, so overlapping
			// runs would contend on the same per-task settle transactions for no
			// throughput gain.
			taskqueue.RiverQueueTaskSettlementSweep: {MaxWorkers: 1},
			// Identity admin reconciliation: one serial maintenance worker.
			// Serial is deliberate: overlapping scans would contend on the
			// same finding fingerprints for no throughput gain.
			identityreconciliation.IdentityReconciliationQueue: {MaxWorkers: 1},
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
			// Final-timeout settlement sweep: converges abandoned Credit Holds
			// through the worker's own settler bridge.
			river.AddWorker(workers, settlementSweepWorker)
			// Identity admin reconciliation: the dry-run scan worker and its
			// schedule worker ride the same client (no new infra).
			river.AddWorker(workers, identityreconciliation.NewExecuteWorker(identityAdminReconciler))
			river.AddWorker(workers, identityreconciliation.NewScheduleWorker(identityAdminReconciler, identityAdminDispatcher))
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
		workspaceTaskRouter: workspaceRouter,
		webhooks:            webhookStore,
		workspaceEvents:     workspaceEvents,
		workspaceEventBus:   eventBus,
		taskStatusPublisher: taskStatusPublisher,
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
		taskqueue.RiverQueueTaskSettlementSweep,
		identityreconciliation.IdentityReconciliationQueue,
	}
	stats := make(map[string]redmetrics.RiverQueueStats, len(queues))
	for _, name := range queues {
		stats[name] = func(ctx context.Context) (int64, float64, error) {
			return monitor.QueueStatsFor(ctx, name)
		}
	}
	return stats
}

// settlerEventBridge is the worker's terminal-settlement seam onto the
// task status event channel.
//
// Why it exists: workspacetasks.Service STAGES a status event inside the
// settlement call (after its own commit) and hands it to whoever calls
// StagedEvents. The v1 HTTP submit and cancel edges drain it themselves;
// the River worker had no drain point, so task.completed, task.failed, and
// the requeue's task.pending were staged and thrown away. The bridge is
// that missing drain point.
//
// Why the flush MUST happen after the settlement call returns:
// publishing inside the transaction would announce a state that a rollback
// can still erase, so every subscriber (SSE hub, NATS, webhooks) could
// observe a transition that never committed. The Service only stages on
// the post-commit line, and this wrapper only publishes after the call
// returned without error, so a rolled-back settlement stages nothing and
// therefore publishes nothing.
//
// mu serializes settle-then-drain so a staged event is always attributable
// to the settlement that produced it. The River client runs several jobs
// concurrently against ONE Service instance, and the staged-event buffer is
// per-instance, so an unlocked drain could hand one job's transition to
// another. The critical section is only the short terminal write (state
// flip + Capture/Release); the expensive work (gateway send, download,
// render) happens before Complete/Fail is called.
type settlerEventBridge struct {
	inner     settlerStager
	publisher events.TaskStatusPublisher
	delivery  settlerEventDelivery
	mu        sync.Mutex
}

// settlerStager is the narrow settlement seam the bridge needs: the three
// terminal writes plus the staged-event drain. *workspacetasks.Service is
// the production implementation.
type settlerStager interface {
	Complete(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error)
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error)
	Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*workspacetasks.WorkspaceTask, error)
	StagedEvents() []workspacetasks.TaskEvent
}

var (
	_ taskworker.WorkspaceTaskWorkerSettler = (*settlerEventBridge)(nil)
	_ settlerStager                         = (*workspacetasks.Service)(nil)
)

func newSettlerEventBridge(inner settlerStager, publisher events.TaskStatusPublisher, delivery settlerEventDelivery) *settlerEventBridge {
	return &settlerEventBridge{inner: inner, publisher: publisher, delivery: delivery}
}

// Complete flips the task to completed, then publishes task.completed.
func (b *settlerEventBridge) Complete(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	completed, err := b.inner.Complete(ctx, workspaceID, taskID, outputs)
	b.flush(ctx, err)
	return completed, err
}

// Fail converges the task to failed, then publishes task.failed.
func (b *settlerEventBridge) Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	failed, err := b.inner.Fail(ctx, workspaceID, taskID, errorMsg)
	b.flush(ctx, err)
	return failed, err
}

// Retry requeues the task, then publishes task.pending for the
// running -> pending transition. It is a requeue signal, never a create
// signal: the bridge exposes no Create, so the worker can never re-emit
// the task.created the submit edge already published.
func (b *settlerEventBridge) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*workspacetasks.WorkspaceTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	retried, err := b.inner.Retry(ctx, workspaceID, taskID, retryable)
	b.flush(ctx, err)
	return retried, err
}

// flush drains the Service's staged events and publishes them. The drain
// always runs (so a failed settlement can never leak an event into the
// NEXT settlement's flush), but publication is gated on settleErr == nil:
// a rolled-back settlement must leave no trace on the event channel.
// Publish errors are warn-logged, never returned: emission is strictly
// downstream of a committed domain fact and must not fail the job.
func (b *settlerEventBridge) flush(ctx context.Context, settleErr error) {
	if b.inner == nil {
		return
	}
	staged := b.inner.StagedEvents()
	if len(staged) == 0 {
		return
	}
	if settleErr != nil {
		return
	}
	for _, event := range staged {
		statusEvent := events.TaskStatusEvent{
			Type:       events.EventTypeTaskStatusChanged,
			TaskID:     event.TaskID,
			Status:     events.TaskStatus(event.Status),
			OccurredAt: event.OccurredAt,
		}
		if err := events.PublishTaskStatusBestEffort(ctx, b.publisher, statusEvent); err != nil {
			slog.Warn("publish workspace task status event failed",
				"task_id", event.TaskID, "status", event.Status, "err", err)
		}
		b.emitWorkspaceEvent(ctx, event)
	}
}

// emitWorkspaceEvent maps the staged worker transition to the frozen v1
// envelope contract. It runs only after settlement returned successfully.
func (b *settlerEventBridge) emitWorkspaceEvent(ctx context.Context, event workspacetasks.TaskEvent) {
	if b.delivery.log == nil && b.delivery.bus == nil && b.delivery.webhooks == nil {
		return
	}
	// The frozen workspace event registry has no task.pending event. Pending
	// is the internal status word used for a requeue; task.created is reserved
	// for the original submit edge and must never be re-emitted by a retry.
	if events.TaskStatus(event.Status) == events.TaskStatusPending {
		return
	}
	typ, ok := events.TaskStatusV1Type(events.TaskStatus(event.Status))
	if !ok {
		return
	}
	envelope, err := events.MintV1Envelope(
		typ,
		event.WorkspaceID,
		events.V1Actor{Type: events.V1ActorService, ID: "workspace-task-worker"},
		events.V1Resource{Type: events.V1ResourceTask, ID: event.TaskID},
		workerWorkspaceEventData(typ, event),
	)
	if err != nil {
		slog.Warn("mint worker workspace event failed", "task_id", event.TaskID, "status", event.Status, "err", err)
		return
	}
	if b.delivery.log != nil {
		b.delivery.log.Append(envelope)
	}
	if b.delivery.bus != nil {
		if publishErr := b.delivery.bus.PublishWorkspaceEvent(ctx, envelope); publishErr != nil {
			slog.Warn("publish worker workspace event failed", "id", envelope.ID, "type", typ, "err", publishErr)
		}
	}
	if b.delivery.webhooks != nil {
		b.delivery.webhooks.Dispatch(ctx, envelope)
	}
}

func workerWorkspaceEventData(typ string, event workspacetasks.TaskEvent) any {
	switch typ {
	case events.EventV1TaskCompleted:
		return events.V1TaskCompletedData{
			TaskID: event.TaskID,
			OutputsURI: "/api/v1/workspaces/" + strings.TrimSpace(event.WorkspaceID) +
				"/tasks/" + strings.TrimSpace(event.TaskID) + "/outputs",
		}
	case events.EventV1TaskFailed:
		// TaskEvent intentionally carries no provider error text. Keep the
		// public payload stable and avoid leaking internal error messages.
		return events.V1TaskFailedData{TaskID: event.TaskID, ErrorCode: "task_failed"}
	case events.EventV1TaskRunning:
		return events.V1TaskRunningData{TaskID: event.TaskID}
	case events.EventV1TaskCancelled:
		return events.V1TaskCancelledData{TaskID: event.TaskID}
	default:
		return nil
	}
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
