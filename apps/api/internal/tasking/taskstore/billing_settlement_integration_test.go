package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	accountspool "ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBilledTaskFailureAndCancellationReleaseHold(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			task := &tasks.Task{TaskID: "billing-" + status, UserID: 1, Status: tasks.StatusPending, FeatureSlug: request.Feature}
			seedTasks(t, repo, t.Context(), task)
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)

			var rows int64
			var err error
			if status == "failed" {
				rows, err = repo.convergePendingFailureForTest(t.Context(), task.TaskID, "provider failed", `{}`, time.Now(), nil)
			} else {
				rows, err = repo.MarkTaskCancelled(t.Context(), tasks.TaskCancellationSnapshot{TaskID: task.TaskID, CancelledAt: time.Now(), Reason: "cancelled"}, nil)
			}
			if err != nil || rows != 1 {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
			assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
		})
	}
}

func TestBilledTaskCancellationWithoutExplicitAttemptFinishConvergesAllTerminalFacts(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 770)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-cancel-active-attempt", accountID, channelID, credentialID, "lease-cancel", "lease-cancel")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	attempt, err := repo.CreateTaskAttempt(t.Context(), tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		RiverJobID:         1,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelledAt := time.Now()
	rows, err := repo.MarkTaskCancelled(t.Context(), tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      "cancelled by user",
	}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}

	loaded, err := repo.GetByTaskID(t.Context(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != tasks.StatusCancelled {
		t.Fatalf("task status=%q, want cancelled", loaded.Status)
	}
	if loaded.ExecutionMetrics == nil || loaded.ExecutionMetrics.CurrentStage != tasks.ExecutionStageCancelled {
		t.Fatalf("execution metrics=%+v, want cancelled", loaded.ExecutionMetrics)
	}
	assertRepositoryActiveLeaseCount(t, db, task.TaskID, 0)
	assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)

	var attemptStatus string
	var attemptCancelledAt sql.NullTime
	if err := db.QueryRowContext(t.Context(), `
SELECT status, cancelled_at
FROM task_attempts
WHERE id = $1`, attempt.ID).Scan(&attemptStatus, &attemptCancelledAt); err != nil {
		t.Fatal(err)
	}
	if attemptStatus != tasks.TaskAttemptStatusCancelled || !attemptCancelledAt.Valid {
		t.Fatalf("attempt status/cancelled_at=%q/%v, want cancelled timestamp", attemptStatus, attemptCancelledAt)
	}
	var statStatus string
	if err := db.QueryRowContext(t.Context(), `
SELECT status
FROM task_stat_events
WHERE task_id = $1`, task.TaskID).Scan(&statStatus); err != nil {
		t.Fatal(err)
	}
	if statStatus != tasks.StatusCancelled {
		t.Fatalf("daily stat event status=%q, want cancelled", statStatus)
	}
}

