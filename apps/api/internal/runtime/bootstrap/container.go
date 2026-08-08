package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/secrets"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type appContainer struct {
	router *gin.Engine

	db            *sql.DB
	riverPool     *pgxpool.Pool
	workerRuntime appWorkerRuntime
}

func newAppContainer(cfg *config.Config) (*appContainer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("application config is required")
	}
	if err := cfg.PublicOrigins.Validate(); err != nil {
		return nil, fmt.Errorf("invalid public origins: %w", err)
	}
	if err := secrets.LoadEncryptionKey(); err != nil {
		return nil, fmt.Errorf("failed to load secrets encryption key: %w", err)
	}
	if err := ensureRuntimeUploadDir(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	dbs, err := openAppDatabases(ctx, cfg)
	if err != nil {
		return nil, err
	}

	repos, err := newAppRepositories(ctx, dbs.db)
	if err != nil {
		closeStartupResources(dbs.db, dbs.riverPool)
		return nil, fmt.Errorf("failed to ensure user storage uid: %w", err)
	}

	services, err := newAppDomainServices(ctx, cfg, repos, dbs.riverPool)
	if err != nil {
		closeStartupResources(dbs.db, dbs.riverPool)
		return nil, err
	}

	taskServices, workerRuntime, err := newAppTaskRuntime(ctx, cfg, dbs, repos, services)
	if err != nil {
		closeStartupResources(dbs.db, dbs.riverPool)
		return nil, err
	}
	router := newAppRouter(cfg, repos, services, taskServices)

	return &appContainer{
		router:        router,
		db:            dbs.db,
		riverPool:     dbs.riverPool,
		workerRuntime: workerRuntime,
	}, nil
}

func (c *appContainer) close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	if err := c.workerRuntime.close(); err != nil {
		closeErr = err
	}
	if c.riverPool != nil {
		c.riverPool.Close()
	}
	if c.db != nil {
		if err := c.db.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}
