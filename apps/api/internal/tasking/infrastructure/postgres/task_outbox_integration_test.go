package postgres

import (
	"context"
	"testing"
)

func TestWorkspaceTaskTransitionsPersistDurableOutboxFacts(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)
	taskID := "task_outbox" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM task_status_event_outbox WHERE task_id=$1 AND status='pending'`, taskID).Scan(&count); err != nil {
		t.Fatalf("read create outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("create outbox rows = %d, want 1", count)
	}
	if _, err := store.ClaimTask(ctx, workspaceID, taskID, created.ExecutionToken); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM task_status_event_outbox WHERE task_id=$1 AND status='running'`, taskID).Scan(&count); err != nil {
		t.Fatalf("read running outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("running outbox rows = %d, want 1", count)
	}
	if _, err := store.FailTask(ctx, workspaceID, taskID, "provider failed"); err != nil {
		t.Fatalf("FailTask: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM task_status_event_outbox WHERE task_id=$1 AND status='failed'`, taskID).Scan(&count); err != nil {
		t.Fatalf("read failed outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("failed outbox rows = %d, want 1", count)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status_state FROM task_status_event_outbox WHERE task_id=$1 AND status='failed'`, taskID).Scan(&status); err != nil {
		t.Fatalf("read outbox state: %v", err)
	}
	if status != "pending" {
		t.Fatalf("outbox state = %q, want pending before replay", status)
	}
}
