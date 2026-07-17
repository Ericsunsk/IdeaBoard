package credits

import (
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestReconciliationDailyScheduleUsesShanghaiThreeAM(t *testing.T) {
	location := reconciliationScheduleLocation
	schedule := reconciliationDailySchedule{}
	before := time.Date(2026, 7, 17, 2, 30, 0, 0, location)
	if got, want := schedule.Next(before), time.Date(2026, 7, 17, 3, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("Next(before) = %v, want %v", got, want)
	}
	atSchedule := time.Date(2026, 7, 17, 3, 0, 0, 0, location)
	if got, want := schedule.Next(atSchedule), time.Date(2026, 7, 18, 3, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("Next(atSchedule) = %v, want %v", got, want)
	}
}

func TestReconciliationScheduleWorkerHandlesStartupDailyAndMissedCatchUp(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := NewService(pool)
	service.SetReconciliationEnqueuer(enqueuer)
	worker := NewReconciliationScheduleWorker(service)
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, reconciliationScheduleLocation)
	worker.now = func() time.Time { return now }

	catchUp := &river.Job[ReconciliationScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   ReconciliationScheduleJobArgs{Mode: ReconciliationScheduleCatchUp},
	}
	if err := worker.Work(t.Context(), catchUp); err != nil {
		t.Fatal(err)
	}
	var firstID int64
	var trigger string
	var requestedBy *int64
	var reason *string
	var scheduledFor time.Time
	var staleSeconds float64
	if err := db.QueryRowContext(t.Context(), `
SELECT id, trigger_type, requested_by, reason, scheduled_for, EXTRACT(EPOCH FROM stale_hold_after)
FROM credit_reconciliation_runs
`).Scan(&firstID, &trigger, &requestedBy, &reason, &scheduledFor, &staleSeconds); err != nil {
		t.Fatal(err)
	}
	if trigger != string(ReconciliationTriggerScheduled) || requestedBy != nil || reason != nil ||
		!scheduledFor.Equal(latestReconciliationScheduleSlot(now)) || int64(staleSeconds) != 86400 {
		t.Fatalf("first scheduled Run trigger=%q requestedBy=%v reason=%v scheduledFor=%v stale=%v",
			trigger, requestedBy, reason, scheduledFor, staleSeconds)
	}
	if len(enqueuer.calls) != 1 || enqueuer.calls[0].RunID != firstID {
		t.Fatalf("enqueue calls = %+v, want first Run only", enqueuer.calls)
	}

	if err := worker.Work(t.Context(), catchUp); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("duplicate startup catch-up Run count = %d, want 1", got)
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("duplicate startup enqueue calls = %d, want 1", len(enqueuer.calls))
	}

	if err := service.ExecuteReconciliation(t.Context(), firstID); err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(t.Context(), catchUp); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("recent-success catch-up Run count = %d, want 1", got)
	}

	if _, err := db.ExecContext(t.Context(), `
UPDATE credit_reconciliation_runs
SET completed_at = clock_timestamp() - INTERVAL '25 hours'
WHERE id = $1
`, firstID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if err := worker.Work(t.Context(), catchUp); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 2 {
		t.Fatalf("missed catch-up Run count = %d, want 2", got)
	}
	if len(enqueuer.calls) != 2 {
		t.Fatalf("missed catch-up enqueue calls = %d, want 2", len(enqueuer.calls))
	}
}

func TestReconciliationDailyIgnoresRecentManualSuccess(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := NewService(pool)
	service.SetReconciliationEnqueuer(enqueuer)
	manual, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(t.Context(), manual.ID); err != nil {
		t.Fatal(err)
	}

	worker := NewReconciliationScheduleWorker(service)
	now := time.Date(2026, 7, 17, 3, 5, 0, 0, reconciliationScheduleLocation)
	worker.now = func() time.Time { return now }
	job := &river.Job[ReconciliationScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   ReconciliationScheduleJobArgs{Mode: ReconciliationScheduleDaily},
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 2 {
		t.Fatalf("daily Run count = %d, want manual + scheduled", got)
	}
	var scheduled int64
	if err := db.QueryRowContext(t.Context(), `
SELECT COUNT(*) FROM credit_reconciliation_runs WHERE trigger_type = 'scheduled'
`).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled != 1 {
		t.Fatalf("scheduled Run count = %d, want 1", scheduled)
	}
}

func TestConcurrentReconciliationSchedulesCreateAndEnqueueOneRun(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := NewService(pool)
	service.SetReconciliationEnqueuer(enqueuer)
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationScheduleLocation)

	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.ScheduleReconciliation(t.Context(), slot, false)
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("concurrent scheduled Run count = %d, want 1", got)
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("concurrent enqueue calls = %d, want 1", len(enqueuer.calls))
	}
}

