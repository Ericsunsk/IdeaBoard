package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/tasking/tasksubmission"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	store := tasksubmission.NewStore(pool, taskRepo, client, submitToRiverWorkerQueue, submitToRiverWorkerMaxAttempts, nil)
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

func TestRiverFinalFailureSettlementRetrySurvivesRestartWithoutProviderRecall(t *testing.T) {
	ctx := t.Context()
	testRunID := time.Now().UnixNano()
	queueName := fmt.Sprintf("drawing_settlement_restart_%d", testRunID)
	taskID := fmt.Sprintf("task_final_failure_settlement_%d", testRunID)
	db, pool := testdb.OpenWithPool(t)
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("migrate River schema failed: %v", err)
	}

	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	taskBilling := taskbilling.NewService(billingService, creditService)
	request, err := taskbilling.AIImageRequest(workflow.FeatureSlugPatternGen, "gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}
	sku, err := taskbilling.ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.CreateSKU(ctx, billing.CreateSKUInput{
		SKU: string(sku), DisplayName: "Pattern Generation", Enabled: true, ActorID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.CreatePrice(ctx, billing.CreatePriceInput{
		SKU: string(sku), UnitCredits: 25, Enabled: true, ActorID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	seedRiverBillingWallet(t, pool, creditService, 42, 100)

	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	taskRepo.SetTaskBilling(taskBilling)
	account := createRiverTestAccount(t, db, accountRepo, "settlement-restart")
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	innerRuntime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	runtime := &failOnceFinalFailureRuntime{inner: innerRuntime}
	var providerCalls atomic.Int32
	newWorker := func() *RiverTaskWorker {
		worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			providerCalls.Add(1)
			return errors.New("forced terminal provider failure")
		}))
		// Keep the job scheduled long enough to stop the first client after the
		// durable settlement-only marker is committed.
		worker.finalFailureRetryDelay = 10 * time.Minute
		return worker
	}
	newClient := func(worker *RiverTaskWorker) *river.Client[pgx.Tx] {
		client, clientErr := NewRiverClient(pool, worker, RiverServerOptions{
			QueueName: queueName, Concurrency: 1,
			MaxRetry: submitToRiverWorkerMaxAttempts, JobTimeout: 5 * time.Second,
		})
		if clientErr != nil {
			t.Fatalf("create River client failed: %v", clientErr)
		}
		return client
	}

	firstWorker := newWorker()
	client := newClient(firstWorker)
	var recordCalls atomic.Int32
	firstWorker.recordOutput = func(ctx context.Context, output any) error {
		if recordCalls.Add(1) == 1 {
			return river.RecordOutput(ctx, output)
		}
		return errors.New("forced terminal marker failure")
	}
	firstWorker.enqueueFinalFailure = func(context.Context, taskqueue.DrawingFinalFailureArgs) error {
		return errors.New("forced fallback enqueue failure")
	}
	task := &tasks.Task{
		TaskID: taskID, UserID: 42,
		ClientActionID: "action_" + taskID,
		Prompt:         "draw a billed pattern", UserPrompt: "draw a billed pattern",
		Params: `{"model":"gpt-image-1"}`, FeatureSlug: request.Feature,
		RequestedModel: request.Model, Status: tasks.StatusPending,
		BillingRequest: &tasks.TaskBillingRequest{Feature: request.Feature, Model: request.Model, Quantity: request.Quantity},
	}
	queueTask := &tasks.TaskPayload{
		TaskID: task.TaskID, UserID: task.UserID, Prompt: task.Prompt,
		Params: task.Params, AccountIDs: []int64{account.ID}, CreatedAt: time.Now().Unix(),
	}
	store := tasksubmission.NewStore(pool, taskRepo, client, queueName, submitToRiverWorkerMaxAttempts, taskBilling)
	result, err := store.SubmitTask(ctx, task, queueTask)
	if err != nil {
		t.Fatal(err)
	}
	if result.HoldTransactionID == "" {
		t.Fatal("billed submission did not create Hold")
	}

	clientStopped := false
	t.Cleanup(func() {
		if !clientStopped {
			stopRiverClientForSubmitToWorkerTest(t, client)
		}
	})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitForSubmitToRiverWorkerJobState(t, db, task.TaskID, "scheduled", 10*time.Second)
	assertFinalFailureRetryMarker(t, db, task.TaskID, finalFailureExecutionOutputKind)
	if got := providerCalls.Load(); got != 1 {
		t.Fatalf("provider calls before restart=%d, want 1", got)
	}
	if got := runtime.convergeCalls.Load(); got != 1 {
		t.Fatalf("convergence calls before restart=%d, want 1", got)
	}
	assertRiverTaskBillingState(t, db, task.TaskID, tasks.StatusRunning, "held", 75, 25)

	stopRiverClientForSubmitToWorkerTest(t, client)
	clientStopped = true
	if _, err := db.ExecContext(ctx, `
UPDATE river_job
SET state = 'available', scheduled_at = clock_timestamp()
WHERE args->'task'->>'task_id' = $1 AND state = 'scheduled'`, task.TaskID); err != nil {
		t.Fatalf("make snoozed settlement retry available after restart: %v", err)
	}

	restartedClient := newClient(newWorker())
	if err := restartedClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopRiverClientForSubmitToWorkerTest(t, restartedClient) })
	waitForSubmitToRiverWorkerTaskStatus(t, taskRepo, task.TaskID, tasks.StatusFailed, 10*time.Second)
	waitForSubmitToRiverWorkerJobState(t, db, task.TaskID, "completed", 5*time.Second)
	assertRiverTaskBillingState(t, db, task.TaskID, tasks.StatusFailed, "released", 100, 0)
	if got := providerCalls.Load(); got != 1 {
		t.Fatalf("provider calls after settlement retry=%d, want 1", got)
	}
	if got := runtime.convergeCalls.Load(); got != 2 {
		t.Fatalf("convergence calls after restart=%d, want 2", got)
	}
}

