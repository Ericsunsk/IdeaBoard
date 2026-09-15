package database

import (
	"context"
	"database/sql"
	"errors"

	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pools holds runtime-owned database pool references.
type Pools struct {
	DB        *sql.DB
	RiverPool *pgxpool.Pool
}

// Close gracefully releases both PostgreSQL and River pools.
func (p *Pools) Close() error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.RiverPool != nil {
		p.RiverPool.Close()
	}
	if p.DB != nil {
		if err := p.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// HealthChecker verifies readiness of runtime database connections.
type HealthChecker struct {
	db        *sql.DB
	riverPool *pgxpool.Pool
}

// NewHealthChecker creates a HealthChecker backed by runtime database pools.
func NewHealthChecker(pools *Pools) *HealthChecker {
	if pools == nil {
		return &HealthChecker{}
	}
	return &HealthChecker{db: pools.DB, riverPool: pools.RiverPool}
}

// CheckHealth queries both Postgres and River pools to report readiness.
func (h *HealthChecker) CheckHealth(ctx context.Context) domain.ComponentStatus {
	if h == nil || h.db == nil || h.riverPool == nil {
		return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: false, Required: true, Detail: "database pools are not initialized"}
	}
	if err := h.db.PingContext(ctx); err != nil {
		return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: false, Required: true, Detail: err.Error()}
	}
	if err := h.riverPool.Ping(ctx); err != nil {
		return domain.ComponentStatus{Name: "river", Kind: domain.ComponentKindRiver, Ready: false, Required: true, Detail: err.Error()}
	}
	return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: true, Required: true}
}

var _ ports.HealthChecker = (*HealthChecker)(nil)
