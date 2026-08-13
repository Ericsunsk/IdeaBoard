package tasksubmission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type noOpRiverTxClient struct{}

func (noOpRiverTxClient) InsertTx(
	context.Context,
	pgx.Tx,
	river.JobArgs,
	*river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return nil, nil
}

func TestBilledStyleVariationSubmissionSucceedsAsRuntimeRole(t *testing.T) {
	db, ownerPool := testdb.OpenWithPool(t)
	ownerCredits := credits.NewService(ownerPool)
	ownerBilling := billing.NewService(ownerPool)
	request, err := taskbilling.AIImageRequest(workflow.FeatureSlugStyleVariation, 1)
	if err != nil {
		t.Fatal(err)
	}
	sku := mustResolveSKU(t, request)
	if _, err := ownerBilling.PublishPrice(t.Context(), submissionPriceInput(sku, "Style Variation", 20)); err != nil {
		t.Fatal(err)
	}
	seedCreditWallet(t, ownerPool, ownerCredits, 42, 200)

	policies := []dbprivileges.Policy{
		database.RuntimePrivilegePolicy(),
		credits.RuntimePrivilegePolicy(),
		billing.RuntimePrivilegePolicy(),
		taskbilling.RuntimePrivilegePolicy(),
	}
	runtimeRole := testdb.CreateRestrictedRole(t, db, "task_submission_runtime")
	if err := dbprivileges.Configure(t.Context(), db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure task submission runtime role: %v", err)
	}
	runtimePool := testdb.OpenPoolAsRole(t, ownerPool, runtimeRole)
	var canReadClaims, canUpdateClaims bool
	if err := runtimePool.QueryRow(t.Context(), `
SELECT has_table_privilege(current_user, 'task_submission_claims', 'SELECT'),
       has_table_privilege(current_user, 'task_submission_claims', 'UPDATE')
`).Scan(&canReadClaims, &canUpdateClaims); err != nil {
		t.Fatalf("inspect task submission runtime privileges: %v", err)
	}
	if !canReadClaims || canUpdateClaims {
		t.Fatalf("runtime task submission claim privileges read/update=%t/%t", canReadClaims, canUpdateClaims)
	}
	store := NewStore(
		runtimePool,
		taskstore.NewRepository(db),
		noOpRiverTxClient{},
		submissionRiverTestQueue,
		submissionRiverTestMaxAttempts,
		taskbilling.NewService(billing.NewService(runtimePool), credits.NewService(runtimePool)),
	)
	task := newBilledAIImageTask("task_style_variation_runtime_role", "style-variation-runtime-role", request)

	result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(task))
	if err != nil {
		t.Fatalf("submit billed style variation as runtime role: %v", err)
	}
	if !result.Created || result.HoldTransactionID == "" {
		t.Fatalf("submission result=%+v", result)
	}
	assertTaskRowCount(t, db, result.TaskID, 1)
	assertBillingSnapshotCount(t, db, result.TaskID, 1)
	assertCreditCommandCount(t, db, result.TaskID, "hold", 1)
	var claimCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_submission_claims WHERE task_id = $1`, result.TaskID).Scan(&claimCount); err != nil {
		t.Fatalf("count task submission claim: %v", err)
	}
	if claimCount != 1 {
		t.Fatalf("task submission claim count=%d want=1", claimCount)
	}
}

func TestBilledSubmissionHoldsAndReturnsOriginalFactsAcrossPriceChange(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 100)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))

	first := newBilledPatternTask("task_billed_first", "pattern-action-1", request)
	firstResult, err := store.SubmitTask(t.Context(), canonicalRequestForTest(first))
	if err != nil {
		t.Fatal(err)
	}
	if !firstResult.Created || firstResult.HoldTransactionID == "" {
		t.Fatalf("first result=%+v", firstResult)
	}
	if _, err := billingService.PublishPrice(t.Context(), submissionPriceInput(mustResolveSKU(t, request), "Pattern Generation", 40)); err != nil {
		t.Fatal(err)
	}

	duplicate := newBilledPatternTask("task_billed_duplicate", "pattern-action-1", request)
	duplicateResult, err := store.SubmitTask(t.Context(), canonicalRequestForTest(duplicate))
	if err != nil {
		t.Fatal(err)
	}
	if duplicateResult.Created || duplicateResult.TaskID != firstResult.TaskID || duplicateResult.HoldTransactionID != firstResult.HoldTransactionID {
		t.Fatalf("duplicate result=%+v first=%+v", duplicateResult, firstResult)
	}
	var displayName string
	var total, reserved int64
	if err := db.QueryRowContext(t.Context(), `SELECT display_name, total_credits FROM task_billing_snapshots WHERE task_id = $1`, firstResult.TaskID).Scan(&displayName, &total); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT balance FROM credit_accounts WHERE wallet_user_id = 42 AND account_role = 'user_reserved'`).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if displayName != "Pattern Generation" || total != 25 || reserved != 25 {
		t.Fatalf("snapshot name/total/reserved=%q/%d/%d, want Pattern Generation/25/25", displayName, total, reserved)
	}
}

