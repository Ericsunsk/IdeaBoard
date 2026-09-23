package billing

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const CreditReconciliationQueue = "credit_reconciliation"

const (
	creditReconciliationTriggerLookupTimeout = 2 * time.Second
	creditReconciliationFinalFailureTimeout  = 5 * time.Second
	creditReconciliationContentionRetryDelay = time.Minute
)

type creditReconciliationJobArgs struct {
	RunID int64 `json:"run_id"`
}

func (creditReconciliationJobArgs) Kind() string { return "credit_reconciliation" }

type CreditReconciliationDispatcher struct {
	client *river.Client[pgx.Tx]
}

func NewCreditReconciliationDispatcher(client *river.Client[pgx.Tx]) (*CreditReconciliationDispatcher, error) {
	if client == nil {
		return nil, fmt.Errorf("configure Credit reconciliation dispatcher: River client is nil")
	}
	return &CreditReconciliationDispatcher{client: client}, nil
}

func (d *CreditReconciliationDispatcher) DispatchReconciliationRun(
	ctx context.Context,
	tx pgx.Tx,
	runID int64,
) error {
	if d == nil || d.client == nil || tx == nil || runID <= 0 {
		return domain.ErrInvalidCommand
	}
	_, err := d.client.InsertTx(ctx, tx, creditReconciliationJobArgs{RunID: runID}, &river.InsertOpts{
		Queue: CreditReconciliationQueue, MaxAttempts: taskqueue.RiverMaxAttemptsCreditReconciliation, Priority: 4,
	})
	return err
}

type creditReconciliationWorkerService interface {
	ExecuteReconciliation(context.Context, int64) error
	FailReconciliation(context.Context, int64, error) error
	GetReconciliationRun(context.Context, int64) (ports.ReconciliationRun, error)
}

type CreditReconciliationWorker struct {
	river.WorkerDefaults[creditReconciliationJobArgs]
	service creditReconciliationWorkerService
}

func NewCreditReconciliationWorker(service creditReconciliationWorkerService) *CreditReconciliationWorker {
	return &CreditReconciliationWorker{service: service}
}

