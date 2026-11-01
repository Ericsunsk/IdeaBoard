// Wiring segment 3/4: task runtime and workers (assembly only).
//
// This file only annotates and composes: it builds the shared workspace
// settlement pieces, the River client/workers, and the event fanout once
// and shares the instances with the HTTP submit path. Settlement, retry,
// and rendering rules live in tasking/worker providers; nothing here writes
// business facts beyond assembly.
package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	eventoutbox "ideaboard/api/internal/events/outbox"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	identityreconciliation "ideaboard/api/internal/identity/users/application/reconciliation"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
	"ideaboard/api/internal/media/workertransfer"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/memberquotas"
	modelregistrydomain "ideaboard/api/internal/modelregistry/domain"
	modelregistryinfrastructure "ideaboard/api/internal/modelregistry/infrastructure"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/runtime/domain"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	workerbilling "ideaboard/api/internal/runtime/interfaces/worker/billing"
	outboxworker "ideaboard/api/internal/runtime/interfaces/worker/outbox"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	providerinfra "ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	visionbilling "ideaboard/api/internal/runtime/interfaces/worker/visionbilling"
	runtimeports "ideaboard/api/internal/runtime/ports"
	visionapplication "ideaboard/api/internal/searchcatalog/application/vision"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	tasks "ideaboard/api/internal/tasking/domain"
	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type AppTaskServices struct {
	invitationMailQueue *invitations.MailQueue
	// userEvents and the River queue monitor are owned locally by
	// NewAppTaskRuntime (user-event fanout + queue RED metrics); HTTP
	// wiring only consumes the workspace settlement pieces below.
	// workspaceTaskQueue carries the River client for the v1 workspace
	// task surface (tasks.TxEnqueuer). Built here where the
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
	webhooks *eventwebhook.WebhookStore
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
	workspaceEventBus *eventnats.NATSJetStreamEventBus
	// userEvents is the process-local recipient-keyed fanout consumed by the
	// per-user notification SSE stream and the payments hint publisher. It
	// is built HERE (not lazily) so HTTP emission and the NATS remote
	// fanout share one hub instance.
	userEvents *events.UserEventHub
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
	bus      *eventnats.NATSJetStreamEventBus
	webhooks *eventwebhook.WebhookStore
}

type AppWorkerRuntime struct {
	riverClient *river.Client[pgx.Tx]
	eventBus    *eventnats.NATSJetStreamEventBus
	queueName   string
	concurrency int
	// consume reports whether this process starts the River client. An
	// http-only process (ADR-0021) keeps the client for transactional
	// enqueue but never consumes: no workers are registered and Start
	// leaves the client stopped, so queued jobs wait for a worker process.
	consume bool
	// queueMetrics polls River depth/lag gauges for every known queue
	// (see riverpolicy.RiverRetryMatrix); nil keeps the runtime
	// uninstrumented (tests).
	queueMetrics *redmetrics.MultiRefresher
}

func (r AppWorkerRuntime) Name() string {
	return "river-worker"
}

func (r AppWorkerRuntime) Kind() domain.ComponentKind {
	return domain.ComponentKindRiver
}

func (r AppWorkerRuntime) Start(ctx context.Context) error {
	if r.consume && r.riverClient != nil {
		slog.Info("River worker starting", "queue", r.queueName, "concurrency", r.concurrency)
		if err := r.riverClient.Start(ctx); err != nil {
			return err
		}
	}
	// queueMetrics keeps polling in every mode: the depth/lag gauges read
	// database truth and are scraped from the HTTP process, so an http-only
	// process still exports queue signals while never consuming.
	if r.queueMetrics == nil {
		return nil
	}
	return r.queueMetrics.Start(ctx)
}

