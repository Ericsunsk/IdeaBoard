package reconciliation_test

import (
	"context"
	"errors"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"testing"
	"time"
)

func TestStartReconciliationIsMutuallyExclusiveAndRollsBackOnEnqueueFailure(t *testing.T) {
	pool := testdb.Open(t)
	enqueuer := &reconciliationEnqueuerStub{}
	service := newReconciliationService(t, pool, enqueuer)
	invalid := ports.StartManualReconciliationInput{}
	if _, err := service.StartReconciliation(t.Context(), invalid); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("StartReconciliation(missing actor) error = %v, want domain.ErrInvalidCommand", err)
	}
	first, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	secondAction := testsupport.AdminAction(t, 1)
	secondInput := ports.StartManualReconciliationInput{ActorID: secondAction.ActorID()}
	second, err := service.StartReconciliation(t.Context(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("concurrent active runs = %d and %d", first.ID, second.ID)
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("active Run enqueue calls = %d, want 1", len(enqueuer.calls))
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("run count = %d", got)
	}

	if err := service.ExecuteReconciliation(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	riverErr := errors.New("river unavailable")
	failing := newReconciliationService(t, pool, &reconciliationEnqueuerStub{err: riverErr})
	if _, err := failing.StartReconciliation(t.Context(), reconciliationStartInput(t)); !errors.Is(err, riverErr) {
		t.Fatalf("StartReconciliation() error = %v, want wrapped River error", err)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("run count after enqueue rollback = %d", got)
	}
}

func TestReconciliationExecutionLockIsScopedToSchema(t *testing.T) {
	firstPool := testdb.Open(t)
	secondPool := testdb.Open(t)

	firstTx, err := firstPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = firstTx.Rollback(t.Context()) }()
	if _, err := firstTx.Exec(t.Context(), `
SELECT
  pg_advisory_xact_lock(5283921922134398278),
  pg_advisory_xact_lock(hashtextextended(current_schema(), 5283921922134398278))
`); err != nil {
		t.Fatal(err)
	}

	service := newReconciliationService(t, secondPool, &reconciliationEnqueuerStub{})
	run, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(t.Context(), run.ID); err != nil {
		t.Fatalf("second schema reconciliation was blocked by the first schema: %v", err)
	}
}

