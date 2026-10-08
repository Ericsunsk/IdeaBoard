package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"ideaboard/api/internal/platform/riverpolicy"
)

// IdentityReconciliationQueue is the serial maintenance queue for the
// Identity admin reconciliation job. MaxWorkers is 1 at registration:
// overlapping scans would contend on the same finding fingerprints for no
// throughput gain (admin surfaces are small). The name is an alias of the
// Platform-owned policy constant so inserts track the matrix with no copied
// literal.
const IdentityReconciliationQueue = riverpolicy.RiverQueueIdentityAdminReconciliation

const riverPriority = 4

type executeArgs struct {
	RunID int64 `json:"run_id"`
}

// Kind implements river.JobArgs.
func (executeArgs) Kind() string { return "identity_admin_reconciliation" }

// RunDispatcher enqueues an execute job inside the schedule transaction,
// so a scheduled run always has its execute job (transactional enqueue,
// mirroring the credits reconciliation dispatcher).
type RunDispatcher interface {
	DispatchReconciliationRun(ctx context.Context, tx pgx.Tx, runID int64) error
}

type riverDispatcher struct {
	client *river.Client[pgx.Tx]
}

// NewRunDispatcher builds the transactional dispatcher. A nil client is a
// wiring error, not a runtime fallback.
func NewRunDispatcher(client *river.Client[pgx.Tx]) (RunDispatcher, error) {
	if client == nil {
		return nil, fmt.Errorf("configure Identity admin reconciliation dispatcher: River client is nil")
	}
	return &riverDispatcher{client: client}, nil
}

// DispatchReconciliationRun implements RunDispatcher.
func (d *riverDispatcher) DispatchReconciliationRun(ctx context.Context, tx pgx.Tx, runID int64) error {
	if d == nil || d.client == nil || tx == nil || runID <= 0 {
		return errInvalidReconciliationCommand
	}
	_, err := d.client.InsertTx(ctx, tx, executeArgs{RunID: runID}, &river.InsertOpts{
		Queue: IdentityReconciliationQueue, MaxAttempts: riverpolicy.RiverMaxAttemptsIdentityAdminReconciliation, Priority: riverPriority,
	})
	return err
}

type executeService interface {
	ExecuteReconciliation(context.Context, int64) error
	RepairRoleDemotions(context.Context, int64) error
	FailReconciliation(context.Context, int64, error) error
	GetReconciliationRun(context.Context, int64) (Run, error)
}

// ExecuteWorker runs one scan and then repairs only local-user/Auth-admin
// role drift. All other findings remain dry-run and require review.
type ExecuteWorker struct {
	river.WorkerDefaults[executeArgs]
	service executeService
}

// NewExecuteWorker builds the execute worker.
func NewExecuteWorker(service executeService) *ExecuteWorker {
	return &ExecuteWorker{service: service}
}

