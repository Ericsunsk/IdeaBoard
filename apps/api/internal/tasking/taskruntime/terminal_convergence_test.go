package taskruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompletedTaskConvergenceCommitsTerminalFactsBeforePublishing(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "completion-convergence-success")
	commitObserver := &committedCompletionPublisher{
		db:       db,
		taskID:   task.TaskID,
		delegate: publisher,
	}
	service.SetTaskStatusPublisher(commitObserver)
	completedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	output := tasks.TaskOutput{
		OutputKind: "remote",
		Role:       "image",
		URL:        "https://provider.example/completed.png",
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success"}`,
		Outputs:    []tasks.TaskOutput{output},
		AttemptFinish: &tasks.TaskAttemptFinish{
			ID:          attemptID,
			CompletedAt: completedAt,
		},
	})
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusCompleted || loaded.Result != `{"status":"success"}` || loaded.CompletedAt == nil {
		t.Fatalf("task terminal fact = status %q result %q completed_at %v", loaded.Status, loaded.Result, loaded.CompletedAt)
	}
	outputsByTaskID, err := repo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("load outputs: %v", err)
	}
	if outputs := outputsByTaskID[task.TaskID]; len(outputs) != 1 || outputs[0].URL != output.URL {
		t.Fatalf("outputs = %+v, want one completed output", outputs)
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusCompleted)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 0)
	assertCompletionSettlement(t, db, task.TaskID, "captured", 75, 0)
	if commitObserver.observationErr != nil {
		t.Fatalf("publisher observed uncommitted completion: %v", commitObserver.observationErr)
	}
	if !commitObserver.observedCommitted {
		t.Fatal("publisher did not observe committed completion facts")
	}

	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusCompleted {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusCompleted)
		}
	default:
		t.Fatal("completed status was not published after terminal commit")
	}
}

func TestCompletedTaskConvergenceRejectsMissingActiveLeaseWithoutPartialWrites(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "completion-convergence-missing-lease")
	result, err := db.ExecContext(ctx, `
UPDATE task_leases
SET released_at = NOW(), release_reason = $2
WHERE task_id = $1 AND released_at IS NULL
`, task.TaskID, tasks.TaskAttemptStatusCompleted)
	if err != nil {
		t.Fatalf("remove active lease: %v", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		t.Fatalf("remove active lease rows=%d err=%v, want rows=1", rows, err)
	}

	err = service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success"}`,
		Outputs: []tasks.TaskOutput{{
			OutputKind: "remote",
			Role:       "image",
			URL:        "https://provider.example/missing-lease.png",
		}},
		AttemptFinish: &tasks.TaskAttemptFinish{ID: attemptID, CompletedAt: time.Now()},
	})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("error=%v, want ErrTaskStateConflict", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusRunning || loaded.Result != "" || loaded.LeaseToken != task.LeaseToken || loaded.CompletedAt != nil {
		t.Fatalf("task changed after missing active lease: %+v", loaded)
	}
	outputsByTaskID, err := repo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("load outputs: %v", err)
	}
	if len(outputsByTaskID[task.TaskID]) != 0 {
		t.Fatalf("missing active lease wrote outputs: %+v", outputsByTaskID[task.TaskID])
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRunning)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 0)
	assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
	select {
	case event := <-publisher.ch:
		t.Fatalf("missing active lease published event: %+v", event)
	default:
	}
}

func TestCompletedTaskConvergenceRejectsStaleLeaseWithoutPartialWrites(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "completion-convergence-stale")

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignmentWithLease(task, "stale-lease-token"),
		Result:     `{"status":"success"}`,
		Outputs: []tasks.TaskOutput{{
			OutputKind: "remote",
			Role:       "image",
			URL:        "https://provider.example/stale.png",
		}},
		AttemptFinish: &tasks.TaskAttemptFinish{ID: attemptID, CompletedAt: time.Now()},
	})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("error=%v, want ErrTaskStateConflict", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusRunning || loaded.Result != "" || loaded.LeaseToken != task.LeaseToken || loaded.CompletedAt != nil {
		t.Fatalf("task changed after stale completion: %+v", loaded)
	}
	outputsByTaskID, err := repo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("load outputs: %v", err)
	}
	if len(outputsByTaskID[task.TaskID]) != 0 {
		t.Fatalf("stale completion wrote outputs: %+v", outputsByTaskID[task.TaskID])
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRunning)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 1)
	assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
	select {
	case event := <-publisher.ch:
		t.Fatalf("stale completion published event: %+v", event)
	default:
	}
}