func TestReconciliationDetectsLedgerTaskIdempotencyRefundAndStateAnomalies(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 500), Metadata: testhelpers.TestMetadata("matrix:grant", "matrix-grant")}); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := pool.QueryRow(ctx, `SELECT price_version_id, version FROM publish_billing_price('test:reconciliation', 'Reconciliation Test', '', 5, 1)`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	releasedHold := createReconciliationTaskFixture(t, pool, service, "task_reconcile_running_released", "pending", "", 93, priceID, priceVersion)
	release, err := service.Release(ctx, ports.ReleaseCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), HoldTransactionID: releasedHold, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("release:running", "task_reconcile_running_released")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_billing_settlements SET state = 'released', actual_output_count = 0, released_credits = 5, release_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, "task_reconcile_running_released", release.TransactionID); err != nil {
		t.Fatal(err)
	}
	createReconciliationTaskFixture(t, pool, service, "task_reconcile_completed_held", "completed", "", 93, priceID, priceVersion)
	createReconciliationTaskFixture(t, pool, service, "task_reconcile_failed_held", "failed", "provider failed", 93, priceID, priceVersion)

	missingTaskMetadata := taskReconciliationMetadata("hold:missing-task:1", "reconcile-missing-task")
	if _, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 5), Metadata: missingTaskMetadata}); err != nil {
		t.Fatal(err)
	}
	missingTaskMetadata.IdempotencyKey = "hold:missing-task:2"
	if _, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 5), Metadata: missingTaskMetadata}); err != nil {
		t.Fatal(err)
	}

	illegal, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("matrix:illegal", "matrix-illegal")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_transactions SET related_transaction_id = transaction_id WHERE transaction_id = $1`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}

	hold, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 10), Metadata: testhelpers.TestMetadata("matrix:capture-hold", "matrix-capture-hold")})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := service.Capture(ctx, ports.CaptureCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), HoldTransactionID: hold.TransactionID, Amount: testhelpers.MustCreditAmount(t, 10), Metadata: testhelpers.TestMetadata("matrix:capture", "matrix-capture")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_charges DROP CONSTRAINT ck_credit_charges_refunded_range`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_charges SET amount = 11, refunded_amount = 12 WHERE capture_transaction_id = $1`, capture.TransactionID); err != nil {
		t.Fatal(err)
	}

	badHold, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 2), Metadata: testhelpers.TestMetadata("matrix:bad-hold", "matrix-bad-hold")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_holds DROP CONSTRAINT ck_credit_holds_terminal_consistency`); err != nil {
		t.Fatal(err)
	}

	missingBillingHold := createReconciliationTaskFixture(t, pool, service, "task_reconcile_missing_billing_facts", "pending", "", 93, priceID, priceVersion)
	missingRunningHold := createReconciliationTaskFixture(t, pool, service, "task_reconcile_running_missing_billing_facts", "pending", "", 93, priceID, priceVersion)
	// The workspace-scoped tasks table carries no assignment or lease guard
	// that blocks this transition, so the status flip needs no bypass.
	if _, err := pool.Exec(ctx, `UPDATE tasks SET status='running' WHERE public_id='task_reconcile_running_missing_billing_facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_settlements DISABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE task_billing_settlements
SET state = 'captured', actual_output_count = 1, captured_credits = 5, settled_at = clock_timestamp()
WHERE task_id = 'task_reconcile_completed_held'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_snapshots DROP CONSTRAINT task_billing_snapshot_hold_fk`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_holds DROP CONSTRAINT fk_credit_holds_hold_transaction_id`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM task_billing_settlements WHERE task_id = 'task_reconcile_missing_billing_facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM task_billing_settlements WHERE task_id = 'task_reconcile_running_missing_billing_facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM credit_holds WHERE hold_transaction_id = $1`, missingBillingHold); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM credit_holds WHERE hold_transaction_id = $1`, missingRunningHold); err != nil {
		t.Fatal(err)
	}

	adminGrant, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 4), Metadata: domain.CommandMetadata{IdempotencyKey: "matrix:admin-grant", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "1"}, Source: "test", Reason: "admin grant", BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: "matrix-admin-grant"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO credit_transactions (transaction_id,billing_account_id,command_type,idempotency_key,request_hash,actor_type,actor_id,source,reason,business_reference_type,business_reference_id,related_transaction_id,command_amount,posting_count,available_balance_after,reserved_balance_after)
VALUES ('ctx_over_correction',(SELECT ensure_personal_billing_account_for_user(93)),'correction','matrix:over-correction',repeat('c',64),'admin','1','test','over correction','admin_adjustment','over-correction',$1,5,2,0,0)`, adminGrant.TransactionID); err != nil {
		t.Fatal(err)
	}
	var grantFromID, grantToID int64
	if err := pool.QueryRow(ctx, `SELECT MAX(account_id) FILTER (WHERE amount < 0), MAX(account_id) FILTER (WHERE amount > 0) FROM credit_postings WHERE transaction_id=$1`, adminGrant.TransactionID).Scan(&grantFromID, &grantToID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO credit_postings(transaction_id,account_id,amount) VALUES ('ctx_over_correction',$1,-5),('ctx_over_correction',$2,5)`, grantToID, grantFromID); err != nil {
		t.Fatal(err)
	}
	duplicate := testhelpers.TestMetadata("matrix:duplicate-nontask", "duplicate-nontask")
	duplicate.BusinessReference = domain.BusinessReference{Type: "external_order", ID: "order-1"}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: duplicate}); err != nil {
		t.Fatal(err)
	}
	duplicate.IdempotencyKey = "matrix:duplicate-nontask-2"
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: duplicate}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_holds SET status = 'captured' WHERE hold_transaction_id = $1`, badHold.TransactionID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER prevent_credit_postings_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM credit_postings WHERE id = (SELECT MAX(id) FROM credit_postings WHERE transaction_id = $1)`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DROP CONSTRAINT uq_credit_transactions_idempotency_key`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO credit_transactions (transaction_id, billing_account_id, command_type, idempotency_key, request_hash, actor_type, actor_id,
 source, reason, business_reference_type, business_reference_id, command_amount, posting_count, available_balance_after, reserved_balance_after)
SELECT 'ctx_reconcile_hash_conflict', billing_account_id, command_type, idempotency_key, repeat('b', 64), actor_type, actor_id,
 source, reason, business_reference_type, business_reference_id, command_amount, 2, available_balance_after, reserved_balance_after
FROM credit_transactions WHERE transaction_id = $1`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}
	var issuanceID, availableID int64
	if err := pool.QueryRow(ctx, `SELECT MAX(id) FILTER (WHERE account_role='system_issuance'), MAX(id) FILTER (WHERE account_role='user_available') FROM credit_accounts WHERE shard_key='billing-account:' || (SELECT ensure_personal_billing_account_for_user(93))::TEXT OR billing_account_id=(SELECT ensure_personal_billing_account_for_user(93))`).Scan(&issuanceID, &availableID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO credit_postings(transaction_id, account_id, amount) VALUES ('ctx_reconcile_hash_conflict',$1,-1),('ctx_reconcile_hash_conflict',$2,1)`, issuanceID, availableID); err != nil {
		t.Fatal(err)
	}

	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, finding := range page.Findings {
		found[string(finding.FindingType)] = true
	}
	for _, expected := range []string{
		"ledger_postings_invalid", "wallet_projection_mismatch", "task_hold_missing_billing_snapshot",
		"active_task_not_held", "terminal_task_settlement_mismatch",
		"task_terminal_transaction_count", "idempotency_request_hash_conflict", "duplicate_business_reference",
		"refund_amount_invalid", "illegal_reference_chain", "hold_state_invalid", "charge_state_invalid",
		"task_billing_fact_missing", "correction_amount_exceeds_original",
	} {
		if !found[expected] {
			t.Errorf("missing finding type %q; got %v", expected, found)
		}
	}
}