func TestScheduledReconciliationRollsBackRunWhenEnqueueFails(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	queueErr := errors.New("River unavailable")
	failing := NewService(pool)
	failing.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{err: queueErr})
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationScheduleLocation)
	if _, err := failing.ScheduleReconciliation(t.Context(), slot, false); !errors.Is(err, queueErr) {
		t.Fatalf("ScheduleReconciliation() error = %v, want queue error", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 0 {
		t.Fatalf("Run count after enqueue rollback = %d, want 0", got)
	}

	enqueuer := &reconciliationEnqueuerStub{}
	healthy := NewService(pool)
	healthy.SetReconciliationEnqueuer(enqueuer)
	run, err := healthy.ScheduleReconciliation(t.Context(), slot, false)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.Trigger != ReconciliationTriggerScheduled || run.RequestedBy != nil ||
		run.Reason != "" || run.ScheduledFor == nil || !run.ScheduledFor.Equal(slot) ||
		len(enqueuer.calls) != 1 {
		t.Fatalf("recovered schedule run=%+v enqueue calls=%d", run, len(enqueuer.calls))
	}
}

func TestReconciliationCatchUpReusesFailedSlotAndEnqueuesOnceAfterRestart(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	firstEnqueuer := &reconciliationEnqueuerStub{}
	firstService := NewService(pool)
	firstService.SetReconciliationEnqueuer(firstEnqueuer)
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationScheduleLocation)

	firstRun, err := firstService.ScheduleReconciliation(t.Context(), slot, false)
	if err != nil {
		t.Fatal(err)
	}
	if firstRun == nil {
		t.Fatal("initial scheduled Run is nil")
	}
	if err := firstService.FailReconciliation(t.Context(), firstRun.ID, errors.New("final scheduled failure")); err != nil {
		t.Fatal(err)
	}
	failingRestart := NewService(pool)
	restartQueueErr := errors.New("restart River unavailable")
	failingRestart.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{err: restartQueueErr})
	if _, err := failingRestart.ScheduleReconciliation(t.Context(), slot, true); !errors.Is(err, restartQueueErr) {
		t.Fatalf("failed restart error = %v, want queue error", err)
	}
	stillFailed, err := firstService.GetReconciliationRun(t.Context(), firstRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillFailed.Status != ReconciliationRunFailed || stillFailed.ErrorMessage == "" ||
		stillFailed.StartedAt == nil || stillFailed.CompletedAt == nil {
		t.Fatalf("Run after rolled-back retry = %+v, want original failed state", stillFailed)
	}

	restartEnqueuer := &reconciliationEnqueuerStub{}
	restartedService := NewService(pool)
	restartedService.SetReconciliationEnqueuer(restartEnqueuer)
	retriedRun, err := restartedService.ScheduleReconciliation(t.Context(), slot, true)
	if err != nil {
		t.Fatal(err)
	}
	if retriedRun == nil || retriedRun.ID != firstRun.ID || retriedRun.Status != ReconciliationRunQueued ||
		retriedRun.ErrorMessage != "" || retriedRun.StartedAt != nil || retriedRun.CompletedAt != nil {
		t.Fatalf("retried Run = %+v, want reset original Run %d", retriedRun, firstRun.ID)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("Run count after catch-up retry = %d, want 1", got)
	}
	if len(restartEnqueuer.calls) != 1 || restartEnqueuer.calls[0].RunID != firstRun.ID {
		t.Fatalf("restart enqueue calls = %+v, want original Run once", restartEnqueuer.calls)
	}

	duplicate, err := restartedService.ScheduleReconciliation(t.Context(), slot, true)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate == nil || duplicate.ID != firstRun.ID || len(restartEnqueuer.calls) != 1 {
		t.Fatalf("duplicate catch-up Run=%+v enqueue calls=%d", duplicate, len(restartEnqueuer.calls))
	}
}

func TestReconciliationDailySnoozesOnManualContentionThenCreatesTargetSlot(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := NewService(pool)
	service.SetReconciliationEnqueuer(enqueuer)
	manual, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}

	worker := NewReconciliationScheduleWorker(service)
	now := time.Date(2026, 7, 17, 3, 5, 0, 0, reconciliationScheduleLocation)
	worker.now = func() time.Time { return now }
	job := &river.Job[ReconciliationScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   ReconciliationScheduleJobArgs{Mode: ReconciliationScheduleDaily},
	}
	err = worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || snoozeErr.Duration != reconciliationScheduleContentionRetryDelay {
		t.Fatalf("Work() error = %v, want %v snooze", err, reconciliationScheduleContentionRetryDelay)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 || len(enqueuer.calls) != 1 {
		t.Fatalf("contention Run count=%d enqueue calls=%d, want manual only", got, len(enqueuer.calls))
	}

	if err := service.ExecuteReconciliation(t.Context(), manual.ID); err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("retry Work() error = %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 2 || len(enqueuer.calls) != 2 {
		t.Fatalf("retry Run count=%d enqueue calls=%d, want manual + target slot", got, len(enqueuer.calls))
	}
	var scheduledID int64
	if err := db.QueryRowContext(t.Context(), `
SELECT id
FROM credit_reconciliation_runs
WHERE trigger_type = 'scheduled' AND scheduled_for = $1
`, latestReconciliationScheduleSlot(now)).Scan(&scheduledID); err != nil {
		t.Fatal(err)
	}
	if enqueuer.calls[1].RunID != scheduledID {
		t.Fatalf("scheduled enqueue Run ID = %d, want %d", enqueuer.calls[1].RunID, scheduledID)
	}
}
