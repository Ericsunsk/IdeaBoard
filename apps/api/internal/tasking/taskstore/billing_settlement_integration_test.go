package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	accountspool "ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBilledTaskFailureAndCancellationReleaseHold(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			task := &tasks.Task{TaskID: "billing-" + status, UserID: 1, Status: tasks.StatusPending, FeatureSlug: request.Feature, RequestedModel: request.Model}
			seedTasks(t, repo, t.Context(), task)
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)

			var rows int64
			var err error
			if status == "failed" {
				rows, err = repo.MarkTaskFailed(t.Context(), task.TaskID, []string{tasks.StatusPending}, "provider failed", `{}`, time.Now(), nil)
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

func TestBilledTaskCompletionCapturesWithResultAndRejectsWrongLease(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	accountID := seedRepositoryTestAccount(t, db, 771)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-complete", accountID, channelID, credentialID, "lease-correct", "lease-correct")
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	rows, err := repo.CompleteWithOutputsAndLeaseToken(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, "lease-wrong", `{}`, time.Now(), nil, nil)
	if err != nil || rows != 0 {
		t.Fatalf("wrong lease rows=%d err=%v", rows, err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "held", 75, 25)

	rows, err = repo.CompleteWithOutputsAndLeaseToken(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, "lease-correct", `{"status":"success"}`, time.Now(), nil, nil)
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
}

func TestAllAIImageWorkflowTerminalSettlementMatrix(t *testing.T) {
	for _, feature := range workflow.AIImageFeatureSlugs() {
		feature := feature
		for _, terminal := range []string{"success", "failure"} {
			terminal := terminal
			t.Run(feature+"/"+terminal, func(t *testing.T) {
				db, pool := testdb.OpenWithPool(t)
				repo, taskBilling, request := newBillingSettlementFixtureForFeature(t, db, pool, feature, 1, 100, 25)
				accountID := seedRepositoryTestAccount(t, db, 880)
				channelID := accountID + 10000
				credentialID := loadRepositoryTestCredentialID(t, db, accountID)
				taskID := "billing-matrix-" + strings.ReplaceAll(feature, "-", "_") + "-" + terminal
				task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), taskID, accountID, channelID, credentialID, "lease-"+terminal, "lease-"+terminal)
				if _, err := db.ExecContext(t.Context(), `UPDATE tasks SET feature_slug = $2, requested_model = $3 WHERE task_id = $1`, task.TaskID, feature, request.Model); err != nil {
					t.Fatal(err)
				}
				holdTaskForSettlementTest(t, pool, taskBilling, task, request)

				var rows int64
				var err error
				if terminal == "success" {
					rows, err = repo.CompleteWithOutputsAndLeaseToken(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, `{"status":"success"}`, time.Now(), nil, nil)
				} else {
					rows, err = repo.MarkTaskFailedWithLease(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "provider failed", `{}`, time.Now(), nil)
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
				rows, err = repo.MarkTaskFailedWithLease(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "provider timeout", `{}`, time.Now(), nil)
			} else {
				rows, err = repo.FinalFailTaskRunningWithLease(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retry limit", `{}`, time.Now(), nil)
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
			rows, err := repo.CompleteWithOutputsAndLeaseToken(context.Background(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, "lease-concurrent", `{}`, time.Now(), nil, nil)
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

	rows, err := repo.CompleteWithOutputsAndLeaseToken(t.Context(), task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, "lease-rollback", `{"status":"success"}`, time.Now(), nil, nil)
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

func TestTaskBillingSettlementIsIdempotentAndRejectsOppositeTerminal(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
	task := &tasks.Task{TaskID: "billing-settlement-conflict", UserID: 1, Status: tasks.StatusPending, FeatureSlug: request.Feature, RequestedModel: request.Model}
	seedTasks(t, repo, t.Context(), task)
	holdTaskForSettlementTest(t, pool, taskBilling, task, request)

	settle := func(run func(context.Context, *sql.Tx, string) error) error {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := run(t.Context(), tx, task.TaskID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := settle(taskBilling.Release); err != nil {
		t.Fatal(err)
	}
	if err := settle(taskBilling.Release); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if err := settle(taskBilling.Capture); !errors.Is(err, taskbilling.ErrSettlementConflict) {
		t.Fatalf("opposite settlement err=%v", err)
	}
	assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
}

func TestBilledTerminalTransitionFailsClosedWithoutCompleteBillingFacts(t *testing.T) {
	for _, testCase := range []struct {
		configured   bool
		withSnapshot bool
	}{
		{configured: false, withSnapshot: false},
		{configured: false, withSnapshot: true},
		{configured: true, withSnapshot: false},
	} {
		configured, withSnapshot := testCase.configured, testCase.withSnapshot
		name := fmt.Sprintf("configured=%t/snapshot=%t", configured, withSnapshot)
		t.Run(name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			creditService := credits.NewService(pool)
			billingService := billing.NewService(pool)
			taskBilling := taskbilling.NewService(billingService, creditService)
			request, err := taskbilling.AIImageRequest(workflow.FeatureSlugPatternGen, "gpt-image-1")
			if err != nil {
				t.Fatal(err)
			}
			repo := NewRepository(db)
			if configured {
				repo.SetTaskBilling(taskBilling)
			}
			task := &tasks.Task{TaskID: fmt.Sprintf("billing-integrity-%t-%t", configured, withSnapshot), UserID: 1, Status: tasks.StatusPending, FeatureSlug: request.Feature, RequestedModel: request.Model}
			seedTasks(t, repo, t.Context(), task)
			if withSnapshot {
				sku, _ := taskbilling.ResolveSKU(request)
				if _, err := billingService.CreateSKU(t.Context(), billing.CreateSKUInput{SKU: string(sku), DisplayName: "Pattern", Enabled: true, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
					t.Fatal(err)
				}
				if _, err := billingService.CreatePrice(t.Context(), billing.CreatePriceInput{SKU: string(sku), UnitCredits: 0, Enabled: true, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
					t.Fatal(err)
				}
				tx, err := pool.Begin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), 1); err != nil {
					t.Fatal(err)
				}
				if _, err := taskBilling.HoldAndPersist(t.Context(), tx, task.TaskID, task.UserID, request); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(t.Context()); err != nil {
					t.Fatal(err)
				}
			}

			rows, err := repo.MarkTaskFailed(t.Context(), task.TaskID, []string{tasks.StatusPending}, "provider failed", `{}`, time.Now(), nil)
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
		})
	}
}

func TestBulkTerminalPathsReleaseBilledHolds(t *testing.T) {
	for _, path := range []string{"account-failure", "startup-recovery"} {
		t.Run(path, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			repo, taskBilling, request := newBillingSettlementFixture(t, db, pool, 1, 100, 25)
			accountID := seedRepositoryTestAccount(t, db, 780)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, t.Context(), "billing-bulk-"+path, accountID, channelID, credentialID, "lease-bulk", "lease-bulk")
			holdTaskForSettlementTest(t, pool, taskBilling, task, request)

			if path == "account-failure" {
				ids, err := repo.FailRunningByAccountID(t.Context(), accountID, "account disabled")
				if err != nil || len(ids) != 1 || ids[0] != task.TaskID {
					t.Fatalf("ids=%v err=%v", ids, err)
				}
			} else {
				ids, err := repo.CancelRunningTasksForStartupRecovery(t.Context(), "startup recovery", time.Now())
				if err != nil || len(ids) != 1 || ids[0].TaskID != task.TaskID {
					t.Fatalf("ids=%v err=%v", ids, err)
				}
			}
			assertTaskBillingSettlement(t, db, task.TaskID, "released", 100, 0)
		})
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
			accountRepo := accountspool.NewAccountRepositoryWithRunningTaskFinalizer(db, repo)

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
	return newBillingSettlementFixtureForFeature(t, db, pool, workflow.FeatureSlugPatternGen, userID, grant, unit)
}

func newBillingSettlementFixtureForFeature(t *testing.T, db *sql.DB, pool *pgxpool.Pool, feature string, userID, grant, unit int64) (*Repository, *taskbilling.Service, taskbilling.Request) {
	t.Helper()
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	taskBilling := taskbilling.NewService(billingService, creditService)
	repo := NewRepository(db)
	repo.SetTaskBilling(taskBilling)
	request, err := taskbilling.AIImageRequest(feature, "gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}
	sku, _ := taskbilling.ResolveSKU(request)
	if _, err := billingService.CreateSKU(t.Context(), billing.CreateSKUInput{SKU: string(sku), DisplayName: feature, Enabled: true, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.CreatePrice(t.Context(), billing.CreatePriceInput{SKU: string(sku), UnitCredits: unit, Enabled: true, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
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
	amount, _ := credits.NewCreditAmount(grant)
	if _, err := creditService.InTx(tx).Grant(t.Context(), credits.GrantCommand{UserID: userID, Amount: amount, Metadata: credits.CommandMetadata{
		IdempotencyKey: fmt.Sprintf("settlement-grant:%d", userID), Actor: credits.Actor{Type: credits.ActorSystem, ID: "test"}, Source: "test", Reason: "test grant", BusinessReference: credits.BusinessReference{Type: "test", ID: fmt.Sprint(userID)},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return repo, taskBilling, request
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
