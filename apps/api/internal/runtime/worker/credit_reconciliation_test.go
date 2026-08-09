package worker

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestCreditReconciliationDailyScheduleUsesShanghaiThreeAM(t *testing.T) {
	location := creditReconciliationScheduleLocation
	schedule := creditReconciliationDailySchedule{}
	before := time.Date(2026, 7, 17, 2, 30, 0, 0, location)
	if got, want := schedule.Next(before), time.Date(2026, 7, 17, 3, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("Next(before) = %v, want %v", got, want)
	}
	atSchedule := time.Date(2026, 7, 17, 3, 0, 0, 0, location)
	if got, want := schedule.Next(atSchedule), time.Date(2026, 7, 18, 3, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("Next(atSchedule) = %v, want %v", got, want)
	}
}

func TestCreditReconciliationScheduleWorkerMapsRuntimePolicyToDomainIntent(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, creditReconciliationScheduleLocation)
	target := latestCreditReconciliationScheduleSlot(now)
	service := &creditReconciliationScheduleServiceStub{
		run: &credits.ReconciliationRun{
			ID:           42,
			Trigger:      credits.ReconciliationTriggerScheduled,
			ScheduledFor: &target,
		},
	}
	worker := NewCreditReconciliationScheduleWorker(service)
	worker.now = func() time.Time { return now }
	job := &river.Job[creditReconciliationScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   creditReconciliationScheduleJobArgs{Mode: creditReconciliationScheduleCatchUp},
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if !service.scheduledFor.Equal(target) || !service.onlyIfOverdue {
		t.Fatalf("ScheduleReconciliation() target = %v, onlyIfOverdue = %v", service.scheduledFor, service.onlyIfOverdue)
	}

	service.run = &credits.ReconciliationRun{ID: 41, Trigger: credits.ReconciliationTriggerManual}
	err := worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || snoozeErr.Duration != creditReconciliationContentionRetryDelay {
		t.Fatalf("Work() error = %v, want %v snooze", err, creditReconciliationContentionRetryDelay)
	}
}

type creditReconciliationScheduleServiceStub struct {
	scheduledFor  time.Time
	onlyIfOverdue bool
	run           *credits.ReconciliationRun
	err           error
}

func (s *creditReconciliationScheduleServiceStub) ScheduleReconciliation(
	_ context.Context,
	scheduledFor time.Time,
	onlyIfOverdue bool,
) (*credits.ReconciliationRun, error) {
	s.scheduledFor = scheduledFor
	s.onlyIfOverdue = onlyIfOverdue
	return s.run, s.err
}

func TestCreditReconciliationWorkerRetriesAndPersistsOnlyFinalFailure(t *testing.T) {
	executeErr := errors.New("execute reconciliation")
	service := &creditReconciliationWorkerServiceStub{executeErr: executeErr}
	worker := NewCreditReconciliationWorker(service)
	first := &river.Job[creditReconciliationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 2},
		Args:   creditReconciliationJobArgs{RunID: 42},
	}
	if err := worker.Work(t.Context(), first); !errors.Is(err, executeErr) {
		t.Fatalf("transient Work() error = %v, want execute error", err)
	}
	if service.failCalls != 0 {
		t.Fatalf("transient FailReconciliation() calls = %d, want 0", service.failCalls)
	}

	final := &river.Job[creditReconciliationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 2},
		Args:   creditReconciliationJobArgs{RunID: 42},
	}
	if err := worker.Work(t.Context(), final); err != nil {
		t.Fatalf("final Work() error = %v", err)
	}
	if service.failCalls != 1 || service.failedRunID != 42 || !errors.Is(service.failure, executeErr) {
		t.Fatalf("final failure call = count %d, run %d, error %v", service.failCalls, service.failedRunID, service.failure)
	}
}

func TestCreditReconciliationWorkerBoundsFinalFailureContextAndJoinsErrors(t *testing.T) {
	executeErr := errors.New("execute reconciliation")
	failErr := errors.New("mark reconciliation failed")
	service := &creditReconciliationWorkerServiceStub{executeErr: executeErr, failErr: failErr}
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	job := &river.Job[creditReconciliationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
		Args:   creditReconciliationJobArgs{RunID: 42},
	}

	err := NewCreditReconciliationWorker(service).Work(parent, job)
	if !errors.Is(err, executeErr) || !errors.Is(err, failErr) {
		t.Fatalf("Work() error = %v, want joined execute and fail errors", err)
	}
	if service.failContextErr != nil {
		t.Fatalf("FailReconciliation() context error = %v", service.failContextErr)
	}
	if service.failDeadline.IsZero() {
		t.Fatal("FailReconciliation() context has no deadline")
	}
	remaining := time.Until(service.failDeadline)
	if remaining <= 0 || remaining > creditReconciliationFinalFailureTimeout {
		t.Fatalf("FailReconciliation() deadline remaining = %v", remaining)
	}
}

type creditReconciliationWorkerServiceStub struct {
	executeErr     error
	failErr        error
	getErr         error
	run            credits.ReconciliationRun
	failCalls      int
	failedRunID    int64
	failure        error
	failContextErr error
	failDeadline   time.Time
}

func (s *creditReconciliationWorkerServiceStub) ExecuteReconciliation(context.Context, int64) error {
	return s.executeErr
}

func (s *creditReconciliationWorkerServiceStub) FailReconciliation(ctx context.Context, runID int64, failure error) error {
	s.failCalls++
	s.failedRunID = runID
	s.failure = failure
	s.failContextErr = ctx.Err()
	s.failDeadline, _ = ctx.Deadline()
	return s.failErr
}

func (s *creditReconciliationWorkerServiceStub) GetReconciliationRun(context.Context, int64) (credits.ReconciliationRun, error) {
	return s.run, s.getErr
}

func TestCreditReconciliationDispatcherUsesCallerTransaction(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	producer, err := NewRiverProducer(pool)
	if err != nil {
		t.Fatalf("NewRiverProducer() error = %v", err)
	}
	dispatcher, err := NewCreditReconciliationDispatcher(producer)
	if err != nil {
		t.Fatalf("NewCreditReconciliationDispatcher() error = %v", err)
	}
	before := countCreditReconciliationJobs(t, db)

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchReconciliationRun(t.Context(), tx, 41); err != nil {
		t.Fatalf("DispatchReconciliationRun() error = %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := countCreditReconciliationJobs(t, db); got != before {
		t.Fatalf("job count after rollback = %d, want %d", got, before)
	}

	tx, err = pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchReconciliationRun(t.Context(), tx, 42); err != nil {
		t.Fatalf("DispatchReconciliationRun() error = %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := countCreditReconciliationJobs(t, db); got != before+1 {
		t.Fatalf("job count after commit = %d, want %d", got, before+1)
	}
}

func countCreditReconciliationJobs(t testing.TB, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM river_job WHERE kind = 'credit_reconciliation'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