func (r AppWorkerRuntime) Stop(ctx context.Context) error {
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

func (r AppWorkerRuntime) CheckHealth(context.Context) domain.ComponentStatus {
	return domain.ComponentStatus{Name: r.Name(), Kind: r.Kind(), Ready: r.riverClient != nil, Required: true}
}

var _ runtimeports.HealthChecker = AppWorkerRuntime{}

// EventBus exposes the shared workspace-envelope NATS bus for health
// checking. The bus itself stays owned by the worker runtime; callers
// must not close it.
func (r AppWorkerRuntime) EventBus() *eventnats.NATSJetStreamEventBus {
	return r.eventBus
}

// Close drains the shared NATS bus. Database pools stay owned by the
// composition root and are closed there.
func (r AppWorkerRuntime) Close() error {
	if r.eventBus == nil {
		return nil
	}
	return r.eventBus.Close()
}

func NewAppTaskRuntime(
	ctx context.Context,
	cfg *config.Config,
	dbs AppDatabases,
	repos AppRepositories,
	services AppDomainServices,
	redMetrics *redmetrics.Set,
) (AppTaskServices, AppWorkerRuntime, error) {
	workerConcurrency := cfg.Worker.EffectiveConcurrency()

	// The workspace settlement pieces below are the only task path: only
	// workspace_task_execute and periodic workers are registered on this
	// client (unregistered kinds fail loudly as unknown kinds, never
	// silently).
	userEventHub := events.NewUserEventHub()
	taskEventHub := taskservice.NewEventHub()
	// The publisher degrades gracefully without a task resolver (nil
	// resolver skips enrichment and still broadcasts bare task events to
	// the local hubs and NATS).
	//
	// The returned publisher is the ONLY way task.status_changed reaches
	// the hubs; it is wired into the worker settler bridge below. The v1 HTTP submit/cancel edges
	// deliberately do NOT route through it: they mint workspace envelopes
	// from their own per-request Service, so no transition is emitted
	// twice.
	taskStatusPublisher, eventBus, err := eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg.Events, taskEventHub, nil, userEventHub)
	if err != nil {
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize task event bus: %w", err)
	}
	// These are created before the worker bridge so worker settlements and
	// HTTP/control transitions share the same workspace envelope log and
	// delivery infrastructure.
	workspaceEvents := events.NewWorkspaceEventLog()
	webhookStore := eventwebhook.NewWebhookStore(eventwebhook.NewHTTPSender(), nil)
	webhookStore.SetPersistence(eventwebhook.NewPostgresWebhookPersistence(dbs.DB))
	// #940: cross-instance workspace-envelope fanout. Remote-origin
	// envelopes append into the shared hot log so this instance's SSE
	// replay + live subscribers observe other instances' commits.
	// Per-recipient permission filtering stays at SSE/webhook serve time;
	// the bridge never republishes (Append dedups without refanout).
	if eventBus != nil {
		if err := eventBus.SubscribeWorkspaceEvents(ctx, events.NewWorkspaceEventBridgeHandler(workspaceEvents)); err != nil {
			_ = eventBus.Close()
			return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to subscribe workspace event fanout: %w", err)
		}
	}

	riverQueue := taskqueue.NewRiverQueue(nil, dbs.DB, riverpolicy.RiverMaxAttemptsDefault)
	modelCustomParams := workerCustomParamsReader{repo: modelregistryinfrastructure.NewConfigRepository(repos.systemConfigRepo)}
	// Process-shared per-model guards: one breaker/concurrency window set
	// across all executions so fuses actually trip (per-client instances
	// would reset on every task). Thresholds come from env-backed config;
	// unset fields fall back to the code-owned conservative defaults.
	workerResilience := providerinfra.NewWorkerResilience(providerinfra.WorkerResilienceConfig{
		MaxConcurrentPerModel: cfg.Worker.Resilience.PerModelLimit,
		Cooldown:              time.Duration(cfg.Worker.Resilience.BreakerCooldownSeconds) * time.Second,
		FailureRate:           float64(cfg.Worker.Resilience.BreakerFailureRatePercent) / 100,
	}, redMetrics)
	// Single Worker exit: one client, no provider branching. A missing
	// WORKER_BASE_URL or WORKER_INTERNAL_SECRET fails every image task closed
	// with a clear message before any send.
	workerFactory := func(ctx context.Context) (runtimeports.WorkerClient, error) {
		return providerinfra.NewWorkerClient(providerinfra.WorkerClientConfig{
			BaseURL:    cfg.Worker.Edge.BaseURL,
			Secret:     cfg.Worker.Edge.Secret,
			Metrics:    redMetrics,
			Resilience: workerResilience,
		})
	}
	// #825 (#809 Phase 4b-01): the workspace settlement pieces are built
	// once here and shared with the HTTP submit path (see AppTaskServices).
	// The enqueuer starts clientless: the settlement Service needs it at
	// construction, the River client needs the worker at registration, so
	// the client attaches after NewRiverClient below (SetClient).
	workspaceStore := taskpostgres.NewStore(dbs.DB)
	workspaceHolder := taskadapters.NewWorkspaceTaskHolder(services.creditService)
	workspaceQuoter := &taskadapters.WorkspaceTaskQuoter{Catalog: services.billingCatalog}
	workspaceMedia := workspacemedia.NewService(dbs.DB)
	workspaceAssets := &workspacemedia.TaskAssetAdapter{Store: workspaceMedia}
	workspaceEnqueuer := taskadapters.NewWorkspaceTaskEnqueuer(nil, riverpolicy.RiverQueueDefault, riverpolicy.RiverMaxAttemptsDefault)
	workspaceRouter := routing.NewRoutingResolver(catalog.NewRepository(dbs.DB))
	workspaceSettler := taskapp.NewService(taskapp.ServiceDeps{
		TaskRead: workspaceStore, TaskLifecycle: workspaceStore,
		TaskBilling: workspaceStore, SubmissionClaims: workspaceStore, BillingAccounts: workspaceStore,
		Quoter: workspaceQuoter, Holder: workspaceHolder,
		Queue: workspaceEnqueuer, Assets: workspaceAssets, Resolver: workspaceRouter,
		// #946: the member cross-workspace consumption quota joins the
		// settlement transaction after Quote and before Hold.
		Quotas: memberquotas.NewStore(dbs.DB),
		Tracer: taskadapters.NewWorkspaceSettlementTracer(),
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
	taskStatusOutboxReplayer := &eventoutbox.TaskStatusOutboxReplayer{
		Store: eventoutbox.NewTaskStatusOutboxStore(dbs.DB), Status: taskStatusPublisher,
		Workspace: workspaceEvents, Bus: eventBus, Webhooks: webhookStore,
	}
	taskStatusOutboxWorker := outboxworker.NewWorker(taskStatusOutboxReplayer)
	// #940: directed board access signals whose live emit never completed
	// replay through the same sinks (hot log + NATS bus, never webhooks).
	boardAccessOutboxReplayer := &eventoutbox.BoardAccessSignals{
		Store: eventoutbox.NewBoardAccessOutboxStore(dbs.DB),
		Log:   workspaceEvents, Bus: eventBus,
	}
	boardAccessOutboxWorker := outboxworker.NewBoardAccessWorker(boardAccessOutboxReplayer)
	// Live dispatch finalize seam: Worker image bytes land directly in
	// workspace media assets, so Generate
	// yields workspace asset refs for the renderer + atomic Complete.
	// Worker configuration resolves per execution inside workerFactory; a
	// missing base URL or secret fails tasks deterministically before
	// this materializer ever runs. The presigned media handoff mints
	// HMAC URLs served by the public worker media route below
	// (http_wiring): inputs GET, single output PUT, exact keys.
	workspaceProviderMaterializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media: workspaceMedia, Objects: services.storageService,
		Fetcher: taskruntime.WorkspaceTaskResultBytesFetcher{},
	})
	if err != nil {
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize workspace provider result materializer: %w", err)
	}
	workspaceTransfer := workertransfer.NewWorkerTransfer(workspaceMedia, services.storageService, cfg.PublicOrigins.Web, cfg.Worker.Edge.Secret)
	workspaceWorkerExec := providers.NewExecutor(workspaceProviderMaterializer, workerFactory, modelCustomParams, workspaceTransfer)
	// Provider ingestion is complete; the renderer verifies workspace assets
	// before settlement and owns reference-safe compensation after confirmed Fail.
	workspaceRenderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: workspaceMedia, Assets: workspaceAssets,
	})
	if err != nil {
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task renderer: %w", err)
	}
	boardStore := boards.NewPostgresWorkspaceBoardStore(repos.db, workspacemedia.ValidateDocumentAssets)
	workspaceLibraryStore := librarypostgres.NewService(repos.db, &workspacemedia.LibraryPublicationAuthorizer{
		Boards: boardStore, TaskBoards: &boardtaskaccess.Authorizer{Store: boardStore},
	})
	workspaceAutoSaver, err := taskruntime.NewWorkspaceTaskAutoSaver(taskruntime.NewWorkspaceTaskLibrarySaver(workspaceLibraryStore))
	if err != nil {
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task auto-saver: %w", err)
	}
	workspaceWorker, err := taskworker.NewWorkspaceTaskWorker(taskworker.WorkspaceTaskWorkerDeps{
		Store: workspaceStore, Settler: workspaceSettlerBridge, Assets: workspaceAssets,
		Provider: taskworker.NewWorkspaceWorkerProvider(&workspaceWorkerCallerAdapter{executor: workspaceWorkerExec}),
		Renderer: workspaceRenderer, LibrarySaver: workspaceAutoSaver,
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
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize workspace task worker: %w", err)
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
		DB:        dbs.DB,
		Users:     repos.userRepo,
		Auth:      services.authServiceClient,
		Manager:   services.authServiceClient,
		Lifecycle: services.userLifecycle,
	})
	identityAdminDispatcher, err := identityreconciliation.NewRunDispatcher(services.riverProducer)
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize identity admin reconciliation dispatcher: %w", err)
	}
	identityPeriodicJobs := identityreconciliation.PeriodicJobs()
	if err := eventinterfaces.ConfigureUserEventFanout(ctx, eventBus, userEventHub, services.paymentService); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize user event fanout: %w", err)
	}
	visionBillingService := visionapplication.NewVisionBillingService(services.dbPool, services.billingCatalog, services.creditService, nil)
	// #946: the member cross-workspace consumption quota joins the Vision
	// reservation transaction after Quote and before Hold.
	visionBillingService.SetQuotaChecker(memberquotas.NewStore(dbs.DB))
	visionBillingPeriodicJobs := visionbilling.VisionBillingPeriodicJobs()
	// #833: the webhook store was built above so the River worker and the HTTP
	// views share one instance. The enqueuer attaches after NewRiverClient
	// below (SetEnqueuer), mirroring the workspaceEnqueuer SetClient seam.
	if err := webhookStore.LoadFromPersistence(ctx); err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to load webhook subscriptions: %w", err)
	}
	// No task-retention periodic jobs are registered (no retention store
	// left to purge against).
	// Workspace task lifecycle retention is #809-close work.
	// Workspace deletion is an immediate irreversible hard delete: there
	// is no archive/restore lifecycle and no background purge.
	taskStatusOutboxPeriodicJobs := outboxworker.PeriodicJobs()
	boardAccessOutboxPeriodicJobs := outboxworker.BoardAccessPeriodicJobs()
	webhookPeriodicJobs := eventwebhook.WebhookReconciliationPeriodicJobs()
	// ADR-0021: an http-only process enqueues but never consumes. Workers
	// stay unregistered and periodic schedules stay home on the worker
	// process, so exactly one scheduler exists per deployment (local `all`
	// keeps the pre-split behavior). The enqueue-only client below still
	// serves the submit transaction (SetClient) without fetching anything.
	runWorkers := cfg.Mode.RunsWorkers()
	periodicJobs := make([]*river.PeriodicJob, 0, len(creditPeriodicJobs)+len(paymentPeriodicJobs)+len(checkoutShutdownPeriodicJobs)+len(visionBillingPeriodicJobs)+len(settlementSweepPeriodicJobs)+len(taskStatusOutboxPeriodicJobs)+len(boardAccessOutboxPeriodicJobs)+len(identityPeriodicJobs)+len(webhookPeriodicJobs))
	if runWorkers {
		periodicJobs = append(periodicJobs, creditPeriodicJobs...)
		periodicJobs = append(periodicJobs, paymentPeriodicJobs...)
		periodicJobs = append(periodicJobs, checkoutShutdownPeriodicJobs...)
		periodicJobs = append(periodicJobs, visionBillingPeriodicJobs...)
		periodicJobs = append(periodicJobs, settlementSweepPeriodicJobs...)
		periodicJobs = append(periodicJobs, taskStatusOutboxPeriodicJobs...)
		periodicJobs = append(periodicJobs, boardAccessOutboxPeriodicJobs...)
		periodicJobs = append(periodicJobs, identityPeriodicJobs...)
		periodicJobs = append(periodicJobs, webhookPeriodicJobs...)
	}
	// Only workspace_task_execute and periodic workers are registered
	// (see workerriver.NewRiverClient); unregistered kinds fail loudly
	// as unknown kinds. The workspace_task_execute consumer and all
	// periodic workers ride this same client. Nil in http-only
	// mode (see runWorkers above): the client then only enqueues.
	invitationMailQueue := &invitations.MailQueue{}
	var registerWorkers func(workers *river.Workers)
	if runWorkers {
		registerWorkers = func(workers *river.Workers) {
			river.AddWorker(workers, &invitations.MailWorker{Store: tenancypg.NewStore(dbs.DB, services.storageService), Sender: services.authServiceClient})
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
			river.AddWorker(workers, eventwebhook.NewWebhookDeliverWorker(webhookStore))
			river.AddWorker(workers, eventwebhook.NewWebhookReconcileWorker(webhookStore))
			// Final-timeout settlement sweep: converges abandoned Credit Holds
			// through the worker's own settler bridge.
			river.AddWorker(workers, settlementSweepWorker)
			// Durable task status event replay shares the maintenance queue; the
			// outbox row is the delivery retry fact, never the task queue.
			river.AddWorker(workers, taskStatusOutboxWorker)
			// #940: directed board access-signal replay rides the same
			// maintenance queue (hot log + NATS sinks, never webhooks).
			river.AddWorker(workers, boardAccessOutboxWorker)
			// Identity admin reconciliation: the dry-run scan worker and its
			// schedule worker ride the same client (no new infra).
			river.AddWorker(workers, identityreconciliation.NewExecuteWorker(identityAdminReconciler))
			river.AddWorker(workers, identityreconciliation.NewScheduleWorker(identityAdminReconciler, identityAdminDispatcher))
		}
	}
	var riverClient *river.Client[pgx.Tx]
	if runWorkers {
		riverClient, err = workerriver.NewRiverClient(dbs.RiverPool, workerriver.RiverServerOptions{
			QueueName:   riverpolicy.RiverQueueDefault,
			Concurrency: workerConcurrency,
			AdditionalQueues: map[string]river.QueueConfig{
				riverpolicy.RiverQueueWorkspaceInvitationMail: {MaxWorkers: 1},
				workerbilling.CreditReconciliationQueue:       {MaxWorkers: 1},
				workerbilling.PaymentReconciliationQueue:      {MaxWorkers: 1},
				workerbilling.PaymentCheckoutShutdownQueue:    {MaxWorkers: 1},
				visionbilling.VisionBillingQueue:              {MaxWorkers: 1},
				// #833: signed webhook deliveries ride their own queue with
				// the shared concurrency config (no new infra); serial
				// workers match the maintenance-queue precedent and the 1m
				// per-attempt timeout bounds head-of-line blocking.
				riverpolicy.RiverQueueWebhookDeliver: {MaxWorkers: 1},
				// Final-timeout settlement sweep: one serial maintenance worker on
				// the shared client (same precedent as the queues above). Serial is
				// deliberate: the sweep converges abandoned holds, so overlapping
				// runs would contend on the same per-task settle transactions for no
				// throughput gain.
				riverpolicy.RiverQueueTaskSettlementSweep: {MaxWorkers: 1},
				// Identity admin reconciliation: one serial maintenance worker.
				// Serial is deliberate: overlapping scans would contend on the
				// same finding fingerprints for no throughput gain.
				identityreconciliation.IdentityReconciliationQueue: {MaxWorkers: 1},
			},
			RegisterWorkers: registerWorkers,
			PeriodicJobs:    periodicJobs,
		})
	} else {
		// HTTP-only instances are insert-only River clients. Leaving Workers nil
		// is River's supported producer topology: jobs can be enqueued in the
		// submit transaction and are consumed by the worker-only peer.
		riverClient, err = workerriver.NewRiverProducer(dbs.RiverPool)
	}
	if err != nil {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to initialize River client: %w", err)
	}
	// No startup recovery pass runs here. Workspace recovery is River
	// redelivery + lease-guarded claim (#825).
	invitationMailQueue.Client = riverClient
	riverQueue.SetClient(riverClient)
	workspaceEnqueuer.SetClient(riverClient)
	webhookStore.SetEnqueuer(riverClient)
	if err := webhookStore.ReconcilePending(ctx); err != nil {
		_ = riverClient.Stop(ctx)
		if eventBus != nil {
			_ = eventBus.Close()
		}
		return AppTaskServices{}, AppWorkerRuntime{}, fmt.Errorf("failed to reconcile pending webhooks: %w", err)
	}

	return AppTaskServices{
		invitationMailQueue: invitationMailQueue, // #821 (#809 Phase 4-04): v1 workspace task enqueue rides the
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
		userEvents:          userEventHub,
		taskStatusPublisher: taskStatusPublisher,
	}, AppWorkerRuntime{
		riverClient:  riverClient,
		eventBus:     eventBus,
		queueName:    riverpolicy.RiverQueueDefault,
		concurrency:  workerConcurrency,
		consume:      runWorkers,
		queueMetrics: redmetrics.NewMultiRefresher(redMetrics, riverQueueStatsByQueue(riverQueue), redmetrics.DefaultRefreshInterval),
	}, nil
}

