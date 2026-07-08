package worker

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/tasking/tasksubmission"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const (
	submitToRiverWorkerQueue       = "drawing_submit_worker_convergence"
	submitToRiverWorkerMaxAttempts = 1
)

func TestSubmitTaskConvergesThroughStartedRiverWorker(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("migrate River schema failed: %v", err)
	}

	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "submit-worker-convergence")
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	taskRuntime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())

	seenRiverJobID := make(chan int64, 1)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, taskRuntime, func(_ context.Context, runtimeAccount *accounts.ApiAccount, task *tasks.TaskPayload, _ taskqueue.TaskAssignment) (providerCallOutput, error) {
		if runtimeAccount.ID != account.ID {
			return providerCallOutput{}, fmt.Errorf("provider got account_id=%d, want %d", runtimeAccount.ID, account.ID)
		}
		select {
		case seenRiverJobID <- task.RiverJobID:
		default:
		}
		return providerOutputJSON(`{"status":"success","image_url":"https://example.com/river-worker-result.png"}`), nil
	})
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, taskRuntime, orchestrator.execute))
	client, err := NewRiverClient(pool, worker, RiverServerOptions{
		QueueName:   submitToRiverWorkerQueue,
		Concurrency: 1,
		MaxRetry:    submitToRiverWorkerMaxAttempts,
		JobTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create River client failed: %v", err)
	}

	task := newSubmitToRiverWorkerTask("task_submit_worker_converges")
	queueTask := newSubmitToRiverWorkerQueueTask(task, account.ID)
	if task.Status != tasks.StatusPending {
		t.Fatalf("new task status=%q, want %q", task.Status, tasks.StatusPending)
	}
	store := tasksubmission.NewStore(pool, taskRepo, client, submitToRiverWorkerQueue, submitToRiverWorkerMaxAttempts)
	result, err := store.SubmitTask(ctx, task, queueTask)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if result.TaskID != task.TaskID || !result.Created {
		t.Fatalf("SubmitTask result=%+v, want created task %q", result, task.TaskID)
	}

	pending := loadSubmitToRiverWorkerTask(t, taskRepo, task.TaskID)
	if pending.Status != tasks.StatusPending {
		t.Fatalf("submitted task status=%q, want %q before River Start", pending.Status, tasks.StatusPending)
	}
	insertedJob := loadSubmitToRiverWorkerJob(t, db, task.TaskID)
	if insertedJob.id <= 0 {
		t.Fatalf("inserted river_job.id=%d, want positive id", insertedJob.id)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("start River client failed: %v", err)
	}
	t.Cleanup(func() {
		stopRiverClientForSubmitToWorkerTest(t, client)
	})

	completed := waitForSubmitToRiverWorkerTaskStatus(t, taskRepo, task.TaskID, tasks.StatusCompleted, 10*time.Second)
	if completed.Result == "" {
		t.Fatal("completed task result is empty")
	}

	job := waitForSubmitToRiverWorkerJobState(t, db, task.TaskID, "completed", 5*time.Second)
	if job.id != insertedJob.id {
		t.Fatalf("river_job.id changed from %d to %d", insertedJob.id, job.id)
	}

	providerRiverJobID := waitForProviderRiverJobID(t, seenRiverJobID, time.Second)
	if providerRiverJobID != job.id {
		t.Fatalf("provider saw RiverJobID=%d, want river_job.id=%d", providerRiverJobID, job.id)
	}

	attempt := loadSubmitToRiverWorkerAttempt(t, db, task.TaskID)
	if attempt.status != tasks.TaskAttemptStatusCompleted {
		t.Fatalf("task attempt status=%q, want %q", attempt.status, tasks.TaskAttemptStatusCompleted)
	}
	if attempt.riverJobID != job.id {
		t.Fatalf("task attempt river_job_id=%d, want %d", attempt.riverJobID, job.id)
	}
	if !taskqueue.LeaseBelongsToRiverJob(attempt.leaseToken, job.id) {
		t.Fatalf("lease_token=%q does not belong to river_job.id=%d", attempt.leaseToken, job.id)
	}
}

func newSubmitToRiverWorkerTask(taskID string) *tasks.Task {
	return &tasks.Task{
		TaskID:         taskID,
		UserID:         42,
		Prompt:         "draw a River convergence proof",
		UserPrompt:     "draw a River convergence proof",
		Params:         `{"feature":"sketch-to-image","model":"gpt-image-1","size":"1024x1024"}`,
		FeatureSlug:    "sketch-to-image",
		RequestedModel: "gpt-image-1",
		APIType:        "openai-compatible",
		Status:         tasks.StatusPending,
	}
}