// Timeout implements river.JobArgsWithTimeout.
func (*ExecuteWorker) Timeout(*river.Job[executeArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutIdentityAdminReconciliation
}

// Work implements river.Worker. A failed scan retries with the job (up to
// MaxAttempts); only the final failure marks the run failed, so a
// concurrently completed run is never regressed.
func (w *ExecuteWorker) Work(ctx context.Context, job *river.Job[executeArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.RunID <= 0 {
		return errInvalidReconciliationCommand
	}
	err := w.service.ExecuteReconciliation(ctx, job.Args.RunID)
	if err != nil {
		w.logFailure(ctx, job, err)
		if job.Attempt < job.MaxAttempts {
			return err
		}
		return w.failRun(ctx, job, err)
	}
	if err := w.service.RepairRoleDemotions(ctx, job.Args.RunID); err != nil {
		w.logFailure(ctx, job, err)
		if job.Attempt < job.MaxAttempts {
			return err
		}
		return w.failRun(ctx, job, err)
	}
	return nil
}

func (w *ExecuteWorker) logFailure(ctx context.Context, job *river.Job[executeArgs], err error) {
	lookupCtx, cancelLookup := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancelLookup()
	run, runErr := w.service.GetReconciliationRun(lookupCtx, job.Args.RunID)
	if runErr == nil && run.Trigger == runTriggerScheduled {
		slog.Error("scheduled Identity admin reconciliation execution failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
		)
		return
	}
	slog.Error("Identity admin reconciliation trigger lookup failed",
		"run_id", job.Args.RunID,
		"attempt", job.Attempt,
		"max_attempts", job.MaxAttempts,
		"err", err,
		"trigger_lookup_err", runErr,
	)
}

func (w *ExecuteWorker) failRun(ctx context.Context, job *river.Job[executeArgs], err error) error {
	failCtx, cancelFail := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFail()
	if failErr := w.service.FailReconciliation(failCtx, job.Args.RunID, err); failErr != nil {
		slog.Error("Identity admin reconciliation final failure persistence failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
			"mark_failed_err", failErr,
		)
		return fmt.Errorf("identity admin reconciliation: %w: %w", err, failErr)
	}
	return nil
}

type scheduleMode string

const (
	scheduleModeDaily     scheduleMode = "daily"
	scheduleModeCatchUp   scheduleMode = "catch_up"
	scheduleModeSyncSweep scheduleMode = "sync_sweep"
)

var scheduleLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type scheduleArgs struct {
	Mode scheduleMode `json:"mode"`
}

// Kind implements river.JobArgs.
func (scheduleArgs) Kind() string { return "identity_admin_reconciliation_schedule" }

type dailySchedule struct{}

// Next implements river.Schedule: one scan per day at 03:00 Asia/Shanghai,
// mirroring the credits reconciliation cadence. Admin drift is rare but
// security-relevant; daily keeps detection latency under 24h without
// loading the Auth read endpoint.
func (dailySchedule) Next(current time.Time) time.Time {
	local := current.In(scheduleLocation)
	next := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, scheduleLocation)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// PeriodicJobs registers the daily scan plus the startup catch-up (which
// only enqueues when the last day has no completed run), plus the frequent
// Auth sync sweep (issue #888): business mutations persist sync intents
// without Auth I/O, and the sweep converges them ~every minute with
// per-user serialization and version fencing. The sweep reuses the
// schedule worker kind and queue, so no new River registration is needed.
func PeriodicJobs() []*river.PeriodicJob {
	insertOpts := func() *river.InsertOpts {
		return &river.InsertOpts{Queue: IdentityReconciliationQueue, MaxAttempts: riverpolicy.RiverMaxAttemptsIdentityAdminReconciliation, Priority: riverPriority}
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			dailySchedule{},
			func() (river.JobArgs, *river.InsertOpts) {
				return scheduleArgs{Mode: scheduleModeDaily}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "identity-admin-reconciliation-daily", RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NeverSchedule(),
			func() (river.JobArgs, *river.InsertOpts) {
				return scheduleArgs{Mode: scheduleModeCatchUp}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "identity-admin-reconciliation-catch-up", RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return scheduleArgs{Mode: scheduleModeSyncSweep}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "identity-auth-sync-sweep", RunOnStart: true},
		),
	}
}

type scheduleService interface {
	ScheduleReconciliation(context.Context, RunDispatcher, time.Time, bool) (*Run, error)
	SweepAuthSync(context.Context) (int, error)
}

// ScheduleWorker resolves each scheduler tick to the day slot and creates
// (or reuses) its run. A tick that loses the race to an active run
// snoozes: the active run's own execute job owns the scan.
func NewScheduleWorker(service scheduleService, dispatcher RunDispatcher) *ScheduleWorker {
	return &ScheduleWorker{service: service, dispatcher: dispatcher, now: time.Now}
}

// ScheduleWorker is the periodic schedule worker.
type ScheduleWorker struct {
	river.WorkerDefaults[scheduleArgs]
	service    scheduleService
	dispatcher RunDispatcher
	now        func() time.Time
}

// Timeout implements river.JobArgsWithTimeout.
func (*ScheduleWorker) Timeout(*river.Job[scheduleArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutIdentityAdminReconciliationSchedule
}

// Work implements river.Worker.
func (w *ScheduleWorker) Work(ctx context.Context, job *river.Job[scheduleArgs]) error {
	if w == nil || w.service == nil || w.dispatcher == nil || job == nil {
		return errInvalidReconciliationCommand
	}
	if job.Args.Mode == scheduleModeSyncSweep {
		if _, err := w.service.SweepAuthSync(ctx); err != nil {
			return err
		}
		return nil
	}
	onlyIfOverdue, err := parseScheduleMode(job.Args.Mode)
	if err != nil {
		return err
	}
	targetSlot := latestScheduleSlot(w.nowTime())
	run, err := w.service.ScheduleReconciliation(ctx, w.dispatcher, targetSlot, onlyIfOverdue)
	if err != nil || run == nil {
		return err
	}
	if run.Trigger == runTriggerScheduled && run.ScheduledFor != nil && run.ScheduledFor.Equal(targetSlot) {
		return nil
	}
	return river.JobSnooze(time.Minute)
}

func parseScheduleMode(mode scheduleMode) (bool, error) {
	switch mode {
	case scheduleModeDaily:
		return false, nil
	case scheduleModeCatchUp:
		return true, nil
	default:
		return false, errInvalidReconciliationCommand
	}
}

func (w *ScheduleWorker) nowTime() time.Time {
	if w != nil && w.now != nil {
		return w.now()
	}
	return time.Now()
}

func latestScheduleSlot(current time.Time) time.Time {
	local := current.In(scheduleLocation)
	slot := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, scheduleLocation)
	if local.Before(slot) {
		slot = slot.AddDate(0, 0, -1)
	}
	return slot
}
