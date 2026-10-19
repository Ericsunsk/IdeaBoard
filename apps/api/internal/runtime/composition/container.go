package composition

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/runtime/application/lifecycle"
	"ideaboard/api/internal/runtime/application/orchestration"
	"ideaboard/api/internal/runtime/composition/wiring"
	configinfra "ideaboard/api/internal/runtime/infrastructure/config"
	databaseinfra "ideaboard/api/internal/runtime/infrastructure/database"
	httpinfra "ideaboard/api/internal/runtime/infrastructure/http"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	natsinfra "ideaboard/api/internal/runtime/infrastructure/nats"
	"ideaboard/api/internal/runtime/ports"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type appContainer struct {
	router *gin.Engine

	db             *pgxpool.Pool
	riverPool      *pgxpool.Pool
	workerRuntime  wiring.AppWorkerRuntime
	healthCheckers []ports.HealthChecker
}

func newAppContainer(cfg *config.Config) (*appContainer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("application config is required")
	}
	if err := cfg.PublicOrigins.Validate(); err != nil {
		return nil, fmt.Errorf("invalid public origins: %w", err)
	}
	if err := secrets.LoadEncryptionKeyFromValue(cfg.Secrets.EncryptionKey); err != nil {
		return nil, fmt.Errorf("failed to load secrets encryption key: %w", err)
	}
	if err := configinfra.EnsureRuntimeUploadDir(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	dbs, err := wiring.OpenAppDatabases(ctx, cfg)
	if err != nil {
		return nil, err
	}

	repos, err := wiring.NewAppRepositories(ctx, dbs.DB)
	if err != nil {
		wiring.CloseStartupResources(dbs.DB, dbs.RiverPool)
		return nil, fmt.Errorf("failed to ensure user storage uid: %w", err)
	}

	services, err := wiring.NewAppDomainServices(ctx, cfg, repos, dbs.RiverPool)
	if err != nil {
		wiring.CloseStartupResources(dbs.DB, dbs.RiverPool)
		return nil, err
	}

	taskServices, workerRuntime, err := wiring.NewAppTaskRuntime(ctx, cfg, dbs, repos, services, redmetrics.DefaultSet())
	if err != nil {
		wiring.CloseStartupResources(dbs.DB, dbs.RiverPool)
		return nil, err
	}
	dbHealthChecker := databaseinfra.NewHealthChecker(&databaseinfra.Pools{Pool: dbs.DB, RiverPool: dbs.RiverPool})
	// ADR-0021: a worker-only process never builds the HTTP router. Every
	// other mode needs it for API/SSE traffic.
	var router *gin.Engine
	if cfg.Mode.ServesHTTP() {
		router, err = wiring.NewAppRouter(cfg, repos, services, taskServices, dbHealthChecker, redmetrics.DefaultSet())
		if err != nil {
			wiring.CloseStartupResources(dbs.DB, dbs.RiverPool)
			return nil, err
		}
	}
	natsHealthChecker := natsinfra.NewEventBusFromUnderlying(workerRuntime.EventBus())

	return &appContainer{
		router:        router,
		db:            dbs.DB,
		riverPool:     dbs.RiverPool,
		workerRuntime: workerRuntime,
		healthCheckers: []ports.HealthChecker{
			dbHealthChecker,
			workerRuntime,
			natsHealthChecker,
			// HTTP runners are added after construction in BuildApp, once their
			// concrete listeners are available.
		},
	}, nil
}

// BuildApp initializes all dependencies and constructs a runtime App.
func BuildApp(cfg *config.Config) (*App, error) {
	container, err := newAppContainer(cfg)
	if err != nil {
		return nil, err
	}
	app := &App{
		coordinator:    lifecycle.NewCoordinator(lifecycle.DefaultShutdownBudget),
		workerRuntime:  container.workerRuntime,
		healthCheckers: container.healthCheckers,
		closer:         container,
	}
	// ADR-0021: a worker-only process binds no HTTP listeners at all (no
	// API routes, no metrics endpoint); its runners stay nil and are
	// skipped by startup/shutdown below.
	if cfg.Mode.ServesHTTP() {
		httpServer := httpinfra.NewHTTPServerRunner(cfg, container.router)
		metricsServer := httpinfra.NewMetricsServerRunner(cfg)
		app.httpServer = httpServer
		app.metricsServer = metricsServer
		app.healthCheckers = append(app.healthCheckers, httpServer, metricsServer)
	}
	return app, nil
}

// App represents the process runtime root adhering to platform lifecycle invariants.
type App struct {
	coordinator    *lifecycle.Coordinator
	httpServer     ports.Runner
	metricsServer  ports.Runner
	workerRuntime  wiring.AppWorkerRuntime
	healthCheckers []ports.HealthChecker
	closer         *appContainer
}

// Start executes the ordered startup sequence and marks runtime ready.
func (a *App) Start(ctx context.Context) error {
	return orchestration.ExecuteStartup(ctx, a.coordinator, orchestration.StartupDependencies{
		Runners:        a.startupRunners(),
		HealthCheckers: a.healthCheckers,
	})
}

// startupRunners lists runners in dependency order: workers first, then
// HTTP listeners. Nil listeners (worker-only mode, ADR-0021) are skipped,
// so a worker process starts only the River runtime while an http-only
// process still starts it without River consumption (enqueue-only).
func (a *App) startupRunners() []ports.Runner {
	runners := []ports.Runner{a.workerRuntime}
	if a.httpServer != nil {
		runners = append(runners, a.httpServer)
	}
	if a.metricsServer != nil {
		runners = append(runners, a.metricsServer)
	}
	return runners
}

// shutdownRunners reverses startup order, preserving the pre-split drain
// sequence (workers stop before listeners release).
func (a *App) shutdownRunners() []ports.Runner {
	started := a.startupRunners()
	runners := make([]ports.Runner, 0, len(started))
	for i := len(started) - 1; i >= 0; i-- {
		runners = append(runners, started[i])
	}
	return runners
}

// WaitForShutdownSignal blocks until an OS termination signal is caught.
func (a *App) WaitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

// Shutdown gracefully stops runners within the shutdown budget.
func (a *App) Shutdown(ctx context.Context) error {
	return a.coordinator.DrainAndShutdown(ctx, a.shutdownRunners())
}

// Close releases database connections and underlying system resources.
func (a *App) Close() error {
	if a == nil || a.closer == nil {
		return nil
	}
	return a.coordinator.StopResources(a.closer)
}

func (c *appContainer) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	if err := c.workerRuntime.Close(); err != nil {
		closeErr = err
	}
	if c.riverPool != nil {
		c.riverPool.Close()
	}
	if c.db != nil {
		c.db.Close()
	}
	return closeErr
}

func configureRuntimeLogging(cfg *config.Config) {
	configinfra.ConfigureRuntimeLogging(cfg)
}

func requireAuthInternalKeyForProduction(cfg *config.Config) {
	if err := configinfra.ValidateAuthInternalKey(cfg); err != nil {
		fatal(err.Error())
	}
}