func TestConcurrentBilledDuplicateCreatesOneTaskHoldAndRiverJob(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 100)
	client := &countingRiverTxClient{inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)}
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))

	start := make(chan struct{})
	results := make(chan tasks.TaskSubmissionResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, taskID := range []string{"task_billed_concurrent_a", "task_billed_concurrent_b"} {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			<-start
			task := newBilledPatternTask(taskID, "pattern-action-concurrent", request)
			result, err := store.SubmitTask(context.Background(), canonicalRequestForTest(task))
			results <- result
			errs <- err
		}(taskID)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var got []tasks.TaskSubmissionResult
	for result := range results {
		got = append(got, result)
	}
	if len(got) != 2 || got[0].TaskID != got[1].TaskID || got[0].HoldTransactionID == "" || got[0].HoldTransactionID != got[1].HoldTransactionID || got[0].Created == got[1].Created {
		t.Fatalf("results=%+v", got)
	}
	var taskCount, snapshotCount, holdCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM tasks WHERE user_id = 42 AND client_action_id = 'pattern-action-concurrent'`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_billing_snapshots WHERE task_id = $1`, got[0].TaskID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE business_reference_id = $1 AND command_type = 'hold'`, got[0].TaskID).Scan(&holdCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || snapshotCount != 1 || holdCount != 1 || client.InsertCount() != 1 {
		t.Fatalf("task/snapshot/hold/job=%d/%d/%d/%d", taskCount, snapshotCount, holdCount, client.InsertCount())
	}
}

func TestBilledDuplicateFailsClosedWhenBillingFactsAreIncomplete(t *testing.T) {
	for _, missing := range []string{"all", "settlement"} {
		t.Run(missing, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			creditService := credits.NewService(pool)
			billingService := billing.NewService(pool)
			request := seedPatternBillingProduct(t, billingService, 25)
			seedCreditWallet(t, pool, creditService, 42, 100)
			repo := taskstore.NewRepository(db)
			client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
			store := NewStore(pool, repo, client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))

			original := newBilledPatternTask("task_billed_incomplete_"+missing, "pattern-action-incomplete-"+missing, request)
			if missing == "all" {
				if err := repo.Create(t.Context(), original); err != nil {
					t.Fatal(err)
				}
			} else {
				result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(original))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(t.Context(), `DROP TRIGGER task_billing_settlements_state_machine ON task_billing_settlements`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(t.Context(), `DELETE FROM task_billing_settlements WHERE task_id = $1`, result.TaskID); err != nil {
					t.Fatal(err)
				}
			}

			duplicate := newBilledPatternTask("task_billed_duplicate_incomplete_"+missing, original.ClientActionID, request)
			if _, err := store.SubmitTask(t.Context(), canonicalRequestForTest(duplicate)); !errors.Is(err, taskbilling.ErrIntegrity) {
				t.Fatalf("duplicate error=%v, want billing integrity error", err)
			}
		})
	}
}

func TestBilledDuplicateReturnsDurableClaimAfterTaskCancellation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 100)
	repo := taskstore.NewRepository(db)
	client := &countingRiverTxClient{inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)}
	store := NewStore(pool, repo, client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
	original := newBilledPatternTask("task_billed_deleted", "pattern-action-deleted", request)

	first, err := store.SubmitTask(t.Context(), canonicalRequestForTest(original))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := repo.MarkTaskCancelled(t.Context(), tasks.TaskCancellationSnapshot{TaskID: first.TaskID, CancelledAt: time.Now(), Reason: "deleted"}, nil); err != nil || rows != 1 {
		t.Fatalf("cancel rows=%d err=%v", rows, err)
	}
	duplicate := newBilledPatternTask("task_billed_after_delete", original.ClientActionID, request)
	replayed, err := store.SubmitTask(t.Context(), canonicalRequestForTest(duplicate))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Created || replayed.TaskID != first.TaskID || replayed.HoldTransactionID != first.HoldTransactionID {
		t.Fatalf("replayed=%+v first=%+v", replayed, first)
	}
	if client.InsertCount() != 1 {
		t.Fatalf("River inserts=%d, want 1", client.InsertCount())
	}
	var holdCount, claimCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'hold' AND business_reference_id = $1`, first.TaskID).Scan(&holdCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_submission_claims WHERE task_id = $1`, first.TaskID).Scan(&claimCount); err != nil {
		t.Fatal(err)
	}
	if holdCount != 1 || claimCount != 1 {
		t.Fatalf("hold/claim=%d/%d, want 1/1", holdCount, claimCount)
	}
}

func TestBilledDuplicateRejectsDifferentProductForSameClientAction(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	firstRequest := seedPatternBillingProduct(t, billingService, 25)
	secondRequest, err := taskbilling.AIImageRequest(workflow.FeatureSlugStyleVariation, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.PublishPrice(t.Context(), submissionPriceInput(mustResolveSKU(t, secondRequest), "Style Variation", 30)); err != nil {
		t.Fatal(err)
	}
	seedCreditWallet(t, pool, creditService, 42, 100)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
	first := newBilledPatternTask("task_billed_product_first", "pattern-action-product-conflict", firstRequest)
	if _, err := store.SubmitTask(t.Context(), canonicalRequestForTest(first)); err != nil {
		t.Fatal(err)
	}
	conflict := newBilledAIImageTask("task_billed_product_second", first.ClientActionID, secondRequest)
	conflictResult, err := store.SubmitTask(t.Context(), canonicalRequestForTest(conflict))
	if !errors.Is(err, taskbilling.ErrIdempotencyConflict) {
		t.Fatalf("error=%v, want idempotency conflict", err)
	}
	assertTaskRowCount(t, db, conflictResult.TaskID, 0)
}

func TestBilledSubmissionInsufficientCreditsCreatesNothing(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 10)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
	task := newBilledPatternTask("task_billed_insufficient", "pattern-action-insufficient", request)

	result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(task))
	if !errors.Is(err, taskbilling.ErrInsufficientCredits) {
		t.Fatalf("error=%v", err)
	}
	assertTaskRowCount(t, db, result.TaskID, 0)
	assertRiverJobRowCount(t, db, result.TaskID, 0)
	assertBillingSnapshotCount(t, db, result.TaskID, 0)
}

func TestConcurrentBilledSubmissionsLeaveNoWritesForInsufficientLoser(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 25)
	client := &countingRiverTxClient{inner: newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)}
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))

	start := make(chan struct{})
	type submissionOutcome struct {
		taskID string
		err    error
	}
	outcomes := make(chan submissionOutcome, 2)
	var wg sync.WaitGroup
	tasksByID := []*tasks.Task{
		newBilledPatternTask("task_billed_race_a", "pattern-action-race-a", request),
		newBilledPatternTask("task_billed_race_b", "pattern-action-race-b", request),
	}
	for _, task := range tasksByID {
		wg.Add(1)
		go func(task *tasks.Task) {
			defer wg.Done()
			<-start
			result, err := store.SubmitTask(context.Background(), canonicalRequestForTest(task))
			outcomes <- submissionOutcome{taskID: result.TaskID, err: err}
		}(task)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	winnerTaskID := ""
	loserTaskID := ""
	for outcome := range outcomes {
		switch {
		case outcome.err == nil:
			winnerTaskID = outcome.taskID
		case errors.Is(outcome.err, taskbilling.ErrInsufficientCredits):
			loserTaskID = outcome.taskID
		default:
			t.Fatalf("unexpected concurrent submission error for %s: %v", outcome.taskID, outcome.err)
		}
	}
	if winnerTaskID == "" || loserTaskID == "" || winnerTaskID == loserTaskID {
		t.Fatalf("winner/loser=%q/%q, want distinct task ids", winnerTaskID, loserTaskID)
	}

	assertTaskRowCount(t, db, winnerTaskID, 1)
	assertRiverJobRowCount(t, db, winnerTaskID, 1)
	assertBillingSnapshotCount(t, db, winnerTaskID, 1)
	assertCreditCommandCount(t, db, winnerTaskID, "hold", 1)
	assertTaskRowCount(t, db, loserTaskID, 0)
	assertRiverJobRowCount(t, db, loserTaskID, 0)
	assertBillingSnapshotCount(t, db, loserTaskID, 0)
	assertCreditCommandCount(t, db, loserTaskID, "hold", 0)
}

func TestBilledSubmissionRollsBackHoldSnapshotTaskAndRiver(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 25)
	seedCreditWallet(t, pool, creditService, 42, 100)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	store := NewStore(pool, taskstore.NewRepository(db), insertThenFailRiverTxClient{inner: client, err: errors.New("forced River failure")}, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
	task := newBilledPatternTask("task_billed_rollback", "pattern-action-rollback", request)

	result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(task))
	if err == nil {
		t.Fatal("expected submission failure")
	}
	assertTaskRowCount(t, db, result.TaskID, 0)
	assertRiverJobRowCount(t, db, result.TaskID, 0)
	assertBillingSnapshotCount(t, db, result.TaskID, 0)
	wallet, err := creditService.GetWallet(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.Available != 100 || wallet.Reserved != 0 {
		t.Fatalf("wallet=%+v", wallet)
	}
}

func TestFreeBilledSubmissionPersistsSnapshotWithoutLedgerMovement(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	request := seedPatternBillingProduct(t, billingService, 0)
	seedCreditWallet(t, pool, creditService, 42, 0)
	client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
	store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
	task := newBilledPatternTask("task_billed_free", "pattern-action-free", request)

	result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.HoldTransactionID != "" {
		t.Fatalf("free hold=%q", result.HoldTransactionID)
	}
	var state string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM task_billing_settlements WHERE task_id = $1`, result.TaskID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "free" {
		t.Fatalf("state=%q", state)
	}
}

