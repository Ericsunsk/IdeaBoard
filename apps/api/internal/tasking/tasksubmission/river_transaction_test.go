package tasksubmission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

const (
	submissionRiverTestQueue       = "drawing_submission_tx"
	submissionRiverTestMaxAttempts = 4
)

func TestStoreSubmitTaskCommitsTaskAndRiverJobTransactionally(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)

	task := newTaskSubmissionTask("task_river_tx_commit")
	queueTask := newTaskSubmissionQueueTask(task)
	queueTask.AccountIDs = []int64{101, 42}
	queueTask.RetryCount = 2
	queueTask.CreatedAt = 1710000000
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts)

	if err := store.SubmitTask(ctx, task, queueTask); err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	assertTaskRowCount(t, db, task.TaskID, 1)
	job := loadRiverJobForTask(t, db, task.TaskID)
	if job.kind != taskqueue.RiverJobKindDrawingGenerate {
		t.Fatalf("river job kind=%q, want %q", job.kind, taskqueue.RiverJobKindDrawingGenerate)
	}
	if job.queue != submissionRiverTestQueue {
		t.Fatalf("river job queue=%q, want %q", job.queue, submissionRiverTestQueue)
	}
	if job.maxAttempts != submissionRiverTestMaxAttempts {
		t.Fatalf("river job max_attempts=%d, want %d", job.maxAttempts, submissionRiverTestMaxAttempts)
	}
	if !slices.Equal(job.tags, []string{task.TaskID}) {
		t.Fatalf("river job tags=%v, want [%s]", job.tags, task.TaskID)
	}

	var args taskqueue.DrawingTaskArgs
	if err := json.Unmarshal(job.args, &args); err != nil {
		t.Fatalf("decode river job args failed: %v", err)
	}
	if args.Task.TaskID != queueTask.TaskID {
		t.Fatalf("river args task_id=%q, want %q", args.Task.TaskID, queueTask.TaskID)
	}
	if args.Task.UserID != queueTask.UserID || args.Task.Prompt != queueTask.Prompt || args.Task.Params != queueTask.Params {
		t.Fatalf("river args task=%+v, want submitted task %+v", args.Task, *queueTask)
	}
	if !slices.Equal(args.Task.AccountIDs, queueTask.AccountIDs) {
		t.Fatalf("river args account_ids=%v, want %v", args.Task.AccountIDs, queueTask.AccountIDs)
	}
	if args.Task.RetryCount != queueTask.RetryCount || args.Task.CreatedAt != queueTask.CreatedAt {
		t.Fatalf("river args retry/created=(%d,%d), want (%d,%d)", args.Task.RetryCount, args.Task.CreatedAt, queueTask.RetryCount, queueTask.CreatedAt)
	}
}

func TestStoreSubmitTaskRollsBackTaskAndRiverJobWhenRealInsertReportsFailure(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)

	task := newTaskSubmissionTask("task_river_tx_rollback")
	queueTask := newTaskSubmissionQueueTask(task)
	forcedErr := errors.New("force rollback after real river insert")
	store := NewStore(pool, taskstore.NewRepository(db), insertThenFailRiverTxClient{
		inner: client,
		err:   forcedErr,
	}, submissionRiverTestQueue, submissionRiverTestMaxAttempts)

	err := store.SubmitTask(ctx, task, queueTask)
	if !errors.Is(err, forcedErr) {
		t.Fatalf("SubmitTask error=%v, want forced error %v", err, forcedErr)
	}

	assertTaskRowCount(t, db, task.TaskID, 0)
	assertRiverJobRowCount(t, db, task.TaskID, 0)
}

type insertThenFailRiverTxClient struct {
	inner taskqueue.RiverTxClient
	err   error
}

func (c insertThenFailRiverTxClient) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	result, err := c.inner.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return result, err
	}
	return result, c.err
}

type noopDrawingWorker struct {
	river.WorkerDefaults[taskqueue.DrawingTaskArgs]
}

func (noopDrawingWorker) Work(context.Context, *river.Job[taskqueue.DrawingTaskArgs]) error {
	return nil
}

func newTaskSubmissionRiverClient(t testing.TB, pool *pgxpool.Pool, queueName string, maxAttempts int) *river.Client[pgx.Tx] {
	t.Helper()
	migrateRiverForTaskSubmissionTest(t, pool)

	workers := river.NewWorkers()
	river.AddWorker(workers, noopDrawingWorker{})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queueName: {MaxWorkers: 1},
		},
		Workers:     workers,
		MaxAttempts: maxAttempts,
		TestOnly:    true,
		Logger:      discardRiverTestLogger(),
	})
	if err != nil {
		t.Fatalf("create River test client failed: %v", err)
	}
	return client
}

func migrateRiverForTaskSubmissionTest(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{
		Logger: discardRiverTestLogger(),
	})
	if err != nil {
		t.Fatalf("create River migrator failed: %v", err)
	}
	if _, err := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("migrate River schema failed: %v", err)
	}
}

func discardRiverTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTaskSubmissionTask(taskID string) *tasks.Task {
	return &tasks.Task{
		TaskID:         taskID,
		UserID:         42,
		Prompt:         "draw a transactional chair",
		UserPrompt:     "draw a transactional chair",
		Params:         `{"size":"1024x1024","style":"line"}`,
		FeatureSlug:    "sketch-to-image",
		RequestedModel: "gpt-image-1",
		Status:         tasks.StatusPending,
	}
}

func newTaskSubmissionQueueTask(task *tasks.Task) *tasks.TaskPayload {
	return &tasks.TaskPayload{
		TaskID:     task.TaskID,
		UserID:     task.UserID,
		Prompt:     task.Prompt,
		Params:     task.Params,
		AccountIDs: []int64{101},
	}
}

type storedRiverJob struct {
	kind        string
	queue       string
	maxAttempts int
	tags        []string
	args        []byte
}

func loadRiverJobForTask(t testing.TB, db *sql.DB, taskID string) storedRiverJob {
	t.Helper()
	var job storedRiverJob
	var tagsJSON string
	err := db.QueryRowContext(t.Context(), `
SELECT kind, queue, max_attempts, args, COALESCE(array_to_json(tags), '[]'::json)::text
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(
		&job.kind,
		&job.queue,
		&job.maxAttempts,
		&job.args,
		&tagsJSON,
	)
	if err != nil {
		t.Fatalf("load River job for %q failed: %v", taskID, err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &job.tags); err != nil {
		t.Fatalf("decode River job tags failed: %v", err)
	}
	return job
}

func assertTaskRowCount(t testing.TB, db *sql.DB, taskID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, taskID).Scan(&count); err != nil {
		t.Fatalf("count task rows for %q failed: %v", taskID, err)
	}
	if count != want {
		t.Fatalf("task row count for %q=%d, want %d", taskID, count, want)
	}
}

func assertRiverJobRowCount(t testing.TB, db *sql.DB, taskID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM river_job WHERE args->'task'->>'task_id' = $1`, taskID).Scan(&count); err != nil {
		t.Fatalf("count River job rows for %q failed: %v", taskID, err)
	}
	if count != want {
		t.Fatalf("River job row count for %q=%d, want %d", taskID, count, want)
	}
}