func TestCompletedTaskConvergenceRollsBackLateLeaseMismatch(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "completion-convergence-late-lease-mismatch")
	if _, err := db.ExecContext(ctx, `
UPDATE task_leases
SET lease_token = 'replacement-lease-token'
WHERE task_id = $1 AND released_at IS NULL
`, task.TaskID); err != nil {
		t.Fatalf("replace active lease token: %v", err)
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success"}`,
		Outputs: []tasks.TaskOutput{{
			OutputKind: "remote",
			Role:       "image",
			URL:        "https://provider.example/late-mismatch.png",
		}},
		AttemptFinish: &tasks.TaskAttemptFinish{ID: attemptID, CompletedAt: time.Now()},
	})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("error=%v, want ErrTaskStateConflict", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusRunning || loaded.Result != "" || loaded.LeaseToken != task.LeaseToken || loaded.CompletedAt != nil {
		t.Fatalf("task changed after late lease mismatch: %+v", loaded)
	}
	outputsByTaskID, err := repo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("load outputs: %v", err)
	}
	if len(outputsByTaskID[task.TaskID]) != 0 {
		t.Fatalf("late lease mismatch wrote outputs: %+v", outputsByTaskID[task.TaskID])
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRunning)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 1)
	assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
	select {
	case event := <-publisher.ch:
		t.Fatalf("late lease mismatch published event: %+v", event)
	default:
	}
}

func TestFailedTaskConvergenceReleasesHoldBeforePublishing(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "failure-convergence-success")
	commitObserver := &committedFailurePublisher{db: db, taskID: task.TaskID, delegate: publisher}
	service.SetTaskStatusPublisher(commitObserver)

	err := service.FailTask(ctx, FailTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Reason:     "provider failed",
		Result:     `{"status":"failed"}`,
		AttemptFinish: &tasks.TaskAttemptFinish{
			ID:          attemptID,
			CompletedAt: time.Date(2026, time.July, 31, 12, 5, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FailTask returned error: %v", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusFailed || loaded.Result != `{"status":"failed"}` || loaded.CompletedAt == nil {
		t.Fatalf("task terminal fact = status %q result %q completed_at %v", loaded.Status, loaded.Result, loaded.CompletedAt)
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusFailed)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 0)
	assertCompletionSettlement(t, db, task.TaskID, "released", 100, 0)
	if commitObserver.observationErr != nil || !commitObserver.observedCommitted {
		t.Fatalf("publisher observed uncommitted failure: %v", commitObserver.observationErr)
	}
	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusFailed {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusFailed)
		}
	default:
		t.Fatal("failed status was not published after terminal commit")
	}
}

func TestRequeueAndResetConvergenceKeepHoldAndReleaseExactLease(t *testing.T) {
	testCases := []struct {
		name string
		run  func(context.Context, *Service, *tasks.Task, int64) error
	}{
		{
			name: "requeue",
			run: func(ctx context.Context, service *Service, task *tasks.Task, attemptID int64) error {
				_, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{
					Assignment: taskRuntimeAssignment(task),
					Reason:     "provider retryable failure",
					RequeuedAttemptFinish: &tasks.TaskAttemptFinish{
						ID:          attemptID,
						CompletedAt: time.Date(2026, time.July, 31, 12, 10, 0, 0, time.UTC),
					},
				})
				return err
			},
		},
		{
			name: "reset",
			run: func(ctx context.Context, service *Service, task *tasks.Task, attemptID int64) error {
				return service.ResetRunningTask(ctx, ResetRunningTaskInput{
					Assignment: taskRuntimeAssignment(task),
					Reason:     "worker reset",
					AttemptFinish: &tasks.TaskAttemptFinish{
						ID:          attemptID,
						CompletedAt: time.Date(2026, time.July, 31, 12, 10, 0, 0, time.UTC),
					},
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "nonterminal-convergence-"+tc.name)

			if err := tc.run(t.Context(), service, task, attemptID); err != nil {
				t.Fatalf("convergence returned error: %v", err)
			}
			loaded, err := repo.GetByTaskID(t.Context(), task.TaskID)
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if loaded.Status != tasks.StatusPending || loaded.CompletedAt != nil || loaded.LeaseToken != "" || loaded.AccountID != nil {
				t.Fatalf("nonterminal task fact = %+v, want pending without lease", loaded)
			}
			assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRequeued)
			assertCompletionActiveLeaseCount(t, db, task.TaskID, 0)
			assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
			select {
			case event := <-publisher.ch:
				if event.TaskID != task.TaskID || event.Status != tasks.StatusPending {
					t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusPending)
				}
			default:
				t.Fatal("pending status was not published after nonterminal convergence")
			}
		})
	}
}