func TestAllAIImageWorkflowSubmissionPricingMatrix(t *testing.T) {
	for _, feature := range workflow.AIImageFeatureSlugs() {
		feature := feature
		for _, scenario := range []struct {
			name          string
			unitCredits   int64
			createSKU     bool
			skuEnabled    bool
			wantErr       error
			wantState     string
			wantHoldCount int
		}{
			{name: "paid", unitCredits: 25, createSKU: true, skuEnabled: true, wantState: "held", wantHoldCount: 1},
			{name: "free", unitCredits: 0, createSKU: true, skuEnabled: true, wantState: "free"},
			{name: "missing", wantErr: billing.ErrSKUNotFound},
			{name: "disabled-sku", unitCredits: 25, createSKU: true, wantErr: billing.ErrPricingUnavailable},
		} {
			scenario := scenario
			t.Run(feature+"/"+scenario.name, func(t *testing.T) {
				db, pool := testdb.OpenWithPool(t)
				creditService := credits.NewService(pool)
				billingService := billing.NewService(pool)
				request, err := taskbilling.AIImageRequest(feature, 1)
				if err != nil {
					t.Fatal(err)
				}
				if scenario.createSKU {
					sku := mustResolveSKU(t, request)
					if _, err := billingService.PublishPrice(t.Context(), submissionPriceInput(sku, feature, scenario.unitCredits)); err != nil {
						t.Fatal(err)
					}
					if !scenario.skuEnabled {
						if _, err := billingService.SetAvailability(t.Context(), string(sku), false, 1); err != nil {
							t.Fatal(err)
						}
					}
				}
				seedCreditWallet(t, pool, creditService, 42, 100)
				client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
				store := NewStore(pool, taskstore.NewRepository(db), client, submissionRiverTestQueue, submissionRiverTestMaxAttempts, taskbilling.NewService(billingService, creditService))
				task := newBilledAIImageTask("task_"+strings.ReplaceAll(feature, "-", "_")+"_"+scenario.name, "action:"+feature+":"+scenario.name, request)

				result, err := store.SubmitTask(t.Context(), canonicalRequestForTest(task))
				if scenario.wantErr != nil {
					if !errors.Is(err, scenario.wantErr) {
						t.Fatalf("error=%v, want %v", err, scenario.wantErr)
					}
					assertTaskRowCount(t, db, result.TaskID, 0)
					assertRiverJobRowCount(t, db, result.TaskID, 0)
					assertBillingSnapshotCount(t, db, result.TaskID, 0)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var snapshotFeature, state string
				if err := db.QueryRowContext(t.Context(), `
SELECT snapshots.feature_slug, settlements.state
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
WHERE snapshots.task_id = $1`, result.TaskID).Scan(&snapshotFeature, &state); err != nil {
					t.Fatal(err)
				}
				if snapshotFeature != feature || state != scenario.wantState {
					t.Fatalf("snapshot feature/state=%q/%q", snapshotFeature, state)
				}
				var holdCount int
				if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'hold' AND business_reference_id = $1`, result.TaskID).Scan(&holdCount); err != nil {
					t.Fatal(err)
				}
				if holdCount != scenario.wantHoldCount {
					t.Fatalf("hold count=%d, want %d", holdCount, scenario.wantHoldCount)
				}
			})
		}
	}
}

func seedPatternBillingProduct(t *testing.T, service *billing.Service, unitCredits int64) taskbilling.Request {
	t.Helper()
	request, err := taskbilling.AIImageRequest(workflow.FeatureSlugPatternGen, 1)
	if err != nil {
		t.Fatal(err)
	}
	sku := mustResolveSKU(t, request)
	if _, err := service.PublishPrice(t.Context(), submissionPriceInput(sku, "Pattern Generation", unitCredits)); err != nil {
		t.Fatal(err)
	}
	return request
}

func submissionPriceInput(sku billing.SKU, displayName string, unitCredits int64) billing.PublishPriceInput {
	return billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: sku, Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: displayName,
		},
		UnitCredits: unitCredits,
		ActorID:     1,
	}
}

func mustResolveSKU(t *testing.T, request taskbilling.Request) billing.SKU {
	t.Helper()
	sku, err := taskbilling.ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	return sku
}

func seedCreditWallet(t *testing.T, pool *pgxpool.Pool, service *credits.Service, userID, amount int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := service.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	if amount > 0 {
		creditAmount, _ := credits.NewCreditAmount(amount)
		if _, err := service.InTx(tx).Grant(t.Context(), credits.GrantCommand{UserID: userID, Amount: creditAmount, Metadata: credits.CommandMetadata{
			IdempotencyKey: "test-grant:" + fmt.Sprint(userID), Actor: credits.Actor{Type: credits.ActorSystem, ID: "test"}, Source: "test", Reason: "test grant", BusinessReference: credits.BusinessReference{Type: "test", ID: fmt.Sprint(userID)},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func newBilledPatternTask(taskID, clientActionID string, request taskbilling.Request) *tasks.Task {
	return newBilledAIImageTask(taskID, clientActionID, request)
}

func newBilledAIImageTask(taskID, clientActionID string, request taskbilling.Request) *tasks.Task {
	task := newTaskSubmissionTask(taskID)
	task.ClientActionID = clientActionID
	task.FeatureSlug = request.Feature
	task.BillingRequest = &tasks.TaskBillingRequest{Feature: request.Feature, Quantity: request.Quantity}
	return task
}

func assertBillingSnapshotCount(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, taskID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_billing_snapshots WHERE task_id = $1`, taskID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("billing snapshot count=%d want=%d", count, want)
	}
}

func assertCreditCommandCount(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, taskID, commandType string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE business_reference_id = $1 AND command_type = $2`, taskID, commandType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("credit %s count=%d want=%d for task %s", commandType, count, want, taskID)
	}
}
