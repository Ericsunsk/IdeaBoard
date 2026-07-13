package taskstore

import (
	"context"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

// UsageReader is the read-only Tasking seam for product usage views. A Task
// is the logical user action, so provider attempts are intentionally excluded.
type UsageReader interface {
	GetUsageSummary(context.Context, int64, string) (UsageSummary, error)
}

type UsageSummary struct {
	TaskCount  int64
	ActiveDays int64
}

func (r *Repository) GetUsageSummary(ctx context.Context, userID int64, timezone string) (UsageSummary, error) {
	if r == nil || r.q == nil {
		return UsageSummary{}, fmt.Errorf("taskstore: usage query unavailable")
	}
	if userID <= 0 || strings.TrimSpace(timezone) == "" {
		return UsageSummary{}, fmt.Errorf("taskstore: invalid usage query")
	}
	row, err := r.q.GetTaskUsageSummary(ctx, dbsqlc.GetTaskUsageSummaryParams{
		UserID:   userID,
		Timezone: timezone,
	})
	if err != nil {
		return UsageSummary{}, fmt.Errorf("taskstore: aggregate usage summary: %w", err)
	}
	return UsageSummary{TaskCount: row.TaskCount, ActiveDays: row.ActiveDays}, nil
}
