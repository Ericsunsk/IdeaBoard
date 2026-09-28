package ports

import "testing"

// TestCanRunTransitionMirrorsSQLGuards pins the durable reconciliation
// run transition table to start/execute/fail_credit_reconciliation in
// internal/db/schema/fragments/credits.sql. Every true entry was
// already possible before typing.
func TestCanRunTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]ReconciliationRunStatus{
		{ReconciliationRunQueued, ReconciliationRunRunning},
		{ReconciliationRunQueued, ReconciliationRunFailed},
		{ReconciliationRunRunning, ReconciliationRunCompleted},
		{ReconciliationRunRunning, ReconciliationRunFailed},
		{ReconciliationRunFailed, ReconciliationRunRunning},
		{ReconciliationRunFailed, ReconciliationRunQueued},
	}
	for _, transition := range allowed {
		if !CanRunTransition(transition[0], transition[1]) {
			t.Errorf("CanRunTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	for _, to := range []ReconciliationRunStatus{
		ReconciliationRunQueued, ReconciliationRunRunning,
		ReconciliationRunCompleted, ReconciliationRunFailed,
	} {
		if CanRunTransition(ReconciliationRunCompleted, to) {
			t.Errorf("CanRunTransition(completed, %q) = true, want false (completed is terminal)", to)
		}
	}
	if CanRunTransition(ReconciliationRunQueued, ReconciliationRunCompleted) {
		t.Error("CanRunTransition(queued, completed) = true, want false (scans pass through running)")
	}
	if CanRunTransition(ReconciliationRunQueued, ReconciliationRunQueued) {
		t.Error("CanRunTransition(queued, queued) = true, want false (no self-transition guard exists)")
	}
}

// TestCanFindingTransitionMirrorsSQLGuards pins the durable finding
// transition table to execute_credit_reconciliation in
// internal/db/schema/fragments/credits.sql: unseen findings resolve,
// re-observed fingerprints reopen.
func TestCanFindingTransitionMirrorsSQLGuards(t *testing.T) {
	if !CanFindingTransition(ReconciliationFindingOpen, ReconciliationFindingResolved) {
		t.Error("CanFindingTransition(open, resolved) = false, want true")
	}
	if !CanFindingTransition(ReconciliationFindingResolved, ReconciliationFindingOpen) {
		t.Error("CanFindingTransition(resolved, open) = false, want true")
	}
	if CanFindingTransition(ReconciliationFindingOpen, ReconciliationFindingOpen) {
		t.Error("CanFindingTransition(open, open) = true, want false (no self-transition guard exists)")
	}
	if CanFindingTransition(ReconciliationFindingResolved, ReconciliationFindingResolved) {
		t.Error("CanFindingTransition(resolved, resolved) = true, want false (no self-transition guard exists)")
	}
}

// TestReconciliationStatusWireWords pins the wire/DB words: typing must
// not change the strings on the wire or in the database.
func TestReconciliationStatusWireWords(t *testing.T) {
	for status, want := range map[ReconciliationRunStatus]string{
		ReconciliationRunQueued: "queued", ReconciliationRunRunning: "running",
		ReconciliationRunCompleted: "completed", ReconciliationRunFailed: "failed",
	} {
		if string(status) != want {
			t.Errorf("ReconciliationRunStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[ReconciliationFindingStatus]string{
		ReconciliationFindingOpen: "open", ReconciliationFindingResolved: "resolved",
	} {
		if string(status) != want {
			t.Errorf("ReconciliationFindingStatus = %q, want %q", string(status), want)
		}
	}
}
