package reconciliation_test

import (
	"errors"
	"ideaboard/api/internal/credits/ports"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

var reconciliationTestLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func TestReconciliationScheduleCreatesOneCatchUpRunAndRetriesMissedSlot(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := newReconciliationService(t, pool, enqueuer)
	target := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationTestLocation)

	first, err := service.ScheduleReconciliation(t.Context(), target, true)
	if err != nil || first == nil {
		t.Fatalf("initial catch-up run = %+v, err = %v", first, err)
	}
	if first.Trigger != ports.ReconciliationTriggerScheduled || first.ScheduledFor == nil || !first.ScheduledFor.Equal(target) {
		t.Fatalf("initial scheduled run = %+v", first)
	}
	if len(enqueuer.calls) != 1 || enqueuer.calls[0] != first.ID {
		t.Fatalf("enqueue calls = %+v, want [%d]", enqueuer.calls, first.ID)
	}

	duplicate, err := service.ScheduleReconciliation(t.Context(), target, true)
	if err != nil || duplicate != nil {
		t.Fatalf("duplicate catch-up run = %+v, err = %v", duplicate, err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("duplicate run count = %d, want 1", got)
	}

	if err := service.ExecuteReconciliation(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ScheduleReconciliation(t.Context(), target, true); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("recent-success run count = %d, want 1", got)
	}

	if _, err := db.Exec(t.Context(), `
UPDATE credit_reconciliation_runs
SET completed_at = clock_timestamp() - INTERVAL '25 hours'
WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := service.ScheduleReconciliation(t.Context(), target.Add(24*time.Hour), true)
	if err != nil || second == nil {
		t.Fatalf("missed catch-up run = %+v, err = %v", second, err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 2 {
		t.Fatalf("missed catch-up run count = %d, want 2", got)
	}
	if len(enqueuer.calls) != 2 || enqueuer.calls[1] != second.ID {
		t.Fatalf("missed catch-up enqueue calls = %+v, want second Run %d", enqueuer.calls, second.ID)
	}
}

func TestReconciliationScheduleDoesNotReplaceRecentManualSuccess(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := newReconciliationService(t, pool, enqueuer)
	manual, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(t.Context(), manual.ID); err != nil {
		t.Fatal(err)
	}
	target := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationTestLocation)
	if _, err := service.ScheduleReconciliation(t.Context(), target, false); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 2 {
		t.Fatalf("run count = %d, want manual + scheduled", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs WHERE trigger_type = 'scheduled'`); got != 1 {
		t.Fatalf("scheduled run count = %d, want 1", got)
	}
}

func TestConcurrentReconciliationSchedulesCreateAndEnqueueOneRun(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := newReconciliationService(t, pool, enqueuer)
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationTestLocation)

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
		t.Fatalf("concurrent scheduled run count = %d, want 1", got)
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("concurrent enqueue calls = %d, want 1", len(enqueuer.calls))
	}
}

func TestScheduledReconciliationRollsBackRunWhenDispatchFails(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	dispatchErr := errors.New("dispatcher unavailable")
	failing := newReconciliationService(t, pool, &reconciliationEnqueuerStub{err: dispatchErr})
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationTestLocation)
	if _, err := failing.ScheduleReconciliation(t.Context(), slot, false); !errors.Is(err, dispatchErr) {
		t.Fatalf("ScheduleReconciliation() error = %v, want dispatcher error", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 0 {
		t.Fatalf("run count after dispatch rollback = %d, want 0", got)
	}
}

func TestReconciliationCatchUpReusesFailedSlotAndDispatchesOnce(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	firstDispatcher := &reconciliationEnqueuerStub{}
	firstService := newReconciliationService(t, pool, firstDispatcher)
	slot := time.Date(2026, 7, 17, 3, 0, 0, 0, reconciliationTestLocation)
	firstRun, err := firstService.ScheduleReconciliation(t.Context(), slot, false)
	if err != nil || firstRun == nil {
		t.Fatalf("initial scheduled run = %+v, err = %v", firstRun, err)
	}
	if err := firstService.FailReconciliation(t.Context(), firstRun.ID, errors.New("final scheduled failure")); err != nil {
		t.Fatal(err)
	}

	restartDispatcher := &reconciliationEnqueuerStub{}
	restartedService := newReconciliationService(t, pool, restartDispatcher)
	retried, err := restartedService.ScheduleReconciliation(t.Context(), slot, true)
	if err != nil || retried == nil || retried.ID != firstRun.ID || retried.Status != ports.ReconciliationRunQueued {
		t.Fatalf("retried run = %+v, err = %v", retried, err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("run count after retry = %d, want 1", got)
	}
	if len(restartDispatcher.calls) != 1 || restartDispatcher.calls[0] != firstRun.ID {
		t.Fatalf("restart dispatch calls = %+v, want [%d]", restartDispatcher.calls, firstRun.ID)
	}
}
