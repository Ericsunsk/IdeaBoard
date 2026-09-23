package postgres

import (
	"context"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
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

	created, err := repo.CreateWithPGXTx(ctx, tx, task)
	if err != nil {
		t.Fatalf("CreateWithPGXTx: %v", err)
	}
	if !created {
		t.Fatal("CreateWithPGXTx reported existing task for a new task")
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
	created, err := repo.CreateWithPGXTx(ctx, tx, task)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateWithPGXTx: %v", err)
	}
	if !created {
		_ = tx.Rollback(ctx)
		t.Fatal("CreateWithPGXTx reported existing task for a new task")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&count); err != nil {
		t.Fatalf("count task failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("task row count=%d, want 0", count)
	}
}
