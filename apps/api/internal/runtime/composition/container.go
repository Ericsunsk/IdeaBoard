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
	router, err := wiring.NewAppRouter(cfg, repos, services, taskServices, dbHealthChecker, redmetrics.DefaultSet())
	if err != nil {
		wiring.CloseStartupResources(dbs.DB, dbs.RiverPool)
		return nil, err
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
	httpServer := httpinfra.NewHTTPServerRunner(cfg, container.router)
	metricsServer := httpinfra.NewMetricsServerRunner(cfg)
	return &App{
		coordinator:    lifecycle.NewCoordinator(lifecycle.DefaultShutdownBudget),
		httpServer:     httpServer,
		metricsServer:  metricsServer,
		workerRuntime:  container.workerRuntime,
		healthCheckers: append(container.healthCheckers, httpServer, metricsServer),
		closer:         container,
	}, nil
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
		Runners: []ports.Runner{
			a.workerRuntime,
			a.httpServer,
			a.metricsServer,
		},
		HealthCheckers: a.healthCheckers,
	})
}

// WaitForShutdownSignal blocks until an OS termination signal is caught.
func (a *App) WaitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

// Shutdown gracefully stops runners within the shutdown budget.
func (a *App) Shutdown(ctx context.Context) error {
	return a.coordinator.DrainAndShutdown(ctx, []ports.Runner{
		a.metricsServer,
		a.httpServer,
		a.workerRuntime,
	})
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
