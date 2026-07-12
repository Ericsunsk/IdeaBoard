package tasksubmission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync/atomic"
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
	task.Priority = tasks.TaskPriorityInteractive
	queueTask.Priority = tasks.TaskPriorityInteractive
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)

	result, err := store.SubmitTask(ctx, task, queueTask)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if result.TaskID != task.TaskID || !result.Created {
		t.Fatalf("SubmitTask result=%+v, want created task %q", result, task.TaskID)
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
	if job.priority != tasks.TaskPriorityInteractive {
		t.Fatalf("river job priority=%d, want %d", job.priority, tasks.TaskPriorityInteractive)
	}
	if !slices.Equal(job.tags, []string{task.TaskID}) {
		t.Fatalf("river job tags=%v, want [%s]", job.tags, task.TaskID)
	}
	assertStoredTaskPriority(t, db, task.TaskID, tasks.TaskPriorityInteractive)

	var args taskqueue.DrawingTaskArgs
	if err := json.Unmarshal(job.args, &args); err != nil {
		t.Fatalf("decode river job args failed: %v", err)
	}
	if args.Task.TaskID != queueTask.TaskID {
		t.Fatalf("river args task_id=%q, want %q", args.Task.TaskID, queueTask.TaskID)
	}
	if args.Task.EffectiveCorrelationID() != task.TaskID {
		t.Fatalf("river args correlation_id=%q, want task_id %q", args.Task.EffectiveCorrelationID(), task.TaskID)
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
	if args.Task.Priority != tasks.TaskPriorityInteractive {
		t.Fatalf("river args priority=%d, want %d", args.Task.Priority, tasks.TaskPriorityInteractive)
	}
}

func TestStoreSubmitTaskReturnsExistingTaskForDuplicateClientActionID(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	client := &countingRiverTxClient{
		inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts),
	}
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)

	first := newTaskSubmissionTask("task_action_first")
	first.ClientActionID = "canvas-action-1"
	firstQueueTask := newTaskSubmissionQueueTask(first)

	firstResult, err := store.SubmitTask(ctx, first, firstQueueTask)
	if err != nil {
		t.Fatalf("first SubmitTask failed: %v", err)
	}
	if firstResult.TaskID != first.TaskID || !firstResult.Created {
		t.Fatalf("first SubmitTask result=%+v, want created task %q", firstResult, first.TaskID)
	}

	duplicate := newTaskSubmissionTask("task_action_duplicate")
	duplicate.UserID = first.UserID
	duplicate.ClientActionID = first.ClientActionID
	duplicateQueueTask := newTaskSubmissionQueueTask(duplicate)

	duplicateResult, err := store.SubmitTask(ctx, duplicate, duplicateQueueTask)
	if err != nil {
		t.Fatalf("duplicate SubmitTask failed: %v", err)
	}
	if duplicateResult.TaskID != first.TaskID || duplicateResult.Created {
		t.Fatalf("duplicate SubmitTask result=%+v, want existing task %q", duplicateResult, first.TaskID)
	}
	if duplicate.TaskID != first.TaskID {
		t.Fatalf("duplicate task_id after submit=%q, want existing %q", duplicate.TaskID, first.TaskID)
	}

	assertTaskRowCount(t, db, first.TaskID, 1)
	assertTaskRowCount(t, db, "task_action_duplicate", 0)
	assertRiverJobRowCount(t, db, first.TaskID, 1)
	assertRiverJobRowCount(t, db, "task_action_duplicate", 0)
	if got := client.InsertCount(); got != 1 {
		t.Fatalf("River InsertTx count=%d, want 1", got)
	}
}

