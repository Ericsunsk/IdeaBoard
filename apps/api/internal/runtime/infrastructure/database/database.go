package database

import (
	"context"

	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pools holds runtime-owned database pool references.
type Pools struct {
	Pool      *pgxpool.Pool
	RiverPool *pgxpool.Pool
}

// Close gracefully releases both PostgreSQL and River pools.
func (p *Pools) Close() {
	if p == nil {
		return
	}
	if p.RiverPool != nil {
		p.RiverPool.Close()
	}
	if p.Pool != nil {
		p.Pool.Close()
	}
}

// HealthChecker verifies readiness of runtime database connections.
type HealthChecker struct {
	pool      *pgxpool.Pool
	riverPool *pgxpool.Pool
}

// NewHealthChecker creates a HealthChecker backed by runtime database pools.
func NewHealthChecker(pools *Pools) *HealthChecker {
	if pools == nil {
		return &HealthChecker{}
	}
	return &HealthChecker{pool: pools.Pool, riverPool: pools.RiverPool}
}

// CheckHealth queries both Postgres and River pools to report readiness.
func (h *HealthChecker) CheckHealth(ctx context.Context) domain.ComponentStatus {
	if h == nil || h.pool == nil || h.riverPool == nil {
		return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: false, Required: true, Detail: "database pools are not initialized"}
	}
	if err := h.pool.Ping(ctx); err != nil {
		return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: false, Required: true, Detail: err.Error()}
	}
	if err := h.riverPool.Ping(ctx); err != nil {
		return domain.ComponentStatus{Name: "river", Kind: domain.ComponentKindRiver, Ready: false, Required: true, Detail: err.Error()}
	}
	return domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: true, Required: true}
}

var _ ports.HealthChecker = (*HealthChecker)(nil)