func newSubmitToRiverWorkerQueueTask(task *tasks.Task, accountID int64) *tasks.TaskPayload {
	return &tasks.TaskPayload{
		TaskID:     task.TaskID,
		UserID:     task.UserID,
		Prompt:     task.Prompt,
		Params:     task.Params,
		AccountIDs: []int64{accountID},
		CreatedAt:  time.Now().Unix(),
	}
}

func stopRiverClientForSubmitToWorkerTest(t testing.TB, client *river.Client[pgx.Tx]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Stop(ctx); err != nil {
		t.Errorf("stop River client failed: %v", err)
	}
}

func loadSubmitToRiverWorkerTask(t testing.TB, repo *taskstore.Repository, taskID string) *tasks.Task {
	t.Helper()
	task, err := repo.GetByTaskID(t.Context(), taskID)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	return task
}

func waitForSubmitToRiverWorkerTaskStatus(t testing.TB, repo *taskstore.Repository, taskID string, want string, timeout time.Duration) *tasks.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *tasks.Task
	var lastErr error
	for time.Now().Before(deadline) {
		task, err := repo.GetByTaskID(t.Context(), taskID)
		if err != nil {
			lastErr = err
		} else {
			last = task
			if task.Status == want {
				return task
			}
			if tasks.IsTerminalStatus(task.Status) {
				t.Fatalf("task %q reached terminal status=%q, want %q; error_msg=%q", taskID, task.Status, want, task.ErrorMsg)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("wait for task %q status=%q timed out after %s; last error=%v", taskID, want, timeout, lastErr)
	}
	if last == nil {
		t.Fatalf("wait for task %q status=%q timed out after %s; task was never loaded", taskID, want, timeout)
		return nil
	}
	t.Fatalf("wait for task %q status=%q timed out after %s; last status=%q error_msg=%q", taskID, want, timeout, last.Status, last.ErrorMsg)
	return nil
}

func waitForProviderRiverJobID(t testing.TB, seen <-chan int64, timeout time.Duration) int64 {
	t.Helper()
	select {
	case riverJobID := <-seen:
		return riverJobID
	case <-time.After(timeout):
		t.Fatalf("provider did not observe RiverJobID within %s", timeout)
		return 0
	}
}

type submitToRiverWorkerJob struct {
	id    int64
	state string
}

func loadSubmitToRiverWorkerJob(t testing.TB, db *sql.DB, taskID string) submitToRiverWorkerJob {
	t.Helper()
	job, err := tryLoadSubmitToRiverWorkerJob(t, db, taskID)
	if err != nil {
		t.Fatalf("load River job for %q failed: %v", taskID, err)
	}
	return job
}

func waitForSubmitToRiverWorkerJobState(t testing.TB, db *sql.DB, taskID string, want string, timeout time.Duration) submitToRiverWorkerJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last submitToRiverWorkerJob
	var lastErr error
	for time.Now().Before(deadline) {
		job, err := tryLoadSubmitToRiverWorkerJob(t, db, taskID)
		if err != nil {
			lastErr = err
		} else {
			last = job
			if job.state == want {
				return job
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("wait for River job %q state=%q timed out after %s; last error=%v", taskID, want, timeout, lastErr)
	}
	t.Fatalf("wait for River job %q state=%q timed out after %s; last state=%q", taskID, want, timeout, last.state)
	return submitToRiverWorkerJob{}
}

func tryLoadSubmitToRiverWorkerJob(t testing.TB, db *sql.DB, taskID string) (submitToRiverWorkerJob, error) {
	t.Helper()
	var job submitToRiverWorkerJob
	err := db.QueryRowContext(t.Context(), `
SELECT id, state::text
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(&job.id, &job.state)
	return job, err
}

type submitToRiverWorkerAttempt struct {
	riverJobID int64
	status     string
	leaseToken string
}

func loadSubmitToRiverWorkerAttempt(t testing.TB, db *sql.DB, taskID string) submitToRiverWorkerAttempt {
	t.Helper()
	var attempt submitToRiverWorkerAttempt
	err := db.QueryRowContext(t.Context(), `
SELECT river_job_id, status, lease_token
FROM task_attempts
WHERE task_id = $1
ORDER BY attempt_no DESC
LIMIT 1`, taskID).Scan(&attempt.riverJobID, &attempt.status, &attempt.leaseToken)
	if err != nil {
		t.Fatalf("load task attempt for %q failed: %v", taskID, err)
	}
	return attempt
}