func TestTaskSubmissionRiverClientPinsCurrentSchemaWhenFallbackContainsRiverTables(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	currentSchema := taskSubmissionCurrentSchema(t, pool)
	fallbackSchema := currentSchema + "_river_fallback"
	fallbackSQL := pgx.Identifier{fallbackSchema}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+fallbackSQL); err != nil {
		t.Fatalf("create fallback River schema failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+fallbackSQL+` CASCADE`)
	})

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{
		Logger: discardRiverTestLogger(),
		Schema: fallbackSchema,
	})
	if err != nil {
		t.Fatalf("create fallback River migrator failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("migrate fallback River schema failed: %v", err)
	}

	poolConfig := pool.Config().Copy()
	poolConfig.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{currentSchema}.Sanitize() + "," + fallbackSQL
	shadowPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create fallback-aware pool failed: %v", err)
	}
	t.Cleanup(shadowPool.Close)
	if err := shadowPool.Ping(ctx); err != nil {
		t.Fatalf("ping fallback-aware pool failed: %v", err)
	}

	client := newTaskSubmissionRiverClient(t, shadowPool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	if got := client.Schema(); got != currentSchema {
		t.Fatalf("River client schema=%q, want current schema %q", got, currentSchema)
	}
	task := newTaskSubmissionTask("task_river_schema_pinned")
	store := NewStore(shadowPool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)
	if _, err := store.SubmitTask(ctx, task, newTaskSubmissionQueueTask(task)); err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	var count int
	currentRiverJob := pgx.Identifier{currentSchema, "river_job"}.Sanitize()
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+currentRiverJob+` WHERE args->'task'->>'task_id' = $1`, task.TaskID).Scan(&count); err != nil {
		t.Fatalf("count current-schema River jobs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("current-schema River job count=%d, want 1", count)
	}
}

func TestStoreSubmitTaskCreatesDistinctTaskForNewClientActionID(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	client := &countingRiverTxClient{
		inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts),
	}
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)

	first := newTaskSubmissionTask("task_action_rerun_first")
	first.ClientActionID = "canvas-action-rerun-1"
	firstQueueTask := newTaskSubmissionQueueTask(first)

	firstResult, err := store.SubmitTask(ctx, first, firstQueueTask)
	if err != nil {
		t.Fatalf("first SubmitTask failed: %v", err)
	}
	if firstResult.TaskID != first.TaskID || !firstResult.Created {
		t.Fatalf("first SubmitTask result=%+v, want created task %q", firstResult, first.TaskID)
	}

	rerun := newTaskSubmissionTask("task_action_rerun_second")
	rerun.UserID = first.UserID
	rerun.ClientActionID = "canvas-action-rerun-2"
	rerunQueueTask := newTaskSubmissionQueueTask(rerun)

	rerunResult, err := store.SubmitTask(ctx, rerun, rerunQueueTask)
	if err != nil {
		t.Fatalf("rerun SubmitTask failed: %v", err)
	}
	if rerunResult.TaskID != rerun.TaskID || !rerunResult.Created {
		t.Fatalf("rerun SubmitTask result=%+v, want created task %q", rerunResult, rerun.TaskID)
	}
	if firstResult.TaskID == rerunResult.TaskID {
		t.Fatalf("rerun task_id=%q, want distinct from first task_id", rerunResult.TaskID)
	}

	assertTaskRowCount(t, db, first.TaskID, 1)
	assertTaskRowCount(t, db, rerun.TaskID, 1)
	assertRiverJobRowCount(t, db, first.TaskID, 1)
	assertRiverJobRowCount(t, db, rerun.TaskID, 1)
	if got := client.InsertCount(); got != 2 {
		t.Fatalf("River InsertTx count=%d, want 2", got)
	}

	firstJob := loadRiverJobForTask(t, db, first.TaskID)
	if !slices.Equal(firstJob.tags, []string{first.TaskID}) {
		t.Fatalf("first River job tags=%v, want [%s]", firstJob.tags, first.TaskID)
	}
	rerunJob := loadRiverJobForTask(t, db, rerun.TaskID)
	if !slices.Equal(rerunJob.tags, []string{rerun.TaskID}) {
		t.Fatalf("rerun River job tags=%v, want [%s]", rerunJob.tags, rerun.TaskID)
	}
}

