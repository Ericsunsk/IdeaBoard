package postgres

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
)

// PurgeExpiredTerminalTasks deletes terminal tasks older than cutoff in one
// transaction: freeze statistics first, then task_cancellations (no FK),
// then tasks (metrics/attempts/outputs follow via ON DELETE CASCADE).
// Only terminal statuses are eligible; pending/running rows are never purged.
func (r *Repository) PurgeExpiredTerminalTasks(ctx context.Context, cutoff time.Time, batchSize int32) (retentioncontracts.PurgedTaskStats, error) {
	if cutoff.IsZero() {
		return retentioncontracts.PurgedTaskStats{}, fmt.Errorf("retention cutoff is required")
	}
	if batchSize <= 0 {
		return retentioncontracts.PurgedTaskStats{}, fmt.Errorf("retention batch size must be positive")
	}
	var stats retentioncontracts.PurgedTaskStats
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		dates, err := q.ListExpiredTerminalTaskDates(ctx, dbutil.NullTime(&cutoff))
		if err != nil {
			return err
		}
		for _, date := range dates {
			statDate := normalizeTaskStatDate(date)
			dayStart := statDate
			dayEnd := statDate.AddDate(0, 0, 1)
			if _, err := q.BackfillDailyStatsByCreatedRange(ctx, dbsqlc.BackfillDailyStatsByCreatedRangeParams{
				StartedAt: dbutil.NullTime(&dayStart),
				EndedAt:   dbutil.NullTime(&dayEnd),
				StatDate:  statDate,
				MarkDone:  true,
			}); err != nil {
				return err
			}
			stats.BackfilledDates++
		}
		cancelled, err := q.DeleteExpiredTaskCancellations(ctx, dbutil.NullTime(&cutoff))
		if err != nil {
			return err
		}
		stats.CancellationsDeleted = cancelled
		deleted, err := q.DeleteExpiredTasks(ctx, dbsqlc.DeleteExpiredTasksParams{
			Cutoff:    dbutil.NullTime(&cutoff),
			BatchSize: batchSize,
		})
		if err != nil {
			return err
		}
		stats.TasksDeleted = deleted
		return nil
	})
	return stats, err
}
