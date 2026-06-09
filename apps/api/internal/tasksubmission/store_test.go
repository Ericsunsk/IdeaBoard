package tasksubmission

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestStoreSubmitTaskRollsBackTaskWhenRiverInsertFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	pool := openSubmitStoreTestPool(t)

	task := &tasks.Task{
		TaskID:     "task_rollback_on_river_failure",
		UserID:     42,
		Prompt:     "draw a chair",
		UserPrompt: "draw a chair",
		Params:     `{"size":"1024x1024"}`,
		Status:     tasks.StatusPending,
	}
	queueTask := &queuetypes.TaskPayload{
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

func openSubmitStoreTestPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if raw == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse PostgreSQL test dsn failed: %v", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("open PostgreSQL test pool failed: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping PostgreSQL test pool failed: %v", err)
	}
	return pool
}

type failingRiverTxClient struct {
	err error
}

func (c failingRiverTxClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, c.err
}