// riverQueueStatsByQueue fans one RiverQueue monitor out to every known
// queue so each queue gets its own depth/lag gauge series. Queue inventory
// is derived from riverpolicy.RiverRetryMatrix (the single roster source);
// a missing queue loses its RED signals (depth/lag alerts in
// docs/runbooks/backend-metrics.md), and a new matrix queue is picked up
// without touching this wiring.
// Only registered matrix queues are polled (no retention queue).
func riverQueueStatsByQueue(monitor *taskqueue.RiverQueue) map[string]redmetrics.RiverQueueStats {
	seen := map[string]struct{}{}
	queues := make([]string, 0, len(riverpolicy.RiverRetryMatrix()))
	for _, policy := range riverpolicy.RiverRetryMatrix() {
		if _, dup := seen[policy.Queue]; dup {
			continue
		}
		seen[policy.Queue] = struct{}{}
		queues = append(queues, policy.Queue)
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
// Why it exists: taskapp.Service STAGES a status event inside the
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
// flip + Capture/Release); the expensive work (Worker send, download,
// render) happens before Complete/Fail is called.
type settlerEventBridge struct {
	inner     settlerStager
	publisher events.TaskStatusPublisher
	delivery  settlerEventDelivery
	mu        sync.Mutex
}

// settlerStager is the narrow settlement seam the bridge needs: the three
// terminal writes plus the staged-event drain. *taskapp.Service is
// the production implementation.
type settlerStager interface {
	Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
	Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error)
	StagedEvents() []tasks.TaskEvent
}

var (
	_ taskworker.WorkspaceTaskWorkerSettler = (*settlerEventBridge)(nil)
	_ settlerStager                         = (*taskapp.Service)(nil)
)

func newSettlerEventBridge(inner settlerStager, publisher events.TaskStatusPublisher, delivery settlerEventDelivery) *settlerEventBridge {
	return &settlerEventBridge{inner: inner, publisher: publisher, delivery: delivery}
}

// Complete publishes task.completed and distinct media.asset_ready hints
// after successful settlement. Worker ingestion has no separate event seam.
func (b *settlerEventBridge) Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	completed, err := b.inner.Complete(ctx, workspaceID, taskID, outputs)
	b.flush(ctx, err)
	if err == nil {
		b.emitMediaAssetReadyEvents(ctx, workspaceID, outputs)
	}
	return completed, err
}

