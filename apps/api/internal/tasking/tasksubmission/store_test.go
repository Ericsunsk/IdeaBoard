package tasksubmission

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestStoreSubmitTaskRollsBackTaskWhenRiverInsertFails(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)

	task := &tasks.Task{
		TaskID:     "task_rollback_on_river_failure",
		UserID:     42,
		Prompt:     "draw a chair",
		UserPrompt: "draw a chair",
		Params:     `{"size":"1024x1024"}`,
		Status:     tasks.StatusPending,
	}
	queueTask := &tasks.TaskPayload{
		TaskID:     task.TaskID,
		UserID:     task.UserID,
		Prompt:     task.Prompt,
		Params:     task.Params,
		AccountIDs: []int64{101},
	}
	store := NewStore(pool, failingRiverTxClient{err: errors.New("river insert failed")}, "drawing_default", 3)

	if err := store.SubmitTask(ctx, task, queueTask); err == nil {
		t.Fatalf("expected submit to fail")
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&count); err != nil {
		t.Fatalf("count task failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("task row count=%d, want 0", count)
	}
}

type failingRiverTxClient struct {
	err error
}

func (c failingRiverTxClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, c.err
}

func TestInsertTaskSQLMatchesSchemaColumns(t *testing.T) {
	ctx := context.Background()
	_, pool := testdb.OpenWithPool(t)

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

	if err := insertTask(ctx, tx, task); err != nil {
		t.Fatalf("insertTask: %v", err)
	}
	_ = tx.Rollback(ctx)
}
