package composition

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/application/administration"
	"ideaboard/api/internal/identity/users/application/authsync"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	libraryapplication "ideaboard/api/internal/library/application"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
	materializerassets "ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	"ideaboard/api/internal/media/workspacemedia"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/prompts"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/runtime/adapters/webdist"
	configinfra "ideaboard/api/internal/runtime/infrastructure/config"
	httpinfra "ideaboard/api/internal/runtime/infrastructure/http"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/ports"
	searchapplication "ideaboard/api/internal/searchcatalog/application"
	searchinfra "ideaboard/api/internal/searchcatalog/infrastructure"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	searchports "ideaboard/api/internal/searchcatalog/ports"
	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	workspacetasks "ideaboard/api/internal/tasking/workspacetasks"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newAppRouter(
	cfg *config.Config,
	repos appRepositories,
	services appDomainServices,
	taskServices appTaskServices,
	readyChecker ports.HealthChecker,
	redMetrics *redmetrics.Set,
) (*gin.Engine, error) {
	// #809 hard cut: the /api/user + /api/admin handler trees are
	// deleted, so only the contract/static/webhook/internal handlers are
	// built here. The Stripe webhook stays live; the Control Plane
	// reuses the platform-config handlers (ai-gateway, storage, search
	// providers, feature prompts) under its capability guard.
	paymentWebhookHandler := paymenthttp.NewWebhookHandler(services.paymentService)
	creditUserResolver := creditsUserResolver{repo: repos.userRepo}

	authDeps := httpapi.AuthDeps{
		AuthServiceClient: services.authServiceClient,
		UserRepo:          repos.userRepo,
		UserLifecycle:     services.userLifecycle,
	}
	searchProviderConfig := searchinfra.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayRepo := aigateway.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayHandler := aigateway.NewHandler(aiGatewayRepo)
	aiGatewayHandler.SetObservedService(aigateway.NewObservedService(
		aigateway.NewObservedRepository(repos.db),
		newBillingSKUCatalogAdapter(services.billingCatalog),
	))
	// Control audit durability (step 2: atomic where the guard owns the
	// transaction). The directory is the request-scoped public->internal
	// resolver the control handlers already use; both sinks reuse that
	// SAME instance for their per-audit operator lookup (no cache, no new
	// global state). The observer sink persists into control_audit_records
	// in its own detached transaction (reads and fail-fast denials have
	// no business write to be atomic with; failures only slog.Warn).
	// The participant sink joins the guard-owned mutation transaction
	// instead, so the business fact and its audit row commit or roll back
	// together; its failures roll the mutation back (semantic inversion
	// versus step 1, by definition of atomic). Both sinks share one
	// audit.Store over the pool.
	controlUserDirectory := httpapi.NewUsersRepositoryDirectory(repos.userRepo)
	controlAuditStore := audit.NewStore(repos.db)
	controlAuditSink := newControlAuditDurableSink(controlAuditStore, controlUserDirectory)
	controlAuditTxSink := newControlAuditAtomicSink(controlAuditStore, controlUserDirectory)
	// Control idempotency (#891): ONE shared process-local replay store
	// for the whole process (a per-request store would never replay:
	// every lookup would miss its own empty cache), backed by the
	// PostgreSQL durable authority. The local store is a same-process
	// fast-path optimization only; correctness never depends on it.
	controlIdempotency := contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	controlIdempotencyDurable := idempotency.NewControlStore(repos.db)

	router := gin.New()
	// ClientIP feeds per-IP rate limiting. Trust only the configured reverse
	// proxies so X-Forwarded-For cannot be spoofed by direct clients.
	// Production behind Nginx/Cloudflare must set APP_TRUSTED_PROXIES.
	if err := router.SetTrustedProxies(cfg.App.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid trusted proxies %v: %w", cfg.App.TrustedProxies, err)
	}
	// Library materialization read-through (issue #908): composition
	// injects the typed store/materializer/executor seams. Media owns
	// remote download + managed-media writes; Library application owns
	// lifecycle + replay orchestration; HTTP only translates
	// requests/responses. A materializer build failure fails startup
	// (fail closed) instead of serving silent 404s.
	boardStore := boards.NewPostgresWorkspaceBoardStore(repos.db, workspacemedia.ValidateDocumentAssets)
	libraryStore := librarypostgres.NewService(repos.db, &workspacemedia.LibraryPublicationAuthorizer{Boards: boardStore, TaskBoards: &boardtaskaccess.Authorizer{Store: boardStore}})
	librarySources := &libraryapplication.SourceResolver{Store: libraryStore}
	libraryMaterializer, err := materializerassets.NewLibraryMaterializer(materializerassets.LibraryMaterializerDeps{
		Media: taskServices.workspaceTaskMedia, Objects: services.storageService,
		Fetcher: materializerassets.RemoteLibrarySourceFetcher{},
		Replay:  librarySources,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize library materializer: %w", err)
	}
	libraryReadThrough := libraryapplication.NewReadThroughService(
		libraryStore, libraryMaterializer, libraryapplication.GoroutineExecutor{}, libraryapplication.DefaultReadThroughTimeout)
	mediaRenderer := imgproxy.NewRenderer(imgproxyConfigFromApp(cfg), services.storageService)
	mediaPreviews, err := preview.NewService(imgproxyConfigFromApp(cfg), configinfra.RuntimeUploadDir, mediaRenderer)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize media previews: %w", err)
	}
	// Panic recovery, access logging and request IDs are registered in
	// registerGlobalMiddleware so every route shares the error envelope.
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		ContractV1: httpapi.ContractV1Deps{
			SupportChannels: services.runtimeSettings,
			Store:           tenancypg.NewStore(repos.db),
			Users:           httpapi.NewUsersRepositoryDirectory(repos.userRepo),
			// Workspace-member emails for invite search reuse the Admin
			// emailReader join (users -> AuthUserIDs -> Auth emails);
			// members without email omit the field, never fail the list.
			MemberEmails:  httpapi.NewMemberEmailService(repos.userRepo, services.authServiceClient),
			MemberLookup:  httpapi.NewMemberLookupService(repos.userRepo, services.authServiceClient),
			AvatarUID:     httpapi.NewV1AvatarDirectory(repos.userRepo),
			AvatarProfile: httpapi.NewV1AvatarProfileDirectory(repos.userRepo),
			// Avatar bytes ride the storage abstraction (active
			// profile): the same seam serves local today and R2 after
			// the profile cutover.
			AvatarObjects: services.storageService,
			// Workspace board store is live (postgres mapping over
			// board_documents/board_revisions, #809 follow-up landed).
			Boards: boardStore,
			// Acknowledge immediate delivery of transaction-committed access signals.
			BoardAccessOutbox: events.NewBoardAccessOutboxStore(repos.db),
			// Workspace library and media stores are live. Library
			// materialization is internal-only (owner hard cut): no
			// public materialization route or emitter is wired here;
			// the read-through orchestrator rides the typed
			// store/materializer/executor seams built above.
			Library: httpapi.ContractV1LibraryDeps{
				Store:       libraryStore,
				ReadThrough: libraryReadThrough,
				Sources:     librarySources,
			},
			Media: taskServices.workspaceTaskMedia,
			// Binary reads ride the storage service.
			MediaObjects:  services.storageService,
			MediaRenderer: mediaRenderer,
			MediaPreviews: mediaPreviews,
			// #821 (#809 Phase 4-04): workspace-scoped task postgres
			// store + production settlement adapters. Task routes
			// are live: Quote via the billing catalog, Hold/Capture/
			// Release via credits.Commands, enqueue via the River
			// client (workspace_task_execute kind; #825 registers the
			// real worker consumer on the same client), input/output
			// asset gates via the media TaskAssetAdapter. The store,
			// holder, quoter, queue, and assets below are the single
			// shared instances built in task_wiring.go (the worker
			// settles through the same holder/Service); settlement is
			// transaction-first (Quote->Hold->Insert->Enqueue and
			// terminal Capture/Release each commit in ONE shared PG
			// transaction via the Tx seams, compensating only for
			// backends without them; see workspace_task_adapters.go).
			Tasks:      taskServices.workspaceTaskStore,
			TaskQuoter: taskServices.workspaceTaskQuoter,
			TaskHolder: taskServices.workspaceTaskHolder,
			TaskQueue:  taskServices.workspaceTaskQueue,
			TaskAssets: taskServices.workspaceTaskAssets,
			// #848: quote, submit, and readiness share one RoutingResolver
			// over the catalog pool (ADR-0012 rule 10, single seam).
			// Submit pins its snapshot atomically via
			// catalog.SaveRoutingSnapshot with the settlement tx's
			// Queries. The fail-closed fallback for an unset resolver is
			// the canonical unavailable sentinel (#854: no legacy gate).
			TaskRouter:            taskServices.workspaceTaskRouter,
			TaskSnapshotSaver:     newPoolRoutingSnapshotSaver(),
			TaskCanonicalCompiler: taskservice.CompileWorkspaceParams,
			// #833: the webhook store is shared with the River delivery
			// worker (built in task_wiring.go where the client exists),
			// so HTTP views and the worker observe one registry.
			Webhooks: taskServices.webhooks,
			// The workspace envelope hot log is built in task_wiring.go and
			// shared with the Control Plane admin-cancel adapter below: one
			// instance, so an operator cancel reaches the same SSE replay
			// window and live subscribers a user cancel does.
			Events: taskServices.workspaceEvents,
			// Use the same workspace-envelope bus for user and worker events;
			// control-plane wiring below shares this instance as well.
			EventBus: taskServices.workspaceEventBus,
		},
		// Workspace billing surface (issue #804). Reads compose the
		// Credits ledger, billing records, and usage through their owning
		// services (no handler ledger writes); checkout reuses the
		// Payments seam with a workspace-namespaced idempotency key, and
		// webhook settlement stays inside Payments -> credits.Commands.
		// Workspace task submission uses the shared transaction-first
		// settlement seams wired above; billing reads below remain owned
		// by the billing services.
		ContractV1Billing: httpapi.ContractV1BillingDeps{
			Tenancy:         tenancypg.NewStore(repos.db),
			Users:           httpapi.NewUsersRepositoryDirectory(repos.userRepo),
			BillingAccounts: tenancypg.NewBillingAccountStore(repos.db),
			Wallets:         services.creditService,
			Records:         services.billingRecords,
			Usage:           services.usageService,
			Quoter:          services.billingCatalog,
			// #851: billing-quotes shares the pool availability decision
			// with task-quotes/readiness/submit for ai-image SKUs
			// (same RoutingResolver instance, never a second lookup).
			PoolRouter: taskServices.workspaceTaskRouter,
			Checkout: httpapi.NewPaymentsCheckoutAdapter(services.paymentService,
				cfg.PublicOrigins.WebURL("/user/boards")),
			// Gaps batch (#809-close, backend B): the Payments service
			// implements the full UserCheckoutService seam (purchase
			// options, pending continue/abandon, recharge history), the
			// billing catalog backs billing-quotes (incl.
			// ai-search:vision), and the Rewards service owns
			// redeem/history (Grant via credits.Commands inside its own
			// transaction; no handler ledger writes).
			PurchaseOptions: services.paymentService,
			PendingOrders:   services.paymentService,
			Recharges:       services.paymentService,
			Rewards:         services.rewardService,
		},
		// Control administration shares domain handlers and stores. The guard
		// enforces authorization, rate limits, idempotency and durable audit.
		// Tasks use the workspace read model and transaction-bound settlement;
		// Monitor uses workspace statistics and the River queue monitor.
		ContractControl: httpapi.ContractControlDeps{
			Users:          services.userService,
			Identity:       administration.NewIdentityAdminCommands(services.userService, services.authServiceClient, authsync.NewStore(repos.db)),
			Impersonator:   services.authServiceClient,
			Credits:        services.creditService,
			CreditUsers:    creditUserResolver,
			CreditReads:    services.creditService,
			Reconciliation: services.creditService,
			Tasks: httpapi.NewWorkspaceControlTasks(services.dbPool, taskServices.workspaceTaskStore, taskServices.workspaceTaskHolder, creditUserResolver,
				// Admin-cancel announcements (admin cancel dead-channel fix).
				// The Control Plane declares a neutral port and this adapter is
				// the only place the two are joined. It reuses the SAME task
				// status publisher the River worker bridge publishes through
				// (a second publisher would re-open the double-send bug), the
				// SAME workspace hot log the SSE endpoints read, the SAME
				// webhook store the worker delivers from, and the SAME NATS
				// workspace bus for cross-instance envelope fanout. No second
				// NATS connection is opened by this wiring.
				newControlTaskEventAdapter(taskServices.taskStatusPublisher, taskServices.workspaceEvents, taskServices.workspaceEventBus, taskServices.webhooks)),
			Tenancy: tenancypg.NewStore(repos.db),
			// #941: operator membership removals purge board grants.
			Boards:             boardStore,
			UserDirectory:      controlUserDirectory,
			BillingAccounts:    tenancypg.NewBillingAccountStore(repos.db),
			Catalog:            services.billingCatalog,
			ModelPools:         catalog.NewRepository(repos.db), // #849: Admin pool reads/replaces; no second model catalog.
			Payments:           services.paymentService,
			Settings:           services.runtimeSettings,
			PaymentWebhookURL:  cfg.PublicOrigins.APIURL("/api/webhooks/stripe"),
			AIGateway:          aiGatewayHandler,
			Storage:            storagehttp.NewHandler(services.storageService),
			SearchProviders:    searchhttp.NewHandler(searchProviderConfig, searchinfra.NewProviderTestAdapters()...),
			FeaturePrompts:     prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			Monitor:            newControlMonitorHandler(repos.db),
			Mail:               services.authServiceClient,
			RewardCommands:     rewardhttp.NewAdminCommandHandler(services.rewardService),
			RewardReads:        rewardhttp.NewAdminReadHandler(services.rewardService, rewardIdentityProjector{repo: repos.userRepo}),
			OAuth:              services.authServiceClient,
			BrandLogos:         adminbranding.NewHandler(repos.adminBrandLogoRepo, controlRequireAdmin),
			BrandingUpload:     services.storageService,
			AuditSink:          controlAuditSink,
			Idempotency:        controlIdempotency,
			IdempotencyDurable: controlIdempotencyDurable,
			// Step 2 atomic scope: every guarded mutation runs inside
			// one guard-owned transaction that the audit row joins
			// before commit (nil in tests keeps the legacy path).
			TxScope: newControlTxScope(repos.db),
			TxAudit: controlAuditTxSink,
		},
		PublicSearchHandler: searchhttp.NewSearchHandler(searchapplication.NewSearchService(searchProviderConfig, []searchports.SearchProviderAdapter{searchinfra.NewUnsplashAdapter(), searchinfra.NewPexelsAdapter()}), mediaPreviews),
		PaymentWebhook:      paymentWebhookHandler,
		Internal:            httpapi.InternalRouteDeps{UserHandler: identityhttp.NewInternalHandler(repos.userRepo, cfg.Auth.InternalAPIKey)},
		CORSMiddleware:      httpinfra.BuildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
		RateLimit:           services.runtimeSettings,
		Branding:            services.runtimeSettings,
		REDMetrics:          redMetrics,
		ReadyChecker:        readyChecker,
	}, httpapi.FrontendRouteOptions{
		WarnMissingIndex: configinfra.IsProductionRuntime(cfg.App.Env),
	})

	return router, nil
}