func TestBilledTaskCompletionCapturesWithResultAndRejectsWrongLease(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 771)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-complete", accountID, channelID, credentialID, "lease-correct", "lease-correct")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	rows, err := repo.ConvergeTask(t.Context(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, "lease-wrong", `{}`, time.Now(), nil, nil))
	if err != nil || rows != 0 {
		t.Fatalf("wrong lease rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "held", 75, 25)

	rows, err = repo.ConvergeTask(t.Context(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, "lease-correct", `{"status":"success"}`, time.Now(), oneRemoteTaskOutput("https://example.com/result.png"), nil))
	if err != nil || rows != 1 {
		t.Fatalf("complete rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "captured", 75, 0)
	var status, result string
	if err := db.QueryRowContext(t.Context(), `SELECT status, result FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&status, &result); err != nil {
		t.Fatal(err)
	}
	if status != tasks.StatusCompleted || result != `{"status":"success"}` {
		t.Fatalf("status=%q result=%q", status, result)
	}
	history, err := repo.ListAdminBillingSnapshots(t.Context(), []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListAdminBillingSnapshots failed: %v", err)
	}
	if history[task.TaskID].SettlementState != "captured" || history[task.TaskID].SettledAt == nil {
		t.Fatalf("Admin Billing settlement projection=%+v", history[task.TaskID])
	}
}

func TestBilledTaskPartialCompletionCapturesOutputsAndReleasesRemainder(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	request, err := taskbilling.AIImageRequest(request.Feature, "1k", 4)
	if err != nil {
		t.Fatal(err)
	}
	accountID := seedRepositoryTestAccount(t, db, 1771)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-partial", accountID, channelID, credentialID, "lease-partial", "lease-partial")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	outputs := []tasks.TaskOutput{
		{OutputKind: "remote", Role: "image", URL: "https://example.com/0.png", ResultIndex: 0},
		{OutputKind: "remote", Role: "image", URL: "https://example.com/2.png", ResultIndex: 2},
	}
	rows, err := repo.ConvergeTask(t.Context(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, task.LeaseToken, `{"status":"success"}`, time.Now(), outputs, nil))
	if err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "captured", 50, 0)
	var actual, captured, released int64
	if err := db.QueryRowContext(t.Context(), `SELECT actual_output_count, captured_credits, released_credits FROM task_billing_settlements WHERE task_id = $1`, task.TaskID).Scan(&actual, &captured, &released); err != nil {
		t.Fatal(err)
	}
	if actual != 2 || captured != 50 || released != 50 {
		t.Fatalf("settlement actual/captured/released=%d/%d/%d", actual, captured, released)
	}
	rowsOut, err := repo.ListOutputsByTaskIDs(t.Context(), []string{task.TaskID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsOut[task.TaskID]) != 2 || rowsOut[task.TaskID][0].ResultIndex != 0 || rowsOut[task.TaskID][1].ResultIndex != 2 {
		t.Fatalf("outputs=%+v, want stable indexes [0,2]", rowsOut[task.TaskID])
	}
	terminalTransactions := countTaskSettlementTransactions(t, db, task.TaskID)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := taskbilling.SettleTerminal(t.Context(), tx, task.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := countTaskSettlementTransactions(t, db, task.TaskID); got != terminalTransactions {
		t.Fatalf("terminal transaction count=%d, want idempotent %d", got, terminalTransactions)
	}
}

func TestBilledTaskCancellationWithResultsCapturesOutputsAndReleasesRemainder(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	request, err := taskbilling.AIImageRequest(request.Feature, "1k", 4)
	if err != nil {
		t.Fatal(err)
	}
	accountID := seedRepositoryTestAccount(t, db, 1772)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-cancelled-with-results", accountID, channelID, credentialID, "lease-cancel-results", "lease-cancel-results")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO task_outputs (task_id, output_kind, role, url, result_index)
VALUES ($1, 'remote', 'image', 'https://example.com/0.png', 0),
       ($1, 'remote', 'image', 'https://example.com/2.png', 2)`, task.TaskID); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.MarkTaskCancelled(t.Context(), tasks.TaskCancellationSnapshot{
		TaskID: task.TaskID, CancelledAt: time.Now(), Reason: "cancelled after partial results",
	}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
	loaded, err := repo.GetByTaskID(t.Context(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != tasks.StatusCancelledWithResults {
		t.Fatalf("task status=%q, want %q", loaded.Status, tasks.StatusCancelledWithResults)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "captured", 50, 0)
	var actual, captured, released int64
	if err := db.QueryRowContext(t.Context(), `SELECT actual_output_count, captured_credits, released_credits FROM task_billing_settlements WHERE task_id = $1`, task.TaskID).Scan(&actual, &captured, &released); err != nil {
		t.Fatal(err)
	}
	if actual != 2 || captured != 50 || released != 50 {
		t.Fatalf("settlement actual/captured/released=%d/%d/%d", actual, captured, released)
	}
	var statStatus string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM task_stat_events WHERE task_id = $1`, task.TaskID).Scan(&statStatus); err != nil {
		t.Fatal(err)
	}
	if statStatus != tasks.StatusCancelled {
		t.Fatalf("daily stat event status=%q, want %q", statStatus, tasks.StatusCancelled)
	}
}

func TestAdminBillingSnapshotProjectsFreeSettlementWithoutLedgerMetadata(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 0, 0)
	task := &tasks.Task{
		TaskID:      "billing-free-admin-history",
		UserID:      1,
		Status:      tasks.StatusPending,
		FeatureSlug: request.Feature,
	}
	seedTasks(t, repo, t.Context(), task)
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	history, err := repo.ListAdminBillingSnapshots(t.Context(), []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListAdminBillingSnapshots failed: %v", err)
	}
	snapshot := history[task.TaskID]
	if snapshot.SettlementState != "free" || snapshot.SettledAt == nil || snapshot.TotalCredits != 0 {
		t.Fatalf("free Admin Billing settlement projection=%+v", snapshot)
	}
}

func TestPaidTaskTerminalSettlementMatrix(t *testing.T) {
	for _, terminal := range []string{"success", "failure"} {
		t.Run(terminal, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			accountID := seedRepositoryTestAccount(t, db, 880)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-matrix-"+terminal, accountID, channelID, credentialID, "lease-"+terminal, "lease-"+terminal)
			if _, err := db.ExecContext(t.Context(), `UPDATE tasks SET feature_slug = $2 WHERE task_id = $1`, task.TaskID, request.Feature); err != nil {
				t.Fatal(err)
			}
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)

			var rows int64
			var err error
			if terminal == "success" {
				rows, err = repo.ConvergeTask(t.Context(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, task.LeaseToken, `{"status":"success"}`, time.Now(), oneRemoteTaskOutput("https://example.com/result.png"), nil))
			} else {
				rows, err = repo.convergeFailedWithLeaseForTest(t.Context(), task.TaskID, accountID, channelID, credentialID, task.LeaseToken, "provider failed", `{}`, time.Now(), nil)
			}
			if err != nil || rows != 1 {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
			wantState, wantAvailable := "captured", int64(75)
			if terminal == "failure" {
				wantState, wantAvailable = "released", 100
			}
			assertTaskBillingSettlement(t, db, task.TaskID, wantState, wantAvailable, 0)
		})
	}
}

func TestBilledTimeoutAndFinalFailureReleaseHold(t *testing.T) {
	for _, path := range []string{"timeout", "final-failure"} {
		t.Run(path, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			accountID := seedRepositoryTestAccount(t, db, 775)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-"+path, accountID, channelID, credentialID, "lease-"+path, "lease-"+path)
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)

			var (
				rows int64
				err  error
			)
			if path == "timeout" {
				rows, err = repo.convergeFailedWithLeaseForTest(t.Context(), task.TaskID, accountID, channelID, credentialID, task.LeaseToken, "provider timeout", `{}`, time.Now(), nil)
			} else {
				rows, err = repo.convergeFinalFailureForTest(t.Context(), task.TaskID, accountID, channelID, credentialID, task.LeaseToken, "retry limit", `{}`, time.Now(), nil)
			}
			if err != nil || rows != 1 {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
			assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
		})
	}
}

func TestConcurrentBilledCompletionCapturesExactlyOnce(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 772)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-concurrent", accountID, channelID, credentialID, "lease-concurrent", "lease-concurrent")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	start := make(chan struct{})
	rowsCh := make(chan int64, 2)
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rows, err := repo.ConvergeTask(context.Background(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, "lease-concurrent", `{}`, time.Now(), oneRemoteTaskOutput("https://example.com/result.png"), nil))
			rowsCh <- rows
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(rowsCh)
	close(errCh)
	totalRows := int64(0)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for rows := range rowsCh {
		totalRows += rows
	}
	if totalRows != 1 {
		t.Fatalf("total rows=%d", totalRows)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "captured", 75, 0)
	var captures int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE business_reference_id = $1 AND command_type = 'capture'`, task.TaskID).Scan(&captures); err != nil {
		t.Fatal(err)
	}
	if captures != 1 {
		t.Fatalf("capture count=%d", captures)
	}
}

func TestCapturePersistenceFailureRollsBackTaskResultAndLedger(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 773)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-rollback", accountID, channelID, credentialID, "lease-rollback", "lease-rollback")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)
	if _, err := db.ExecContext(t.Context(), `
CREATE FUNCTION fail_task_billing_settlement_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'forced settlement failure'; END $$;
CREATE TRIGGER force_task_billing_settlement_failure
BEFORE UPDATE ON task_billing_settlements FOR EACH ROW EXECUTE FUNCTION fail_task_billing_settlement_update()`); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ConvergeTask(t.Context(), completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, "lease-rollback", `{"status":"success"}`, time.Now(), oneRemoteTaskOutput("https://example.com/result.png"), nil))
	if err == nil || rows != 0 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "held", 75, 25)
	var status, result string
	if err := db.QueryRowContext(t.Context(), `SELECT status, result FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&status, &result); err != nil {
		t.Fatal(err)
	}
	if status != tasks.StatusRunning || result != "" {
		t.Fatalf("status=%q result=%q", status, result)
	}
}

func TestTerminalTransitionRollsBackWhenBillingSettlementIsMissing(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 0, 0)
	task := &tasks.Task{TaskID: "billing-integrity-missing-settlement", UserID: 1, Status: tasks.StatusPending, FeatureSlug: request.Feature}
	seedTasks(t, repo, t.Context(), task)
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER task_billing_settlements_state_machine ON task_billing_settlements`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM task_billing_settlements WHERE task_id = $1`, task.TaskID); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.convergePendingFailureForTest(t.Context(), task.TaskID, "provider failed", `{}`, time.Now(), nil)
	if !errors.Is(err, taskbilling.ErrIntegrity) || rows != 0 {
		t.Fatalf("rows=%d err=%v, want billing integrity rollback", rows, err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != tasks.StatusPending {
		t.Fatalf("status=%q, want pending after rollback", status)
	}
}

func TestFinalFailureConvergenceReleasesBilledHolds(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 780)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-startup-recovery", accountID, channelID, credentialID, "lease-bulk", "lease-bulk")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	rows, err := repo.ConvergeTask(t.Context(), TaskConvergenceIntent{
		Kind:   TaskConvergenceFinalFailed,
		TaskID: task.TaskID,
		Lease: &TaskLeaseIdentity{
			AccountID: accountID, ProviderEndpointID: channelID, CredentialID: credentialID, LeaseToken: task.LeaseToken,
		},
		ErrorMessage: "startup recovery",
		OccurredAt:   time.Now(),
	})
	if err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
	history, err := repo.ListAdminBillingSnapshots(t.Context(), []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListAdminBillingSnapshots failed: %v", err)
	}
	if history[task.TaskID].SettlementState != "released" || history[task.TaskID].SettledAt == nil {
		t.Fatalf("released Admin Billing settlement projection=%+v", history[task.TaskID])
	}
}

func TestAccountStopAndArchiveReleaseBilledHoldsAtomically(t *testing.T) {
	for _, operation := range []string{"stop", "archive"} {
		t.Run(operation, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			accountID := seedRepositoryTestAccount(t, db, 785)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-account-"+operation, accountID, channelID, credentialID, "lease-account", "lease-account")
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)
			accountRepo := accountspool.NewAccountRepository(db, repo)

			var (
				ids []string
				err error
			)
			if operation == "stop" {
				ids, err = accountRepo.StopAccountAndFinalizeRunningTasks(t.Context(), accountID, "account stopped")
			} else {
				ids, err = accountRepo.ArchiveAccountAndFinalizeRunningTasks(t.Context(), accountID, "account archived")
			}
			if err != nil || len(ids) != 1 || ids[0] != task.TaskID {
				t.Fatalf("ids=%v err=%v", ids, err)
			}
			assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
			var status string
			if err := db.QueryRowContext(t.Context(), `SELECT status FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != tasks.StatusFailed {
				t.Fatalf("task status=%q, want failed", status)
			}
		})
	}
}

