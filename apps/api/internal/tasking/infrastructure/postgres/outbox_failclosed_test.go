package postgres

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/testdb"
)

// TestInsertTaskOutboxNilFailsClosed pins P1-5: a settlement Store without
// its durable outbox must fail closed instead of silently skipping the
// delivery fact (which would commit the task without any event and lose
// it on a crash before the staged publish).
func TestInsertTaskOutboxNilFailsClosed(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	// Bypass NewStore (which always wires the outbox) to simulate a
	// mis-wired settlement Store.
	bare := &Store{}
	task := &workspacetasks.WorkspaceTask{
		ID: "task_outbox_nil", WorkspaceID: "ws_outbox_nil",
		Status: workspacetasks.StatusCompleted,
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := bare.insertTaskOutboxTx(ctx, tx, task, workspacetasks.StatusCompleted, "task.completed", nil); err == nil {
		t.Fatal("insertTaskOutboxTx(nil outbox) succeeded, want fail-closed error")
	} else if !errors.Is(err, workspacetasks.ErrSettlementUnavailable) {
		t.Fatalf("insertTaskOutboxTx err = %v, want ErrSettlementUnavailable", err)
	}
	_ = tx.Rollback(ctx)
}
