package composition

import (
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/application/administration"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/library"
	storageauth "ideaboard/api/internal/media/storage/application/authorization"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/config"
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
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
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
	uploadReadAccess := storageauth.NewReadableUploadAccess(
		uploadBoardAccessResolver{service: services.boardService},
		uploadOwnerStorageUIDResolver{repo: repos.userRepo},
	)
	searchProviderConfig := searchinfra.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayRepo := aigateway.NewConfigRepository(repos.systemConfigRepo)
	aiGatewayHandler := aigateway.NewHandler(aiGatewayRepo)
	aiGatewayHandler.SetObservedService(aigateway.NewObservedService(
		aigateway.NewObservedRepository(repos.db),
		aiGatewayRepo,
		newBillingSKUCatalogAdapter(services.billingCatalog),
	))

	router := gin.New()
	// ClientIP feeds per-IP rate limiting. Trust only the configured reverse
	// proxies so X-Forwarded-For cannot be spoofed by direct clients.
	// Production behind Nginx/Cloudflare must set APP_TRUSTED_PROXIES.
	if err := router.SetTrustedProxies(cfg.App.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid trusted proxies %v: %w", cfg.App.TrustedProxies, err)
	}
	// Panic recovery, access logging and request IDs are registered in
	// registerGlobalMiddleware so every route shares the error envelope.
	registerHTTPRoutes(router, httpapi.Dependencies{
		Auth: authDeps,
		ContractV1: httpapi.ContractV1Deps{
			Store: tenancypg.NewStore(repos.db),
			Users: httpapi.NewUsersRepositoryDirectory(repos.userRepo),
			// Workspace library and media stores are live. Library
			// materialization has server-side billing adapters, but the
			// HTTP path intentionally leaves remote media Materializer nil
			// until that fetch/storage adapter is available; the endpoint
			// then fails closed before creating a billable job.
			Library: httpapi.ContractV1LibraryDeps{
				Store:   library.NewService(repos.db),
				Quoter:  taskruntime.NewLibraryMaterializeQuoter(services.billingCatalog),
				Billing: taskruntime.NewLibraryMaterializeSettler(services.creditService),
			},
			Media: taskServices.workspaceTaskMedia,
			// Binary reads ride the storage service. Derivative rendering
			// is wired in the workspace task worker; this HTTP contract
			// keeps its optional renderer nil, so metadata remains usable
			// while ungenerated tiers stay pending.
			MediaObjects: services.storageService,
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
			Tasks:                 taskServices.workspaceTaskStore,
			TaskQuoter:            taskServices.workspaceTaskQuoter,
			TaskHolder:            taskServices.workspaceTaskHolder,
			TaskQueue:             taskServices.workspaceTaskQueue,
			TaskAssets:            taskServices.workspaceTaskAssets,
			TaskCanonicalCompiler: taskservice.CompileWorkspaceParams,
			// The AI gateway repo doubles as the submit-time model
			// resolver: it mints the durable plan model at submit.
			// Unmapped features fail submission closed; gateway
			// credentials stay request-scoped via Resolve (never
			// persisted, never logged).
			TaskModelResolver: aiGatewayRepo,
			// #833: the webhook store is shared with the River delivery
			// worker (built in task_wiring.go where the client exists),
			// so HTTP views and the worker observe one registry.
			Webhooks: taskServices.webhooks,
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
		// Control Plane administration (issue #808). Every seam below is
		// shared with the platform-config handlers (no second ledger,
		// queue, or settings store); the control guard adds capability
		// auth, the separate rate-limit bucket, header-only idempotency,
		// the operator reason, and the mandatory audit record.
		// #809-close: Tasks rides the workspace task read model
		// (workspaceControlTaskAdapter over the shared workspace store +
		// global control queries; cancel settles through the shared
		// holder with the operator reason). Rewards, OAuth providers,
		// brand-logo overrides, and the AI gateway extras reuse their
		// domain handlers under the guard (single-sourced validation);
		// branding logo upload stores through the shared Storage
		// service. Monitor rides the workspace-backed statistics store
		// (task_daily_stats / task_stat_events / task_execution_attempts
		// over the workspace tasks table) plus the River queue monitor,
		// so the monitor / statistics / stream proxies are live.
		ContractControl: httpapi.ContractControlDeps{
			Users:             services.userService,
			Identity:          administration.NewIdentityAdminCommands(services.userService, services.authServiceClient),
			Impersonator:      services.authServiceClient,
			Credits:           services.creditService,
			CreditUsers:       creditUserResolver,
			CreditReads:       services.creditService,
			Reconciliation:    services.creditService,
			Tasks:             httpapi.NewWorkspaceControlTasks(services.dbPool, taskServices.workspaceTaskStore, taskServices.workspaceTaskHolder, creditUserResolver),
			Tenancy:           tenancypg.NewStore(repos.db),
			UserDirectory:     httpapi.NewUsersRepositoryDirectory(repos.userRepo),
			BillingAccounts:   tenancypg.NewBillingAccountStore(repos.db),
			Catalog:           services.billingCatalog,
			Payments:          services.paymentService,
			Settings:          services.runtimeSettings,
			PaymentWebhookURL: cfg.PublicOrigins.APIURL("/api/webhooks/stripe"),
			AIGateway:         aiGatewayHandler,
			Storage:           storagehttp.NewHandler(services.storageService),
			SearchProviders:   searchhttp.NewHandler(searchProviderConfig, searchinfra.NewProviderTestAdapters()...),
			FeaturePrompts:    prompts.NewFeaturePromptHandler(repos.featurePromptRepo),
			Monitor:           newControlMonitorHandler(repos.db),
			Mail:              services.authServiceClient,
			RewardCommands:    rewardhttp.NewAdminCommandHandler(services.rewardService),
			RewardReads:       rewardhttp.NewAdminReadHandler(services.rewardService, rewardIdentityProjector{repo: repos.userRepo}),
			OAuth:             services.authServiceClient,
			BrandLogos:        adminbranding.NewHandler(repos.adminBrandLogoRepo, controlRequireAdmin),
			BrandingUpload:    services.storageService,
		},
		PublicSearchHandler: searchhttp.NewSearchHandler(searchapplication.NewSearchService(searchProviderConfig, []searchports.SearchProviderAdapter{searchinfra.NewUnsplashAdapter(), searchinfra.NewPexelsAdapter()})),
		PaymentWebhook:      paymentWebhookHandler,
		Internal:            httpapi.InternalRouteDeps{UserHandler: identityhttp.NewInternalHandler(repos.userRepo, cfg.Auth.InternalAPIKey)},
		StaticUploads: httpapi.StaticUploadDeps{Auth: authDeps,
			BoardService: services.boardService,
			ReadAccess:   uploadReadAccess,
		},
		CORSMiddleware: httpinfra.BuildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
		RateLimit:      services.runtimeSettings,
		Branding:       services.runtimeSettings,
		REDMetrics:     redMetrics,
		ReadyChecker:   readyChecker,
	}, httpapi.FrontendRouteOptions{
		WarnMissingIndex: configinfra.IsProductionRuntime(cfg.App.Env),
	})

	return router, nil
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
	monitor := taskqueue.NewRiverQueue(nil, db, taskqueue.RiverMaxAttemptsDefault)
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
