package workspacetasks_test

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/events"
)

// TestTerminalOutboxDurablyWritten pins P1-5 (second half): every terminal
// settlement commits its durable outbox row in the same transaction, so
// the staged drain + best-effort Publish path (warn-only on failure) never
// loses the event — the River outbox replayer retries the pending row.
func TestTerminalOutboxDurablyWritten(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	completed, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_ = env.service.StagedEvents()

	wantEventID := events.StableEventID(completed.ID, string(completed.Status), completed.ExecutionToken)
	var gotEventID, gotStatus string
	if err := env.pool.QueryRow(context.Background(),
		`SELECT event_id, status FROM task_status_event_outbox WHERE task_id=$1 ORDER BY internal_id DESC LIMIT 1`,
		completed.ID).Scan(&gotEventID, &gotStatus); err != nil {
		t.Fatalf("read terminal outbox row: %v", err)
	}
	if gotEventID != wantEventID {
		t.Fatalf("outbox event_id = %q, want stable %q", gotEventID, wantEventID)
	}
	if gotStatus != strings.TrimSpace(string(completed.Status)) {
		t.Fatalf("outbox status = %q, want %q", gotStatus, completed.Status)
	}
}
