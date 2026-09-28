package workspacetasks

import "testing"

// TestCanTransitionMirrorsSQLGuards pins the durable transition table to
// the guards in internal/db/queries/workspace_tasks.sql: Claim fences
// pending -> running; Complete/Fail/Cancel flip a live (pending/running)
// task; Retry returns a live task to pending; terminals have no outgoing
// transitions. Every true entry was already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]Status{
		{StatusPending, StatusRunning},
		{StatusPending, StatusPending},
		{StatusPending, StatusCompleted},
		{StatusPending, StatusFailed},
		{StatusPending, StatusCancelled},
		{StatusRunning, StatusPending},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusCancelled},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []Status{StatusCompleted, StatusFailed, StatusCancelled}
	live := []Status{StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(StatusRunning, StatusRunning) {
		t.Error("CanTransition(running, running) = true, want false (retry returns to pending, never running -> running)")
	}
}

func TestIsTerminalStatus(t *testing.T) {
	for status, want := range map[Status]bool{
		StatusPending:   false,
		StatusRunning:   false,
		StatusCompleted: true,
		StatusFailed:    true,
		StatusCancelled: true,
	} {
		if got := IsTerminalStatus(status); got != want {
			t.Errorf("IsTerminalStatus(%q) = %v, want %v", status, got, want)
		}
	}
}
