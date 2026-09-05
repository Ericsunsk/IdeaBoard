package database

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

type HealthChecker struct {
	db        *sql.DB
	riverPool *pgxpool.Pool
}

func NewHealthChecker(pools *Pools) *HealthChecker {
	if pools == nil {
		return &HealthChecker{}
	}
	return &HealthChecker{db: pools.DB, riverPool: pools.RiverPool}
}

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
