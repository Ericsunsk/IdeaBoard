package ports_test

import (
	"ideaboard/api/internal/credits/ports"
	"testing"
)

// TestCanRunTransitionMirrorsSQLGuards pins the durable reconciliation
// run transition table to start/execute/fail_credit_reconciliation in
// internal/db/schema/fragments/credits.sql. Every true entry was
// already possible before typing.
func TestCanRunTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]ports.ReconciliationRunStatus{
		{ports.ReconciliationRunQueued, ports.ReconciliationRunRunning},
		{ports.ReconciliationRunQueued, ports.ReconciliationRunFailed},
		{ports.ReconciliationRunRunning, ports.ReconciliationRunCompleted},
		{ports.ReconciliationRunRunning, ports.ReconciliationRunFailed},
		{ports.ReconciliationRunFailed, ports.ReconciliationRunRunning},
		{ports.ReconciliationRunFailed, ports.ReconciliationRunQueued},
	}
	for _, transition := range allowed {
		if !ports.CanRunTransition(transition[0], transition[1]) {
			t.Errorf("CanRunTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	for _, to := range []ports.ReconciliationRunStatus{
		ports.ReconciliationRunQueued, ports.ReconciliationRunRunning,
		ports.ReconciliationRunCompleted, ports.ReconciliationRunFailed,
	} {
		if ports.CanRunTransition(ports.ReconciliationRunCompleted, to) {
			t.Errorf("CanRunTransition(completed, %q) = true, want false (completed is terminal)", to)
		}
	}
	if ports.CanRunTransition(ports.ReconciliationRunQueued, ports.ReconciliationRunCompleted) {
		t.Error("CanRunTransition(queued, completed) = true, want false (scans pass through running)")
	}
	if ports.CanRunTransition(ports.ReconciliationRunQueued, ports.ReconciliationRunQueued) {
		t.Error("CanRunTransition(queued, queued) = true, want false (no self-transition guard exists)")
	}
}

// TestCanFindingTransitionMirrorsSQLGuards pins the durable finding
// transition table to execute_credit_reconciliation in
// internal/db/schema/fragments/credits.sql: unseen findings resolve,
// re-observed fingerprints reopen.
func TestCanFindingTransitionMirrorsSQLGuards(t *testing.T) {
	if !ports.CanFindingTransition(ports.ReconciliationFindingOpen, ports.ReconciliationFindingResolved) {
		t.Error("CanFindingTransition(open, resolved) = false, want true")
	}
	if !ports.CanFindingTransition(ports.ReconciliationFindingResolved, ports.ReconciliationFindingOpen) {
		t.Error("CanFindingTransition(resolved, open) = false, want true")
	}
	if ports.CanFindingTransition(ports.ReconciliationFindingOpen, ports.ReconciliationFindingOpen) {
		t.Error("CanFindingTransition(open, open) = true, want false (no self-transition guard exists)")
	}
	if ports.CanFindingTransition(ports.ReconciliationFindingResolved, ports.ReconciliationFindingResolved) {
		t.Error("CanFindingTransition(resolved, resolved) = true, want false (no self-transition guard exists)")
	}
}

// TestReconciliationStatusWireWords pins the wire/DB words: typing must
// not change the strings on the wire or in the database.
func TestReconciliationStatusWireWords(t *testing.T) {
	for status, want := range map[ports.ReconciliationRunStatus]string{
		ports.ReconciliationRunQueued: "queued", ports.ReconciliationRunRunning: "running",
		ports.ReconciliationRunCompleted: "completed", ports.ReconciliationRunFailed: "failed",
	} {
		if string(status) != want {
			t.Errorf("ReconciliationRunStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[ports.ReconciliationFindingStatus]string{
		ports.ReconciliationFindingOpen: "open", ports.ReconciliationFindingResolved: "resolved",
	} {
		if string(status) != want {
			t.Errorf("ReconciliationFindingStatus = %q, want %q", string(status), want)
		}
	}
}
