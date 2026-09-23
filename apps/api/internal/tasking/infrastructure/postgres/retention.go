package postgres

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
)

// PurgeExpiredTerminalTasks deletes terminal tasks older than cutoff after
// freezing their statistics. Work proceeds in bounded passes (one short
// transaction each: backfill up to batchSize dates, delete one batch of
// cancellations, delete one batch of tasks) and repeats until a pass deletes
// less than a full batch. Only terminal statuses are eligible;
// pending/running rows are never purged. The cutoff is fixed for the whole
// run, so every pass strictly shrinks the remainder and the loop terminates.
func (r *Repository) PurgeExpiredTerminalTasks(ctx context.Context, cutoff time.Time, batchSize int32) (retentioncontracts.PurgedTaskStats, error) {
	if cutoff.IsZero() {
		return retentioncontracts.PurgedTaskStats{}, fmt.Errorf("retention cutoff is required")
	}
	if batchSize <= 0 {
		return retentioncontracts.PurgedTaskStats{}, fmt.Errorf("retention batch size must be positive")
	}
	var total retentioncontracts.PurgedTaskStats
	for {
		pass, err := r.purgeExpiredTerminalTasksPass(ctx, cutoff, batchSize)
		if err != nil {
			return retentioncontracts.PurgedTaskStats{}, err
		}
		total.BackfilledDates += pass.BackfilledDates
		total.CancellationsDeleted += pass.CancellationsDeleted
		total.TasksDeleted += pass.TasksDeleted
		if pass.CancellationsDeleted < int64(batchSize) && pass.TasksDeleted < int64(batchSize) {
			return total, nil
		}
	}
}

// purgeExpiredTerminalTasksPass runs one bounded purge transaction.
func (r *Repository) purgeExpiredTerminalTasksPass(ctx context.Context, cutoff time.Time, batchSize int32) (retentioncontracts.PurgedTaskStats, error) {
	var stats retentioncontracts.PurgedTaskStats
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		dates, err := q.ListExpiredTerminalTaskDates(ctx, dbsqlc.ListExpiredTerminalTaskDatesParams{
			Cutoff:   dbutil.NullTime(&cutoff),
			RowLimit: batchSize,
		})
		if err != nil {
			return err
		}
		for _, date := range dates {
			statDate := normalizeTaskStatDate(dbutil.DateValue(date))
			dayStart := statDate
			dayEnd := statDate.AddDate(0, 0, 1)
			if _, err := q.BackfillDailyStatsByCreatedRange(ctx, dbsqlc.BackfillDailyStatsByCreatedRangeParams{
				StartedAt: dbutil.NullTime(&dayStart),
				EndedAt:   dbutil.NullTime(&dayEnd),
				StatDate:  dbutil.DateFromTime(statDate),
				MarkDone:  true,
			}); err != nil {
				return err
			}
			stats.BackfilledDates++
		}
		cancelled, err := q.DeleteExpiredTaskCancellations(ctx, dbsqlc.DeleteExpiredTaskCancellationsParams{
			Cutoff:    dbutil.NullTime(&cutoff),
			BatchSize: batchSize,
		})
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