func newBillingSettlementFixture(t *testing.T, db *sql.DB, pool *pgxpool.Pool, userID, grant, unit int64) (*Repository, *taskbilling.Service, taskbilling.Request) {
	return newBillingSettlementFixtureForFeature(t, db, pool, workflow.FeatureSlugGeneration, userID, grant, unit)
}

func newBillingSettlementFixtureForFeature(t *testing.T, db *sql.DB, pool *pgxpool.Pool, feature string, userID, grant, unit int64) (*Repository, *taskbilling.Service, taskbilling.Request) {
	t.Helper()
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	taskBilling := taskbilling.NewService(billingService, creditService)
	repo := NewRepository(db)
	request, err := taskbilling.AIImageRequest(feature, "1k", 1)
	if err != nil {
		t.Fatal(err)
	}
	sku, _ := taskbilling.ResolveSKU(request)
	if _, err := billingService.PublishPrice(t.Context(), taskstorePriceInput(sku, feature, unit)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	if grant > 0 {
		amount, _ := credits.NewCreditAmount(grant)
		if _, err := creditService.InTx(tx).Grant(t.Context(), credits.GrantCommand{UserID: userID, Amount: amount, Metadata: credits.CommandMetadata{
			IdempotencyKey: fmt.Sprintf("settlement-grant:%d", userID), Actor: credits.Actor{Type: credits.ActorSystem, ID: "test"}, Source: "test", Reason: "test grant", BusinessReference: credits.BusinessReference{Type: "test", ID: fmt.Sprint(userID)},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return repo, taskBilling, request
}

func taskstorePriceInput(sku billing.SKU, displayName string, unitCredits int64) billing.PublishPriceInput {
	return billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: sku, Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: displayName,
		},
		UnitCredits: unitCredits,
		ActorID:     1,
	}
}

func oneRemoteTaskOutput(url string) []tasks.TaskOutput {
	return []tasks.TaskOutput{{OutputKind: "remote", Role: "image", URL: url, ResultIndex: 0}}
}

func holdTaskForSettlementTest(t *testing.T, pool *pgxpool.Pool, service *taskbilling.Service, task *tasks.Task, request taskbilling.Request) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := service.HoldAndPersist(t.Context(), tx, task.TaskID, task.UserID, request); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertTaskBillingSettlement(t *testing.T, db *sql.DB, taskID, wantState string, wantAvailable, wantReserved int64) {
	t.Helper()
	var state string
	if err := db.QueryRow(`SELECT state FROM task_billing_settlements WHERE task_id = $1`, taskID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != wantState {
		t.Fatalf("state=%q want=%q", state, wantState)
	}
	var available, reserved int64
	if err := db.QueryRow(`SELECT balance FROM credit_accounts WHERE wallet_user_id = 1 AND account_role = 'user_available'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT balance FROM credit_accounts WHERE wallet_user_id = 1 AND account_role = 'user_reserved'`).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if available != wantAvailable || reserved != wantReserved {
		t.Fatalf("available/reserved=%d/%d want=%d/%d", available, reserved, wantAvailable, wantReserved)
	}
}

func countTaskSettlementTransactions(t *testing.T, db *sql.DB, taskID string) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(t.Context(), `
SELECT COUNT(*)
FROM credit_transactions
WHERE business_reference_type = 'task'
  AND business_reference_id = $1
  AND command_type IN ('capture', 'release')`, taskID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
