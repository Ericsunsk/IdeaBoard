package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/platform/riverpolicy"
	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"

	"github.com/riverqueue/river"
)

const (
	// QueueName is a single-worker queue: purge runs are serial by design.
	// It aliases the Platform-owned policy constant so inserts track the
	// matrix with no copied literal.
	QueueName = riverpolicy.RiverQueueTaskRetention
	// The purge Timeout lives in the River timeout matrix
	// (riverpolicy.RiverJobTimeoutTaskRetention).
)

// retentionLocation matches the other daily maintenance jobs so operators
// see one maintenance window in Asia/Shanghai time.
var retentionLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type taskRetentionScheduleJobArgs struct{}

func (taskRetentionScheduleJobArgs) Kind() string { return "task_retention_schedule" }

type retentionService interface {
	PurgeExpiredTasks(ctx context.Context, retentionDays int) (retentioncontracts.PurgedTaskStats, error)
}

type TaskRetentionWorker struct {
	river.WorkerDefaults[taskRetentionScheduleJobArgs]
	service       retentionService
	retentionDays int
}

func NewTaskRetentionWorker(service retentionService, retentionDays int) *TaskRetentionWorker {
	return &TaskRetentionWorker{service: service, retentionDays: retentionDays}
}

func (w *TaskRetentionWorker) Work(ctx context.Context, job *river.Job[taskRetentionScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid task retention job")
	}
	result, err := w.service.PurgeExpiredTasks(ctx, w.retentionDays)
	if err != nil {
		return err
	}
	slog.Info("task retention purge completed",
		"retention_days", w.retentionDays,
		"backfilled_dates", result.BackfilledDates,
		"tasks_deleted", result.TasksDeleted,
		"cancellations_deleted", result.CancellationsDeleted,
	)
	return nil
}

func (*TaskRetentionWorker) Timeout(*river.Job[taskRetentionScheduleJobArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutTaskRetention
}

type taskRetentionDailySchedule struct{}

func (taskRetentionDailySchedule) Next(current time.Time) time.Time {
	local := current.In(retentionLocation)
	next := time.Date(local.Year(), local.Month(), local.Day(), 3, 30, 0, 0, retentionLocation)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func PeriodicJobs() []*river.PeriodicJob {
	insertOpts := func() *river.InsertOpts {
		return &river.InsertOpts{Queue: QueueName, MaxAttempts: riverpolicy.RiverMaxAttemptsTaskRetention, Priority: 4}
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			taskRetentionDailySchedule{},
			func() (river.JobArgs, *river.InsertOpts) {
				return taskRetentionScheduleJobArgs{}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "task-retention-daily", RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NeverSchedule(),
			func() (river.JobArgs, *river.InsertOpts) {
				return taskRetentionScheduleJobArgs{}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "task-retention-catch-up", RunOnStart: true},
		),
	}
}