func TestFinalFailureConvergenceRollsBackLateLeaseMismatch(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "failure-convergence-late-lease")
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET retry_count = $2 WHERE task_id = $1`, task.TaskID, defaultTaskRetryLimit-1); err != nil {
		t.Fatalf("set retry count: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_leases SET lease_token = 'replacement-lease-token' WHERE task_id = $1 AND released_at IS NULL`, task.TaskID); err != nil {
		t.Fatalf("replace active lease token: %v", err)
	}

	_, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{
		Assignment: taskRuntimeAssignment(task),
		Reason:     "retry limit reached",
		FinalFailAttemptFinish: &tasks.TaskAttemptFinish{
			ID:          attemptID,
			CompletedAt: time.Now(),
		},
	})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("error=%v, want ErrTaskStateConflict", err)
	}
	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if loaded.Status != tasks.StatusRunning || loaded.CompletedAt != nil || loaded.LeaseToken != task.LeaseToken {
		t.Fatalf("task changed after late lease mismatch: %+v", loaded)
	}
	assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRunning)
	assertCompletionActiveLeaseCount(t, db, task.TaskID, 1)
	assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
	select {
	case event := <-publisher.ch:
		t.Fatalf("late lease mismatch published event: %+v", event)
	default:
	}
}

func TestFailureConvergenceRollsBackAttemptAndSettlementFailures(t *testing.T) {
	testCases := []struct {
		name    string
		prepare func(*testing.T, *sql.DB, *tasks.Task, int64)
	}{
		{
			name: "attempt conflict",
			prepare: func(t *testing.T, db *sql.DB, task *tasks.Task, attemptID int64) {
				t.Helper()
				if _, err := db.ExecContext(t.Context(), `
UPDATE task_leases
SET lease_token = 'replacement-attempt-lease'
WHERE id = (SELECT lease_id FROM task_attempts WHERE id = $1)`, attemptID); err != nil {
					t.Fatalf("replace attempt lease token: %v", err)
				}
			},
		},
		{
			name: "settlement failure",
			prepare: func(t *testing.T, db *sql.DB, task *tasks.Task, attemptID int64) {
				t.Helper()
				if _, err := db.ExecContext(t.Context(), `
CREATE FUNCTION fail_runtime_failure_settlement() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'forced settlement failure'; END $$;
CREATE TRIGGER force_runtime_failure_settlement
BEFORE UPDATE ON task_billing_settlements FOR EACH ROW EXECUTE FUNCTION fail_runtime_failure_settlement()`); err != nil {
					t.Fatalf("install settlement failure trigger: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			service, repo, task, attemptID, publisher := newBilledCompletionConvergenceFixture(t, db, pool, "failure-convergence-rollback-"+tc.name)
			tc.prepare(t, db, task, attemptID)

			err := service.FailTask(t.Context(), FailTaskInput{
				Assignment: taskRuntimeAssignment(task),
				Reason:     "provider failed",
				AttemptFinish: &tasks.TaskAttemptFinish{
					ID:          attemptID,
					CompletedAt: time.Now(),
				},
			})
			if !errors.Is(err, ErrTaskStateConflict) && tc.name == "attempt conflict" {
				t.Fatalf("error=%v, want ErrTaskStateConflict", err)
			}
			if err == nil {
				t.Fatal("convergence unexpectedly succeeded")
			}
			loaded, loadErr := repo.GetByTaskID(t.Context(), task.TaskID)
			if loadErr != nil {
				t.Fatalf("reload task: %v", loadErr)
			}
			if loaded.Status != tasks.StatusRunning || loaded.CompletedAt != nil || loaded.LeaseToken != task.LeaseToken {
				t.Fatalf("task changed after rollback: %+v", loaded)
			}
			assertCompletionAttemptStatus(t, db, attemptID, tasks.TaskAttemptStatusRunning)
			assertCompletionActiveLeaseCount(t, db, task.TaskID, 1)
			assertCompletionSettlement(t, db, task.TaskID, "held", 75, 25)
			select {
			case event := <-publisher.ch:
				t.Fatalf("rollback published event: %+v", event)
			default:
			}
		})
	}
}

type committedFailurePublisher struct {
	db                *sql.DB
	taskID            string
	delegate          *recordingTaskStatusPublisher
	observedCommitted bool
	observationErr    error
}

func (p *committedFailurePublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	var taskStatus string
	var settlementState string
	p.observationErr = p.db.QueryRowContext(ctx, `
SELECT tasks.status, task_billing_settlements.state
FROM tasks
JOIN task_billing_settlements USING (task_id)
WHERE tasks.task_id = $1
`, p.taskID).Scan(&taskStatus, &settlementState)
	if p.observationErr != nil {
		return p.observationErr
	}
	if taskStatus != tasks.StatusFailed || settlementState != "released" {
		p.observationErr = fmt.Errorf("failure facts status=%q settlement=%q, want failed/released", taskStatus, settlementState)
		return p.observationErr
	}
	p.observedCommitted = true
	return p.delegate.PublishTaskStatus(ctx, event)
}

type committedCompletionPublisher struct {
	db                *sql.DB
	taskID            string
	delegate          *recordingTaskStatusPublisher
	observedCommitted bool
	observationErr    error
}

func (p *committedCompletionPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	var taskStatus string
	var settlementState string
	p.observationErr = p.db.QueryRowContext(ctx, `
SELECT tasks.status, task_billing_settlements.state
FROM tasks
JOIN task_billing_settlements USING (task_id)
WHERE tasks.task_id = $1
`, p.taskID).Scan(&taskStatus, &settlementState)
	if p.observationErr != nil {
		return p.observationErr
	}
	if taskStatus != tasks.StatusCompleted || settlementState != "captured" {
		p.observationErr = fmt.Errorf("completion facts status=%q settlement=%q, want completed/captured", taskStatus, settlementState)
		return p.observationErr
	}
	p.observedCommitted = true
	return p.delegate.PublishTaskStatus(ctx, event)
}

func newBilledCompletionConvergenceFixture(
	t *testing.T,
	db *sql.DB,
	pool *pgxpool.Pool,
	taskID string,
) (*Service, *taskstore.Repository, *tasks.Task, int64, *recordingTaskStatusPublisher) {
	t.Helper()
	ctx := t.Context()
	const userID int64 = 1
	const accountID int64 = 731

	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	taskBilling := taskbilling.NewService(billingService, creditService)
	request, err := taskbilling.AIImageRequest(workflow.FeatureSlugPatternGen)
	if err != nil {
		t.Fatal(err)
	}
	sku, err := taskbilling.ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.PublishPrice(ctx, billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU:         sku,
			Namespace:   "ai-image",
			ProductKind: "workflow-feature",
			DisplayName: workflow.FeatureSlugPatternGen,
		},
		UnitCredits: 25,
		ActorID:     1,
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(ctx, userID); err != nil {
		t.Fatal(err)
	}
	grantAmount, err := credits.NewCreditAmount(100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creditService.InTx(tx).Grant(ctx, credits.GrantCommand{
		UserID: userID,
		Amount: grantAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "completion-convergence-grant:" + taskID,
			Actor:          credits.Actor{Type: credits.ActorSystem, ID: "taskruntime-test"},
			Source:         "taskruntime_test",
			Reason:         "seed completion convergence",
			BusinessReference: credits.BusinessReference{
				Type: "taskruntime_test",
				ID:   taskID,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	seedTaskRuntimeUser(t, db, userID)
	seedTaskRuntimeAccount(t, db, accountID)
	repo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID:                     taskID,
		UserID:                     userID,
		FeatureSlug:                workflow.FeatureSlugPatternGen,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "current-lease-token",
	}
	if err := seedTaskRuntimeTask(t, db, repo, ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	holdTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holdTx.Rollback(ctx)
	if _, err := taskBilling.HoldAndPersist(ctx, holdTx, task.TaskID, task.UserID, request); err != nil {
		t.Fatal(err)
	}
	if err := holdTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	attempt, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		RiverJobID:         1,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: tasks.IDValue(task.AssignedProviderEndpointID),
		CredentialID:       tasks.IDValue(task.AssignedCredentialID),
		ProviderStartedAt:  time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, repo, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	return service, repo, task, attempt.ID, publisher
}

func assertCompletionAttemptStatus(t *testing.T, db *sql.DB, attemptID int64, want string) {
	t.Helper()
	var status string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM task_attempts WHERE id = $1`, attemptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("attempt status=%q, want %q", status, want)
	}
}

func assertCompletionActiveLeaseCount(t *testing.T, db *sql.DB, taskID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_leases WHERE task_id = $1 AND released_at IS NULL`, taskID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("active lease count=%d, want %d", count, want)
	}
}

func assertCompletionSettlement(t *testing.T, db *sql.DB, taskID, wantState string, wantAvailable, wantReserved int64) {
	t.Helper()
	var state string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM task_billing_settlements WHERE task_id = $1`, taskID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != wantState {
		t.Fatalf("settlement state=%q, want %q", state, wantState)
	}
	var available, reserved int64
	if err := db.QueryRowContext(t.Context(), `SELECT balance FROM credit_accounts WHERE wallet_user_id = 1 AND account_role = 'user_available'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT balance FROM credit_accounts WHERE wallet_user_id = 1 AND account_role = 'user_reserved'`).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if available != wantAvailable || reserved != wantReserved {
		t.Fatalf("available/reserved=%d/%d, want %d/%d", available, reserved, wantAvailable, wantReserved)
	}
}