// newPoolRoutingSnapshotSaver pins the submit-time RoutingSnapshot with the
// settlement transaction's Queries. A missing transaction is a production
// wiring error and fails closed; snapshots must never be persisted outside
// the settlement transaction.
func newPoolRoutingSnapshotSaver() workspacetasks.RoutingSnapshotSaver {
	return func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		if tx == nil {
			return errors.New("routing snapshot saver requires the settlement transaction")
		}
		return catalog.SaveRoutingSnapshot(ctx, dbsqlc.New(tx), taskID, snap)
	}
}

// newControlMonitorHandler wires the workspace-backed statistics store
// plus the River queue monitor into the control monitor/statistics
// handler (GetMonitor, StreamMonitor, GetStatistics, BackfillStatistics).
// The River client stays nil: queue monitoring only needs the database
// handle (river_job depth/lag, running attempts). A nil pool keeps the
// handler nil so the monitor routes stay fail-closed with 501.
func newControlMonitorHandler(db *pgxpool.Pool) *statisticshttp.Handler {
	if db == nil {
		return nil
	}
	store := taskpostgres.NewStatisticsStore(db)
	monitor := taskqueue.NewRiverQueue(nil, db, riverpolicy.RiverMaxAttemptsDefault)
	return statisticshttp.NewHandler(statisticsapp.NewService(store), monitor)
}

// controlRequireAdmin adapts the identity admin check to the platform
// brand-logo override handler port. The control guard already enforced
// the capability; this keeps the handler's fail-closed default when it
// is ever served outside the guard.
func controlRequireAdmin(c *gin.Context) bool {
	_, ok := principal.RequireAdmin(c)
	return ok
}

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies, frontendOptions httpapi.FrontendRouteOptions) {
	frontendFS, err := webdist.DistFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend dist", "err", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS, frontendOptions)
}
