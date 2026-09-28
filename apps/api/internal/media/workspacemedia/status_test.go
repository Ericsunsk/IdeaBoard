package workspacemedia

import "testing"

// TestCanTransitionMirrorsSQLGuards pins the durable transition table
// to the guards in internal/db/queries/workspace_media.sql:
// CreateUploadSession inserts pending; MarkUploadSessionTransferring
// fences pending -> transferring; Complete/Fail/Expire flip a live
// (pending/transferring) session to completed/failed/expired;
// terminals have no outgoing transitions. Every true entry was
// already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]SessionStatus{
		{StatusPending, StatusTransferring},
		{StatusPending, StatusCompleted},
		{StatusPending, StatusFailed},
		{StatusPending, StatusExpired},
		{StatusTransferring, StatusCompleted},
		{StatusTransferring, StatusFailed},
		{StatusTransferring, StatusExpired},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []SessionStatus{StatusCompleted, StatusFailed, StatusExpired}
	live := []SessionStatus{StatusPending, StatusTransferring, StatusCompleted, StatusFailed, StatusExpired}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(StatusTransferring, StatusTransferring) {
		t.Error("CanTransition(transferring, transferring) = true, want false (first read is a no-op when already transferring, never a flip)")
	}
	if CanTransition(StatusTransferring, StatusPending) {
		t.Error("CanTransition(transferring, pending) = true, want false (no backward transition exists)")
	}
}

// TestSessionStatusWireWords pins the wire/DB words: typing must not
// change the strings on the wire or in the database.
func TestSessionStatusWireWords(t *testing.T) {
	for status, want := range map[SessionStatus]string{
		StatusPending:      "pending",
		StatusTransferring: "transferring",
		StatusCompleted:    "completed",
		StatusFailed:       "failed",
		StatusExpired:      "expired",
	} {
		if string(status) != want {
			t.Errorf("SessionStatus = %q, want %q", string(status), want)
		}
	}
	if AssetStatusReady != "ready" {
		t.Errorf("AssetStatusReady = %q, want %q", AssetStatusReady, "ready")
	}
}

func TestSessionIsTerminal(t *testing.T) {
	for status, want := range map[SessionStatus]bool{
		StatusPending:      false,
		StatusTransferring: false,
		StatusCompleted:    true,
		StatusFailed:       true,
		StatusExpired:      true,
	} {
		session := &Session{Status: status}
		if got := session.IsTerminal(); got != want {
			t.Errorf("IsTerminal(%q) = %v, want %v", status, got, want)
		}
		if got := session.IsLive(); got == want {
			t.Errorf("IsLive(%q) = %v, want %v", status, got, !want)
		}
	}
}
