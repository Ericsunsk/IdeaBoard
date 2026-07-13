package taskbilling

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UsageStore reads immutable Task Billing submission snapshots. Snapshot rows
// survive user-visible Task deletion and remain the logical AI invocation fact.
type UsageStore struct {
	db *sql.DB
}

type contextQueryer struct {
	ctx context.Context
	db  *sql.DB
}

func NewUsageStore(db *sql.DB) *UsageStore {
	return &UsageStore{db: db}
}

func (s *UsageStore) dbWithContext(ctx context.Context) contextQueryer {
	return contextQueryer{ctx: ctx, db: s.db}
}

func (q contextQueryer) QueryRow(query string, args ...any) *sql.Row {
	return q.db.QueryRowContext(q.ctx, query, args...)
}

func (s *UsageStore) GetTaskUsage(
	ctx context.Context,
	userID int64,
	location *time.Location,
) (taskCount int64, activeDays int64, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("task billing usage store: database unavailable")
	}
	if userID <= 0 || location == nil {
		return 0, 0, fmt.Errorf("task billing usage store: invalid query")
	}
	err = s.dbWithContext(ctx).QueryRow(`
SELECT COUNT(*)::bigint,
       COUNT(DISTINCT (created_at AT TIME ZONE $2)::date)::bigint
FROM task_billing_snapshots
WHERE user_id = $1`, userID, location.String()).Scan(&taskCount, &activeDays)
	if err != nil {
		return 0, 0, fmt.Errorf("task billing usage store: aggregate snapshots: %w", err)
	}
	return taskCount, activeDays, nil
}
