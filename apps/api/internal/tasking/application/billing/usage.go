package billing

import (
	"context"
	"fmt"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageStore reads immutable Task Billing submission snapshots. Snapshot rows
// survive user-visible Task deletion and remain the logical AI invocation fact.
type UsageStore struct {
	db *pgxpool.Pool
}

type contextQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func NewUsageStore(db *pgxpool.Pool) *UsageStore {
	return &UsageStore{db: db}
}

func (s *UsageStore) dbWithContext(ctx context.Context) contextQueryer {
	return contextQueryer{ctx: ctx, db: s.db}
}

func (q contextQueryer) QueryRow(query string, args ...any) pgx.Row {
	return q.db.QueryRow(q.ctx, query, args...)
}

func (s *UsageStore) GetTaskUsage(
	ctx context.Context,
	billingAccountID int64,
	filter creditsdomain.UsageFilter,
	location *time.Location,
) (taskCount int64, activeDays int64, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("task billing usage store: database unavailable")
	}
	if billingAccountID <= 0 || !filter.Valid() || location == nil {
		return 0, 0, fmt.Errorf("task billing usage store: invalid query")
	}
	err = s.dbWithContext(ctx).QueryRow(`
SELECT COUNT(*)::bigint,
       COUNT(DISTINCT (created_at AT TIME ZONE $2)::date)::bigint
FROM task_billing_snapshots
WHERE billing_account_id = $1
  AND ($3::bigint = 0 OR workspace_id = $3)
  AND ($4::bigint = 0 OR user_id = $4)`, billingAccountID, location.String(), filter.WorkspaceID, filter.ActorUserID).Scan(&taskCount, &activeDays)
	if err != nil {
		return 0, 0, fmt.Errorf("task billing usage store: aggregate snapshots: %w", err)
	}
	return taskCount, activeDays, nil
}