func TestFinalAttemptMarkerFailureResetsLeaseBeforeProviderRetry(t *testing.T) {
	db, _ := testdb.OpenWithPool(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "marker-reset")
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	providerCalls := 0
	processor := newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		providerCalls++
		return nil
	})
	task := newSubmitToRiverWorkerTask(fmt.Sprintf("task_marker_reset_%d", time.Now().UnixNano()))
	if err := taskRepo.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	payload := newSubmitToRiverWorkerQueueTask(task, account.ID)
	payload.RiverJobID = 9915

	err := processor.ProcessBeforeExecute(t.Context(), payload, func() error {
		return errors.New("forced marker failure")
	})
	if !errors.Is(err, errFinalAttemptMarkerUnavailable) {
		t.Fatalf("error=%v, want marker unavailable", err)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls after marker failure=%d, want 0", providerCalls)
	}
	stored, err := taskRepo.GetByTaskID(t.Context(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != tasks.StatusPending || stored.AccountID != nil || stored.LeaseToken != "" {
		t.Fatalf("task after marker reset status/account/lease=%s/%v/%q", stored.Status, stored.AccountID, stored.LeaseToken)
	}
	var activeLeases int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_leases WHERE task_id = $1 AND released_at IS NULL`, task.TaskID).Scan(&activeLeases); err != nil {
		t.Fatal(err)
	}
	if activeLeases != 0 {
		t.Fatalf("active leases after marker reset=%d, want 0", activeLeases)
	}

	if err := processor.ProcessBeforeExecute(t.Context(), payload, func() error { return nil }); err != nil {
		t.Fatalf("provider retry after marker recovery failed: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls after retry=%d, want 1", providerCalls)
	}
}

type failOnceFinalFailureRuntime struct {
	inner         *taskruntime.Service
	convergeCalls atomic.Int32
}

func (r *failOnceFinalFailureRuntime) RefreshTaskAccounts(ctx context.Context, task *tasks.TaskPayload) error {
	return r.inner.RefreshTaskAccounts(ctx, task)
}

func (r *failOnceFinalFailureRuntime) FailTask(ctx context.Context, input taskruntime.FailTaskInput) error {
	return r.inner.FailTask(ctx, input)
}

func (r *failOnceFinalFailureRuntime) ResetRunningTask(ctx context.Context, input taskruntime.ResetRunningTaskInput) error {
	return r.inner.ResetRunningTask(ctx, input)
}

func (r *failOnceFinalFailureRuntime) ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error {
	if r.convergeCalls.Add(1) == 1 {
		return errors.New("forced settlement persistence failure")
	}
	return r.inner.ConvergeTaskAfterRiverFinalFailure(ctx, taskID, riverJobID, reason)
}

func seedRiverBillingWallet(t testing.TB, pool *pgxpool.Pool, service *credits.Service, userID, amount int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := service.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	creditAmount, err := credits.NewCreditAmount(amount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.InTx(tx).Grant(t.Context(), credits.GrantCommand{
		UserID: userID, Amount: creditAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "river-settlement-restart-grant",
			Actor:          credits.Actor{Type: credits.ActorSystem, ID: "test"},
			Source:         "test", Reason: "seed River settlement retry",
			BusinessReference: credits.BusinessReference{Type: "test", ID: "river-settlement-restart"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertFinalFailureRetryMarker(t testing.TB, db *sql.DB, taskID, wantKind string) {
	t.Helper()
	var metadata []byte
	if err := db.QueryRowContext(t.Context(), `
SELECT metadata
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Output finalFailureRetryOutput `json:"output"`
	}
	if err := json.Unmarshal(metadata, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Output.Kind != wantKind || stored.Output.TaskID != taskID || stored.Output.Reason != finalFailureErrorReason {
		t.Fatalf("River metadata output=%+v, want durable settlement-only marker", stored.Output)
	}
}

func assertRiverTaskBillingState(t testing.TB, db *sql.DB, taskID, wantTaskStatus, wantSettlementState string, wantAvailable, wantReserved int64) {
	t.Helper()
	var taskStatus, settlementState string
	var available, reserved int64
	if err := db.QueryRowContext(t.Context(), `
SELECT tasks.status, settlements.state, available.balance, reserved.balance
FROM tasks
JOIN task_billing_settlements settlements USING (task_id)
JOIN credit_accounts available ON available.wallet_user_id = tasks.user_id AND available.account_role = 'user_available'
JOIN credit_accounts reserved ON reserved.wallet_user_id = tasks.user_id AND reserved.account_role = 'user_reserved'
WHERE tasks.task_id = $1`, taskID).Scan(&taskStatus, &settlementState, &available, &reserved); err != nil {
		t.Fatal(err)
	}
	if taskStatus != wantTaskStatus || settlementState != wantSettlementState || available != wantAvailable || reserved != wantReserved {
		t.Fatalf("task/settlement/wallet=%s/%s/%d/%d, want %s/%s/%d/%d",
			taskStatus, settlementState, available, reserved,
			wantTaskStatus, wantSettlementState, wantAvailable, wantReserved,
		)
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
