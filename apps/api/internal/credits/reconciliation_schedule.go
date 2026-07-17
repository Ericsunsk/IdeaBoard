package credits

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

const ReconciliationScheduleJobKind = "credit_reconciliation_schedule"

const reconciliationScheduleContentionRetryDelay = time.Minute

type ReconciliationScheduleMode string

const (
	ReconciliationScheduleDaily   ReconciliationScheduleMode = "daily"
	ReconciliationScheduleCatchUp ReconciliationScheduleMode = "catch_up"
)

var reconciliationScheduleLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type ReconciliationScheduleJobArgs struct {
	Mode ReconciliationScheduleMode `json:"mode"`
}

func (ReconciliationScheduleJobArgs) Kind() string { return ReconciliationScheduleJobKind }

type reconciliationDailySchedule struct{}

func (reconciliationDailySchedule) Next(current time.Time) time.Time {
	local := current.In(reconciliationScheduleLocation)
	next := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, reconciliationScheduleLocation)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func ReconciliationPeriodicJobs() []*river.PeriodicJob {
	insertOpts := func() *river.InsertOpts {
		return &river.InsertOpts{
			Queue:       ReconciliationQueue,
			MaxAttempts: 5,
			Priority:    4,
		}
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			reconciliationDailySchedule{},
			func() (river.JobArgs, *river.InsertOpts) {
				return ReconciliationScheduleJobArgs{Mode: ReconciliationScheduleDaily}, insertOpts()
			},
			&river.PeriodicJobOpts{
				ID:         "credit-reconciliation-daily",
				RunOnStart: false,
			},
		),
		river.NewPeriodicJob(
			river.NeverSchedule(),
			func() (river.JobArgs, *river.InsertOpts) {
				return ReconciliationScheduleJobArgs{Mode: ReconciliationScheduleCatchUp}, insertOpts()
			},
			&river.PeriodicJobOpts{
				ID:         "credit-reconciliation-catch-up",
				RunOnStart: true,
			},
		),
	}
}

type reconciliationScheduleService interface {
	ScheduleReconciliation(context.Context, time.Time, bool) (*ReconciliationRun, error)
}

type ReconciliationScheduleWorker struct {
	river.WorkerDefaults[ReconciliationScheduleJobArgs]
	service reconciliationScheduleService
	now     func() time.Time
}

func NewReconciliationScheduleWorker(service reconciliationScheduleService) *ReconciliationScheduleWorker {
	return &ReconciliationScheduleWorker{service: service, now: time.Now}
}

func (w *ReconciliationScheduleWorker) Work(
	ctx context.Context,
	job *river.Job[ReconciliationScheduleJobArgs],
) error {
	if w == nil || w.service == nil || job == nil {
		return ErrInvalidCommand
	}
	onlyIfOverdue := false
	switch job.Args.Mode {
	case ReconciliationScheduleDaily:
	case ReconciliationScheduleCatchUp:
		onlyIfOverdue = true
	default:
		return ErrInvalidCommand
	}
	now := time.Now()
	if w.now != nil {
		now = w.now()
	}
	targetSlot := latestReconciliationScheduleSlot(now)
	run, err := w.service.ScheduleReconciliation(ctx, targetSlot, onlyIfOverdue)
	if err != nil || run == nil {
		return err
	}
	if run.Trigger == ReconciliationTriggerScheduled &&
		run.ScheduledFor != nil &&
		run.ScheduledFor.Equal(targetSlot) {
		return nil
	}
	return river.JobSnooze(reconciliationScheduleContentionRetryDelay)
}

func latestReconciliationScheduleSlot(current time.Time) time.Time {
	local := current.In(reconciliationScheduleLocation)
	slot := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, reconciliationScheduleLocation)
	if local.Before(slot) {
		slot = slot.AddDate(0, 0, -1)
	}
	return slot
}
