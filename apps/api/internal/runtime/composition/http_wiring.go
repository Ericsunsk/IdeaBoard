package composition

import (
	"fmt"
	"log/slog"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/identity/users/application/administration"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/library"
	storageauth "ideaboard/api/internal/media/storage/application/authorization"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/prompts"
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
	taskservice "ideaboard/api/internal/tasking/application/task"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
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
				Quoter:  newLibraryMaterializeQuoter(services.billingCatalog),
				Billing: newLibraryMaterializeSettler(services.creditService),
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
		},
		// Control Plane administration (issue #808). Every seam below is
		// shared with the platform-config handlers (no second ledger,
		// queue, or settings store); the control guard adds capability
		// auth, the separate rate-limit bucket, header-only idempotency,
		// the operator reason, and the mandatory audit record.
		// #809 hard cut: Tasks and Monitor stay nil (the legacy task
		// services are deleted with the stub), so the control task
		// read/ops routes fail closed with 503 and the monitor/
		// statistics proxies with 501 until the workspace task read
		// model lands. Routes stay registered (openapi operationIds
		// intact).
		ContractControl: httpapi.ContractControlDeps{
			Users:             services.userService,
			Identity:          administration.NewIdentityAdminCommands(services.userService, services.authServiceClient),
			Impersonator:      services.authServiceClient,
			Credits:           services.creditService,
			CreditUsers:       creditUserResolver,
			CreditReads:       services.creditService,
			Reconciliation:    services.creditService,
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
			Mail:              services.authServiceClient,
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

func registerHTTPRoutes(r *gin.Engine, deps httpapi.Dependencies, frontendOptions httpapi.FrontendRouteOptions) {
	frontendFS, err := webdist.DistFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend dist", "err", err)
	}

	httpapi.RegisterRoutes(r, deps, frontendFS, frontendOptions)
}