func TestStoreSubmitTaskSerializesConcurrentDuplicateClientActionID(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	client := &countingRiverTxClient{
		inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts),
	}
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)

	start := make(chan struct{})
	results := make(chan tasks.TaskSubmissionResult, 2)
	errs := make(chan error, 2)
	for _, taskID := range []string{"task_action_concurrent_a", "task_action_concurrent_b"} {
		taskID := taskID
		go func() {
			<-start
			task := newTaskSubmissionTask(taskID)
			task.ClientActionID = "canvas-action-concurrent"
			result, err := store.SubmitTask(ctx, task, newTaskSubmissionQueueTask(task))
			results <- result
			errs <- err
		}()
	}

	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent SubmitTask failed: %v", err)
		}
	}
	first := <-results
	second := <-results
	if first.TaskID == "" || second.TaskID == "" || first.TaskID != second.TaskID {
		t.Fatalf("results=(%+v,%+v), want same existing task id", first, second)
	}
	if first.Created == second.Created {
		t.Fatalf("results=(%+v,%+v), want one created and one existing", first, second)
	}

	assertTaskRowCount(t, db, first.TaskID, 1)
	assertRiverJobRowCount(t, db, first.TaskID, 1)
	var duplicateRows int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
WHERE user_id = $1 AND client_action_id = $2`, int64(42), "canvas-action-concurrent").Scan(&duplicateRows); err != nil {
		t.Fatalf("count duplicate client_action_id tasks failed: %v", err)
	}
	if duplicateRows != 1 {
		t.Fatalf("duplicate client_action_id row count=%d, want 1", duplicateRows)
	}
	if got := client.InsertCount(); got != 1 {
		t.Fatalf("River InsertTx count=%d, want 1", got)
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
	}, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)

	_, err := store.SubmitTask(ctx, task, queueTask)
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

type countingRiverTxClient struct {
	inner       taskqueue.RiverTxClient
	insertCount atomic.Int64
}

func (c *countingRiverTxClient) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	c.insertCount.Add(1)
	return c.inner.InsertTx(ctx, tx, args, opts)
}

func (c *countingRiverTxClient) InsertCount() int64 {
	return c.insertCount.Load()
}

type noopDrawingWorker struct {
	river.WorkerDefaults[taskqueue.DrawingTaskArgs]
}

func (noopDrawingWorker) Work(context.Context, *river.Job[taskqueue.DrawingTaskArgs]) error {
	return nil
}

func newTaskSubmissionRiverClient(t testing.TB, pool *pgxpool.Pool, queueName string, maxAttempts int) *river.Client[pgx.Tx] {
	t.Helper()
	schema := taskSubmissionCurrentSchema(t, pool)
	migrateRiverForTaskSubmissionTest(t, pool, schema)

	workers := river.NewWorkers()
	river.AddWorker(workers, noopDrawingWorker{})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queueName: {MaxWorkers: 1},
		},
		Workers:     workers,
		MaxAttempts: maxAttempts,
		Schema:      schema,
		TestOnly:    true,
		Logger:      discardRiverTestLogger(),
	})
	if err != nil {
		t.Fatalf("create River test client failed: %v", err)
	}
	return client
}

func migrateRiverForTaskSubmissionTest(t testing.TB, pool *pgxpool.Pool, schema string) {
	t.Helper()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{
		Logger: discardRiverTestLogger(),
		Schema: schema,
	})
	if err != nil {
		t.Fatalf("create River migrator failed: %v", err)
	}
	if _, err := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("migrate River schema failed: %v", err)
	}
}

func taskSubmissionCurrentSchema(t testing.TB, pool *pgxpool.Pool) string {
	t.Helper()
	var schema string
	if err := pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("read current PostgreSQL schema failed: %v", err)
	}
	if schema == "" {
		t.Fatal("current PostgreSQL schema is empty")
	}
	return schema
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
	priority    int
	tags        []string
	args        []byte
}

func loadRiverJobForTask(t testing.TB, db *sql.DB, taskID string) storedRiverJob {
	t.Helper()
	var job storedRiverJob
	var tagsJSON string
	err := db.QueryRowContext(t.Context(), `
SELECT kind, queue, max_attempts, priority, args, COALESCE(array_to_json(tags), '[]'::json)::text
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(
		&job.kind,
		&job.queue,
		&job.maxAttempts,
		&job.priority,
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

func assertStoredTaskPriority(t testing.TB, db *sql.DB, taskID string, want int) {
	t.Helper()
	var priority int
	if err := db.QueryRowContext(t.Context(), `SELECT priority FROM tasks WHERE task_id = $1`, taskID).Scan(&priority); err != nil {
		t.Fatalf("load task priority for %q failed: %v", taskID, err)
	}
	if priority != want {
		t.Fatalf("task priority for %q=%d, want %d", taskID, priority, want)
	}
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