// Fail converges the task to failed, then publishes task.failed.
func (b *settlerEventBridge) Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
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
func (b *settlerEventBridge) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error) {
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
		if strings.TrimSpace(event.EventID) == "" {
			event.EventID = events.StableEventID(event.TaskID, string(event.Status), "")
		}
		statusEvent := events.TaskStatusEvent{
			Type:       events.EventTypeTaskStatusChanged,
			EventID:    event.EventID,
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
func (b *settlerEventBridge) emitWorkspaceEvent(ctx context.Context, event tasks.TaskEvent) {
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
	b.deliverWorkspaceEnvelope(ctx, event.EventID, typ, event.WorkspaceID,
		events.V1Resource{Type: events.V1ResourceTask, ID: event.TaskID},
		workerWorkspaceEventData(typ, event))
}

// Stable per-asset event IDs let the sinks deduplicate redelivered hints.
func (b *settlerEventBridge) emitMediaAssetReadyEvents(ctx context.Context, workspaceID string, outputs []tasks.OutputRef) {
	workspaceID = strings.TrimSpace(workspaceID)
	seen := make(map[string]struct{}, len(outputs))
	for _, ref := range outputs {
		if strings.TrimSpace(ref.Kind) != tasks.OutputKindAsset {
			continue
		}
		assetID := strings.TrimSpace(ref.AssetID)
		if assetID == "" {
			continue
		}
		if _, dup := seen[assetID]; dup {
			continue
		}
		seen[assetID] = struct{}{}
		b.deliverWorkspaceEnvelope(ctx,
			events.StableEventID(assetID, events.EventV1MediaAssetReady),
			events.EventV1MediaAssetReady, workspaceID,
			events.V1Resource{Type: events.V1ResourceMediaAsset, ID: assetID},
			events.V1MediaAssetReadyData{AssetID: assetID})
	}
}

// Mint only when a sink is attached; delivery failures never fail settlement.
func (b *settlerEventBridge) deliverWorkspaceEnvelope(ctx context.Context, id, typ, workspaceID string, resource events.V1Resource, data any) {
	if b.delivery.log == nil && b.delivery.bus == nil && b.delivery.webhooks == nil {
		return
	}
	envelope, err := events.MintV1EnvelopeWithID(id, typ, workspaceID,
		events.V1Actor{Type: events.V1ActorService, ID: "workspace-task-worker"}, resource, data)
	if err != nil {
		slog.Warn("mint worker workspace event failed", "id", id, "type", typ,
			"resource_type", resource.Type, "resource_id", resource.ID, "err", err)
		return
	}
	if b.delivery.log != nil {
		b.delivery.log.Append(envelope)
	}
	if b.delivery.bus != nil {
		if publishErr := b.delivery.bus.PublishWorkspaceEvent(ctx, envelope); publishErr != nil {
			slog.Warn("publish worker workspace event failed", "id", envelope.ID, "type", envelope.Type, "err", publishErr)
		}
	}
	if b.delivery.webhooks != nil {
		b.delivery.webhooks.Dispatch(ctx, envelope)
	}
}

func workerWorkspaceEventData(typ string, event tasks.TaskEvent) any {
	switch typ {
	case events.EventV1TaskCompleted:
		return events.V1TaskCompletedData{
			TaskID: event.TaskID,
			OutputsURI: "/api/v1/workspaces/" + strings.TrimSpace(event.WorkspaceID) +
				"/tasks/" + strings.TrimSpace(event.TaskID) + "/outputs",
			Outputs: event.Outputs,
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

// workerCustomParamsReader adapts catalog custom params by structured pool
// model (provider plus bare model). Runtime stays independent of the model
// registry business context. Only opaque custom JSON travels to the Worker;
// @path overrides are rejected directly (single Worker endpoint).
type workerCustomParamsReader struct {
	repo interface {
		ListCatalog(context.Context) ([]modelregistrydomain.CatalogItem, error)
	}
}

func (a workerCustomParamsReader) LookupCustomParams(ctx context.Context, provider, model string) (string, bool, error) {
	items, err := a.repo.ListCatalog(ctx)
	if err != nil {
		return "", false, err
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Provider), strings.TrimSpace(provider)) &&
			strings.TrimSpace(item.ID) != "" && strings.EqualFold(strings.TrimSpace(item.ID), strings.TrimSpace(model)) {
			return item.CustomParams, true, nil
		}
	}
	return "", false, nil
}