func TestReconciliationRequiresPaidActiveTasksToRemainHeld(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), Amount: testhelpers.MustCreditAmount(t, 50), Metadata: testhelpers.TestMetadata("active-held:grant", "active-held-grant")}); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := pool.QueryRow(ctx, `SELECT price_version_id, version FROM publish_billing_price('test:reconciliation', 'Active Held Test', '', 5, 1)`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	capturedHold := createReconciliationTaskFixture(t, pool, service, "task_active_captured", "pending", "", 93, priceID, priceVersion)
	capture, err := service.Capture(ctx, ports.CaptureCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 93), HoldTransactionID: capturedHold, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("capture:active", "task_active_captured")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_billing_settlements SET state = 'captured', actual_output_count = 1, captured_credits = 5, capture_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, "task_active_captured", capture.TransactionID); err != nil {
		t.Fatal(err)
	}

	createReconciliationTaskFixture(t, pool, service, "task_active_free", "pending", "", 93, priceID, priceVersion)
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_settlements DISABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_billing_settlements SET state = 'free', settled_at = clock_timestamp() WHERE task_id = 'task_active_free'`); err != nil {
		t.Fatal(err)
	}
	createReconciliationTaskFixture(t, pool, service, "task_active_held", "pending", "", 93, priceID, priceVersion)

	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	foundSubjects := make(map[string]bool)
	for _, finding := range page.Findings {
		if finding.FindingType == "active_task_not_held" {
			foundSubjects[finding.SubjectID] = true
			if finding.Risk != ports.ReconciliationRiskHigh {
				t.Errorf("finding %q risk = %q", finding.SubjectID, finding.Risk)
			}
		}
	}
	for _, taskID := range []string{"task_active_captured", "task_active_free"} {
		if !foundSubjects[taskID] {
			t.Errorf("missing active_task_not_held for %q; got %v", taskID, foundSubjects)
		}
	}
	if foundSubjects["task_active_held"] {
		t.Fatal("normally held active task was reported")
	}
}

func TestReconciliationScanDoesNotWaitOnNormalWalletHotRows(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	if _, err := service.Grant(t.Context(), ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 99), Amount: testhelpers.MustCreditAmount(t, 20), Metadata: testhelpers.TestMetadata("nonblock:grant", "nonblock-grant")}); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), `SELECT id FROM credit_accounts WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(99)) ORDER BY id FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := pool.QueryRow(t.Context(), `SELECT run_id FROM start_credit_reconciliation('manual', 1, NULL, FALSE)`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- service.ExecuteReconciliation(context.Background(), runID) }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("ExecuteReconciliation() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconciliation scan waited on a normal Wallet hot-row lock")
	}
}
