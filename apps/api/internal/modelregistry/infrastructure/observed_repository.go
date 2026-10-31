// Package infrastructure owns the modelregistry persistence adapters.
// The domain package keeps pure rules and narrow seams only; this package
// implements them over platform storage (raw SQL, no sqlc, no migration).
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/modelregistry/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservedRepository reads attempt facts with raw SQL: the aggregation needs
// no schema change and no new sqlc query, so generated code stays untouched.
// The status+created_at filter matches idx_task_execution_attempts_status_created_at.
// It implements domain.ObservedAttemptStore.
type ObservedRepository struct {
	db *pgxpool.Pool
}

// NewObservedRepository builds the repository over the runtime database.
func NewObservedRepository(db *pgxpool.Pool) *ObservedRepository {
	return &ObservedRepository{db: db}
}

type observedContextDB struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func (r *ObservedRepository) dbWithContext(ctx context.Context) observedContextDB {
	return observedContextDB{ctx: ctx, db: r.db}
}

func (q observedContextDB) Query(query string, args ...any) (pgx.Rows, error) {
	return q.db.Query(q.ctx, query, args...)
}

// ListAttemptObservations aggregates terminal (completed/failed) attempts
// created since the window start, grouped by resolved-or-requested model and
// provider, plus the per-model failure stage/category breakdown.
func (r *ObservedRepository) ListAttemptObservations(ctx context.Context, since time.Time) (domain.AttemptObservation, error) {
	if r == nil || r.db == nil {
		return domain.AttemptObservation{}, errors.New("model observation repository is unavailable")
	}
	aggregates, err := r.listAggregates(ctx, since)
	if err != nil {
		return domain.AttemptObservation{}, err
	}
	failures, err := r.listFailures(ctx, since)
	if err != nil {
		return domain.AttemptObservation{}, err
	}
	return domain.AttemptObservation{Aggregates: aggregates, Failures: failures}, nil
}

const listObservedAggregatesSQL = `
SELECT
  COALESCE(NULLIF(resolved_model, ''), requested_model) AS model,
  provider AS provider,
  service AS service,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'completed') AS completed,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed,
  COUNT(*) FILTER (WHERE cache_hit) AS cache_hits
FROM task_execution_attempts
WHERE created_at >= $1
  AND status IN ('completed', 'failed')
  AND (resolved_model <> '' OR requested_model <> '')
GROUP BY 1, 2, 3
ORDER BY 1 ASC, 2 ASC
`

func (r *ObservedRepository) listAggregates(ctx context.Context, since time.Time) ([]domain.ModelAttemptAggregate, error) {
	rows, err := r.dbWithContext(ctx).Query(listObservedAggregatesSQL, since)
	if err != nil {
		return nil, fmt.Errorf("list model attempt aggregates: %w", err)
	}
	defer rows.Close()
	aggregates := make([]domain.ModelAttemptAggregate, 0)
	for rows.Next() {
		var aggregate domain.ModelAttemptAggregate
		if err := rows.Scan(&aggregate.Model, &aggregate.Provider, &aggregate.Service, &aggregate.Total, &aggregate.Completed, &aggregate.Failed, &aggregate.CacheHits); err != nil {
			return nil, fmt.Errorf("scan model attempt aggregate: %w", err)
		}
		aggregates = append(aggregates, aggregate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read model attempt aggregates: %w", err)
	}
	return aggregates, nil
}

const listObservedFailuresSQL = `
SELECT
  COALESCE(NULLIF(resolved_model, ''), requested_model) AS model,
  provider AS provider,
  failure_stage AS failure_stage,
  failure_category AS failure_category,
  COUNT(*) AS total
FROM task_execution_attempts
WHERE created_at >= $1
  AND status = 'failed'
  AND (resolved_model <> '' OR requested_model <> '')
  AND failure_stage <> ''
GROUP BY 1, 2, 3, 4
ORDER BY 1 ASC, 2 ASC, 5 DESC, 3 ASC, 4 ASC
`

func (r *ObservedRepository) listFailures(ctx context.Context, since time.Time) ([]domain.ModelFailureCount, error) {
	rows, err := r.dbWithContext(ctx).Query(listObservedFailuresSQL, since)
	if err != nil {
		return nil, fmt.Errorf("list model failure breakdown: %w", err)
	}
	defer rows.Close()
	failures := make([]domain.ModelFailureCount, 0)
	for rows.Next() {
		var failure domain.ModelFailureCount
		if err := rows.Scan(&failure.Model, &failure.Provider, &failure.FailureStage, &failure.FailureCategory, &failure.Count); err != nil {
			return nil, fmt.Errorf("scan model failure breakdown: %w", err)
		}
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read model failure breakdown: %w", err)
	}
	return failures, nil
}