func (*CreditReconciliationWorker) Timeout(*river.Job[creditReconciliationJobArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutCreditReconciliation
}

func (w *CreditReconciliationWorker) Work(ctx context.Context, job *river.Job[creditReconciliationJobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.RunID <= 0 {
		return domain.ErrInvalidCommand
	}
	err := w.service.ExecuteReconciliation(ctx, job.Args.RunID)
	if err == nil {
		return nil
	}
	w.logReconciliationFailure(ctx, job, err)
	if job.Attempt < job.MaxAttempts {
		return err
	}
	return w.failReconciliation(ctx, job, err)
}

func (w *CreditReconciliationWorker) logReconciliationFailure(ctx context.Context, job *river.Job[creditReconciliationJobArgs], err error) {
	lookupCtx, cancelLookup := context.WithTimeout(context.WithoutCancel(ctx), creditReconciliationTriggerLookupTimeout)
	defer cancelLookup()
	run, runErr := w.service.GetReconciliationRun(lookupCtx, job.Args.RunID)
	if runErr == nil && run.Trigger == ports.ReconciliationTriggerScheduled {
		slog.Error("scheduled Credit reconciliation execution failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
		)
		return
	}
	if runErr != nil {
		slog.Error("Credit reconciliation trigger lookup failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
			"trigger_lookup_err", runErr,
		)
	}
}

func (w *CreditReconciliationWorker) failReconciliation(ctx context.Context, job *river.Job[creditReconciliationJobArgs], err error) error {
	failCtx, cancelFail := context.WithTimeout(context.WithoutCancel(ctx), creditReconciliationFinalFailureTimeout)
	defer cancelFail()
	if failErr := w.service.FailReconciliation(failCtx, job.Args.RunID, err); failErr != nil {
		slog.Error("Credit reconciliation final failure persistence failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
			"mark_failed_err", failErr,
		)
		return errors.Join(err, failErr)
	}
	return nil
}

type creditReconciliationScheduleMode string

const (
	creditReconciliationScheduleDaily   creditReconciliationScheduleMode = "daily"
	creditReconciliationScheduleCatchUp creditReconciliationScheduleMode = "catch_up"
)

var creditReconciliationScheduleLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type creditReconciliationScheduleJobArgs struct {
	Mode creditReconciliationScheduleMode `json:"mode"`
}

func (creditReconciliationScheduleJobArgs) Kind() string { return "credit_reconciliation_schedule" }

type creditReconciliationDailySchedule struct{}

func (creditReconciliationDailySchedule) Next(current time.Time) time.Time {
	local := current.In(creditReconciliationScheduleLocation)
	next := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, creditReconciliationScheduleLocation)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func CreditReconciliationPeriodicJobs() []*river.PeriodicJob {
	insertOpts := func() *river.InsertOpts {
		return &river.InsertOpts{Queue: CreditReconciliationQueue, MaxAttempts: taskqueue.RiverMaxAttemptsCreditReconciliation, Priority: 4}
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			creditReconciliationDailySchedule{},
			func() (river.JobArgs, *river.InsertOpts) {
				return creditReconciliationScheduleJobArgs{Mode: creditReconciliationScheduleDaily}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "credit-reconciliation-daily", RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NeverSchedule(),
			func() (river.JobArgs, *river.InsertOpts) {
				return creditReconciliationScheduleJobArgs{Mode: creditReconciliationScheduleCatchUp}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "credit-reconciliation-catch-up", RunOnStart: true},
		),
	}
}

type creditReconciliationScheduleService interface {
	ScheduleReconciliation(context.Context, time.Time, bool) (*ports.ReconciliationRun, error)
}

type CreditReconciliationScheduleWorker struct {
	river.WorkerDefaults[creditReconciliationScheduleJobArgs]
	service creditReconciliationScheduleService
	now     func() time.Time
}

func NewCreditReconciliationScheduleWorker(service creditReconciliationScheduleService) *CreditReconciliationScheduleWorker {
	return &CreditReconciliationScheduleWorker{service: service, now: time.Now}
}

func (*CreditReconciliationScheduleWorker) Timeout(*river.Job[creditReconciliationScheduleJobArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutCreditSchedule
}

func (w *CreditReconciliationScheduleWorker) Work(
	ctx context.Context,
	job *river.Job[creditReconciliationScheduleJobArgs],
) error {
	if w == nil || w.service == nil || job == nil {
		return domain.ErrInvalidCommand
	}
	onlyIfOverdue, err := w.parseScheduleMode(job.Args.Mode)
	if err != nil {
		return err
	}
	targetSlot := latestCreditReconciliationScheduleSlot(w.nowTime())
	run, err := w.service.ScheduleReconciliation(ctx, targetSlot, onlyIfOverdue)
	if err != nil || run == nil {
		return err
	}
	if isMatchingScheduledRun(run, targetSlot) {
		return nil
	}
	return river.JobSnooze(creditReconciliationContentionRetryDelay)
}

func (w *CreditReconciliationScheduleWorker) parseScheduleMode(mode creditReconciliationScheduleMode) (bool, error) {
	switch mode {
	case creditReconciliationScheduleDaily:
		return false, nil
	case creditReconciliationScheduleCatchUp:
		return true, nil
	default:
		return false, domain.ErrInvalidCommand
	}
}

func (w *CreditReconciliationScheduleWorker) nowTime() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func isMatchingScheduledRun(run *ports.ReconciliationRun, targetSlot time.Time) bool {
	return run != nil &&
		run.Trigger == ports.ReconciliationTriggerScheduled &&
		run.ScheduledFor != nil &&
		run.ScheduledFor.Equal(targetSlot)
}

func latestCreditReconciliationScheduleSlot(current time.Time) time.Time {
	local := current.In(creditReconciliationScheduleLocation)
	slot := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, creditReconciliationScheduleLocation)
	if local.Before(slot) {
		slot = slot.AddDate(0, 0, -1)
	}
	return slot
}
