package taskstore

import (
	"context"
	"testing"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
)

func TestCreateWithPGXTxMatchesTaskInsertSchema(t *testing.T) {
	ctx := context.Background()
	_, pool := testdb.OpenWithPool(t)
	repo := NewRepository(nil)

	task := &tasks.Task{
		TaskID:     "task_schema_drift_test",
		UserID:     42,
		Prompt:     "draw a chair",
		UserPrompt: "draw a chair",
		Params:     `{"size":"1024x1024"}`,
		Status:     tasks.StatusPending,
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := repo.CreateWithPGXTx(ctx, tx, task); err != nil {
		t.Fatalf("CreateWithPGXTx: %v", err)
	}
	if task.ID == 0 {
		t.Fatal("CreateWithPGXTx left task.ID unset")
	}
}

func TestCreateWithPGXTxRollsBackWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	repo := NewRepository(nil)

	task := &tasks.Task{
		TaskID: "task_pgx_tx_rollback",
		UserID: 42,
		Status: tasks.StatusPending,
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := repo.CreateWithPGXTx(ctx, tx, task); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateWithPGXTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&count); err != nil {
		t.Fatalf("count task failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("task row count=%d, want 0", count)
	}
}
