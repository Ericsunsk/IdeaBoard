package credits

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type reconciliationEnqueuerStub struct {
	mu    sync.Mutex
	calls []ReconciliationJobArgs
	err   error
}

func (e *reconciliationEnqueuerStub) InsertTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if e.err != nil {
		return nil, e.err
	}
	typed, ok := args.(ReconciliationJobArgs)
	if !ok {
		return nil, errors.New("unexpected reconciliation args")
	}
	e.mu.Lock()
	e.calls = append(e.calls, typed)
	e.mu.Unlock()
	return &rivertype.JobInsertResult{}, nil
}

func TestReconciliationRunPersistsFindingsIsolatesAndDeduplicates(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	service.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{})
	ctx := t.Context()

	if _, err := service.Grant(ctx, GrantCommand{UserID: 94, Amount: mustCreditAmount(t, 100), Metadata: testMetadata("reconcile:grant", "reconcile-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	var accountID int64
	var originalBalance int64
	if err := db.QueryRowContext(ctx, `SELECT id, balance FROM credit_accounts WHERE wallet_user_id = 94 AND account_role = 'user_available'`).Scan(&accountID, &originalBalance); err != nil {
		t.Fatalf("read account error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_accounts SET balance = balance + 7 WHERE id = $1`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}

	ledgerBefore := countRows(t, db, `SELECT COUNT(*) FROM credit_transactions`)
	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatalf("StartReconciliation() error = %v", err)
	}
	if run.Reason != "integration reconciliation" {
		t.Fatalf("run reason = %q", run.Reason)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatalf("ExecuteReconciliation() error = %v", err)
	}
	run, err = service.GetReconciliationRun(ctx, run.ID)
	if err != nil || run.Status != "completed" || run.CriticalCount == 0 || run.IsolatedWalletCount != 1 {
		t.Fatalf("completed run = %+v, err = %v", run, err)
	}
	wallet, err := service.GetWallet(ctx, 94)
	if err != nil || wallet.Status != WalletStatusFrozen {
		t.Fatalf("wallet = %+v, err = %v", wallet, err)
	}
	page, err := service.ListReconciliationFindings(ctx, run.ID, "open", ListReconciliationInput{Limit: 50})
	if err != nil || len(page.Findings) == 0 {
		t.Fatalf("findings = %+v, err = %v", page, err)
	}
	var projectionFinding ReconciliationFinding
	for _, finding := range page.Findings {
		if finding.FindingType == "wallet_projection_mismatch" {
			projectionFinding = finding
		}
	}
	if projectionFinding.ID == 0 || !projectionFinding.Isolated || projectionFinding.Risk != "high" {
		t.Fatalf("projection finding = %+v", projectionFinding)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_alerts WHERE finding_id = $1`, projectionFinding.ID); got != 1 {
		t.Fatalf("alert count = %d, want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_transactions`); got != ledgerBefore {
		t.Fatalf("ledger transactions changed from %d to %d", ledgerBefore, got)
	}

	second, err := service.StartReconciliation(ctx, reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_findings WHERE fingerprint = $1`, projectionFinding.Fingerprint); got != 1 {
		t.Fatalf("deduplicated finding count = %d, want 1", got)
	}
	if got := countRows(t, db, `SELECT occurrence_count FROM credit_reconciliation_findings WHERE id = $1`, projectionFinding.ID); got != 2 {
		t.Fatalf("occurrence count = %d, want 2", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE finding_id = $1`, projectionFinding.ID); got != 2 {
		t.Fatalf("run history links = %d, want 2", got)
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, accountID, originalBalance); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	third, err := service.StartReconciliation(ctx, reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM credit_reconciliation_findings WHERE id = $1`, projectionFinding.ID).Scan(&status); err != nil || status != "resolved" {
		t.Fatalf("resolved status = %q, err = %v", status, err)
	}
	historical, err := service.ListReconciliationFindings(ctx, run.ID, "open", ListReconciliationInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	historicalFound := false
	for _, finding := range historical.Findings {
		if finding.ID == projectionFinding.ID {
			historicalFound = true
			if finding.Status != ReconciliationFindingOpen || !finding.Isolated {
				t.Fatalf("historical snapshot = %+v", finding)
			}
		}
	}
	if !historicalFound {
		t.Fatal("original run lost its open finding snapshot")
	}
}

func TestReconciliationStaleHoldOnlyReportsWithoutIsolation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	service.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, GrantCommand{UserID: 95, Amount: mustCreditAmount(t, 20), Metadata: testMetadata("stale:grant", "stale-grant")}); err != nil {
		t.Fatal(err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: 95, Amount: mustCreditAmount(t, 5), Metadata: testMetadata("stale:hold", "stale-hold")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_holds SET created_at = clock_timestamp() - INTERVAL '2 hours' WHERE hold_transaction_id = $1`, hold.TransactionID); err != nil {
		t.Fatal(err)
	}
	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListReconciliationFindings(ctx, run.ID, "open", ListReconciliationInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range page.Findings {
		if finding.FindingType == "stale_hold" {
			found = true
			if finding.Risk != "low" || finding.Severity != "warning" || finding.Isolated {
				t.Fatalf("stale finding = %+v", finding)
			}
		}
	}
	if !found {
		t.Fatal("stale_hold finding not reported")
	}
	wallet, err := service.GetWallet(ctx, 95)
	if err != nil || wallet.Status != WalletStatusActive {
		t.Fatalf("wallet = %+v, err = %v", wallet, err)
	}
}

func TestStartReconciliationIsMutuallyExclusiveAndRollsBackOnEnqueueFailure(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	enqueuer := &reconciliationEnqueuerStub{}
	service.SetReconciliationEnqueuer(enqueuer)
	invalid := reconciliationStartInput(t, 24*time.Hour)
	invalid.Action = adminaction.Metadata{}
	if _, err := service.StartReconciliation(t.Context(), invalid); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("StartReconciliation(blank reason) error = %v, want ErrInvalidCommand", err)
	}
	first, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	secondInput := reconciliationStartInput(t, 24*time.Hour)
	secondInput.Action = testsupport.AdminAction(t, 1, "second request must not overwrite active run reason")
	second, err := service.StartReconciliation(t.Context(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Reason != first.Reason {
		t.Fatalf("concurrent active runs = %d and %d", first.ID, second.ID)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("run count = %d", got)
	}

	if err := service.ExecuteReconciliation(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	failing := NewService(pool)
	riverErr := errors.New("river unavailable")
	failing.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{err: riverErr})
	if _, err := failing.StartReconciliation(t.Context(), reconciliationStartInput(t, 24*time.Hour)); !errors.Is(err, riverErr) {
		t.Fatalf("StartReconciliation() error = %v, want wrapped River error", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM credit_reconciliation_runs`); got != 1 {
		t.Fatalf("run count after enqueue rollback = %d", got)
	}
}

func TestReconciliationDetectsLedgerTaskIdempotencyRefundAndStateAnomalies(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	service.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 500), Metadata: testMetadata("matrix:grant", "matrix-grant")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `SELECT create_billing_sku('test:reconciliation', 'Reconciliation Test', '', TRUE, 1, 'test setup')`); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := db.QueryRowContext(ctx, `SELECT price_version_id, version FROM create_billing_price_version('test:reconciliation', 5, TRUE, 1, 'test setup')`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	releasedHold := createReconciliationTaskFixture(t, db, service, "reconcile-running-released", "pending", "", 93, priceID, priceVersion)
	release, err := service.Release(ctx, ReleaseCommand{UserID: 93, HoldTransactionID: releasedHold, Metadata: taskReconciliationMetadata("release:running", "reconcile-running-released")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_billing_settlements SET state = 'released', settlement_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, "reconcile-running-released", release.TransactionID); err != nil {
		t.Fatal(err)
	}
	createReconciliationTaskFixture(t, db, service, "reconcile-completed-held", "completed", "", 93, priceID, priceVersion)
	createReconciliationTaskFixture(t, db, service, "reconcile-failed-held", "failed", "provider failed", 93, priceID, priceVersion)

	missingTaskMetadata := taskReconciliationMetadata("hold:missing-task:1", "reconcile-missing-task")
	if _, err := service.Hold(ctx, HoldCommand{UserID: 93, Amount: mustCreditAmount(t, 5), Metadata: missingTaskMetadata}); err != nil {
		t.Fatal(err)
	}
	missingTaskMetadata.IdempotencyKey = "hold:missing-task:2"
	if _, err := service.Hold(ctx, HoldCommand{UserID: 93, Amount: mustCreditAmount(t, 5), Metadata: missingTaskMetadata}); err != nil {
		t.Fatal(err)
	}

	illegal, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("matrix:illegal", "matrix-illegal")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_transactions SET related_transaction_id = transaction_id WHERE transaction_id = $1`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}

	hold, err := service.Hold(ctx, HoldCommand{UserID: 93, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("matrix:capture-hold", "matrix-capture-hold")})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: 93, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("matrix:capture", "matrix-capture")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_charges DROP CONSTRAINT ck_credit_charges_refunded_range`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_charges SET amount = 11, refunded_amount = 12 WHERE capture_transaction_id = $1`, capture.TransactionID); err != nil {
		t.Fatal(err)
	}

	badHold, err := service.Hold(ctx, HoldCommand{UserID: 93, Amount: mustCreditAmount(t, 2), Metadata: testMetadata("matrix:bad-hold", "matrix-bad-hold")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_holds DROP CONSTRAINT ck_credit_holds_terminal_consistency`); err != nil {
		t.Fatal(err)
	}

	missingBillingHold := createReconciliationTaskFixture(t, db, service, "reconcile-missing-billing-facts", "pending", "", 93, priceID, priceVersion)
	missingRunningHold := createReconciliationTaskFixture(t, db, service, "reconcile-running-missing-billing-facts", "pending", "", 93, priceID, priceVersion)
	if _, err := db.ExecContext(ctx, `ALTER TABLE tasks DROP CONSTRAINT ck_tasks_assignment_status`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE tasks DROP CONSTRAINT ck_tasks_assignment_pair`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE tasks DROP CONSTRAINT ck_tasks_lease_status`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET status='running' WHERE task_id='reconcile-running-missing-billing-facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE task_billing_settlements DISABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE task_billing_snapshots DROP CONSTRAINT task_billing_snapshot_hold_fk`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_holds DROP CONSTRAINT fk_credit_holds_hold_transaction_id`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM task_billing_settlements WHERE task_id = 'reconcile-missing-billing-facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM task_billing_settlements WHERE task_id = 'reconcile-running-missing-billing-facts'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM credit_holds WHERE hold_transaction_id = $1`, missingBillingHold); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM credit_holds WHERE hold_transaction_id = $1`, missingRunningHold); err != nil {
		t.Fatal(err)
	}

	adminGrant, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 4), Metadata: CommandMetadata{IdempotencyKey: "matrix:admin-grant", Actor: Actor{Type: ActorAdmin, ID: "1"}, Source: "test", Reason: "admin grant", BusinessReference: BusinessReference{Type: "admin_adjustment", ID: "matrix-admin-grant"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO credit_transactions (transaction_id,wallet_user_id,command_type,idempotency_key,request_hash,actor_type,actor_id,source,reason,business_reference_type,business_reference_id,related_transaction_id,command_amount,posting_count,available_balance_after,reserved_balance_after)
VALUES ('ctx_over_correction',93,'correction','matrix:over-correction',repeat('c',64),'admin','1','test','over correction','admin_adjustment','over-correction',$1,5,2,0,0)`, adminGrant.TransactionID); err != nil {
		t.Fatal(err)
	}
	var grantFromID, grantToID int64
	if err := db.QueryRowContext(ctx, `SELECT MAX(account_id) FILTER (WHERE amount < 0), MAX(account_id) FILTER (WHERE amount > 0) FROM credit_postings WHERE transaction_id=$1`, adminGrant.TransactionID).Scan(&grantFromID, &grantToID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO credit_postings(transaction_id,account_id,amount) VALUES ('ctx_over_correction',$1,-5),('ctx_over_correction',$2,5)`, grantToID, grantFromID); err != nil {
		t.Fatal(err)
	}
	duplicate := testMetadata("matrix:duplicate-nontask", "duplicate-nontask")
	duplicate.BusinessReference = BusinessReference{Type: "external_order", ID: "order-1"}
	if _, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 1), Metadata: duplicate}); err != nil {
		t.Fatal(err)
	}
	duplicate.IdempotencyKey = "matrix:duplicate-nontask-2"
	if _, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 1), Metadata: duplicate}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE credit_holds SET status = 'captured' WHERE hold_transaction_id = $1`, badHold.TransactionID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER prevent_credit_postings_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM credit_postings WHERE id = (SELECT MAX(id) FROM credit_postings WHERE transaction_id = $1)`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_transactions DROP CONSTRAINT uq_credit_transactions_idempotency_key`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO credit_transactions (transaction_id, wallet_user_id, command_type, idempotency_key, request_hash, actor_type, actor_id,
 source, reason, business_reference_type, business_reference_id, command_amount, posting_count, available_balance_after, reserved_balance_after)
SELECT 'ctx_reconcile_hash_conflict', wallet_user_id, command_type, idempotency_key, repeat('b', 64), actor_type, actor_id,
 source, reason, business_reference_type, business_reference_id, command_amount, 2, available_balance_after, reserved_balance_after
FROM credit_transactions WHERE transaction_id = $1`, illegal.TransactionID); err != nil {
		t.Fatal(err)
	}
	var issuanceID, availableID int64
	if err := db.QueryRowContext(ctx, `SELECT MAX(id) FILTER (WHERE account_role='system_issuance'), MAX(id) FILTER (WHERE account_role='user_available') FROM credit_accounts WHERE shard_key='wallet:93' OR wallet_user_id=93`).Scan(&issuanceID, &availableID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO credit_postings(transaction_id, account_id, amount) VALUES ('ctx_reconcile_hash_conflict',$1,-1),('ctx_reconcile_hash_conflict',$2,1)`, issuanceID, availableID); err != nil {
		t.Fatal(err)
	}

	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListReconciliationFindings(ctx, run.ID, "open", ListReconciliationInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, finding := range page.Findings {
		found[finding.FindingType] = true
	}
	for _, expected := range []string{
		"ledger_postings_invalid", "wallet_projection_mismatch", "task_hold_missing_task",
		"active_task_not_held", "completed_task_not_captured", "failed_task_not_released",
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
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	service.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, GrantCommand{UserID: 93, Amount: mustCreditAmount(t, 50), Metadata: testMetadata("active-held:grant", "active-held-grant")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `SELECT create_billing_sku('test:reconciliation', 'Active Held Test', '', TRUE, 1, 'test setup')`); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := db.QueryRowContext(ctx, `SELECT price_version_id, version FROM create_billing_price_version('test:reconciliation', 5, TRUE, 1, 'test setup')`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	capturedHold := createReconciliationTaskFixture(t, db, service, "active-captured", "pending", "", 93, priceID, priceVersion)
	capture, err := service.Capture(ctx, CaptureCommand{UserID: 93, HoldTransactionID: capturedHold, Metadata: taskReconciliationMetadata("capture:active", "active-captured")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_billing_settlements SET state = 'captured', settlement_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, "active-captured", capture.TransactionID); err != nil {
		t.Fatal(err)
	}

	createReconciliationTaskFixture(t, db, service, "active-free", "pending", "", 93, priceID, priceVersion)
	if _, err := db.ExecContext(ctx, `ALTER TABLE task_billing_settlements DISABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_billing_settlements SET state = 'free', settled_at = clock_timestamp() WHERE task_id = 'active-free'`); err != nil {
		t.Fatal(err)
	}
	createReconciliationTaskFixture(t, db, service, "active-held", "pending", "", 93, priceID, priceVersion)

	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t, 24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListReconciliationFindings(ctx, run.ID, ReconciliationFindingOpen, ListReconciliationInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	foundSubjects := make(map[string]bool)
	for _, finding := range page.Findings {
		if finding.FindingType == "active_task_not_held" {
			foundSubjects[finding.SubjectID] = true
			if finding.Risk != ReconciliationRiskHigh {
				t.Errorf("finding %q risk = %q", finding.SubjectID, finding.Risk)
			}
		}
	}
	for _, taskID := range []string{"active-captured", "active-free"} {
		if !foundSubjects[taskID] {
			t.Errorf("missing active_task_not_held for %q; got %v", taskID, foundSubjects)
		}
	}
	if foundSubjects["active-held"] {
		t.Fatal("normally held active task was reported")
	}
}

func TestReconciliationWorkerRetriesTransientFailureAndMarksOnlyFinalFailure(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	var runID int64
	if err := db.QueryRowContext(t.Context(), `SELECT start_credit_reconciliation(1, 86400, 'worker execution test')`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE credit_postings CASCADE`); err != nil {
		t.Fatal(err)
	}
	worker := NewReconciliationWorker(service)
	first := &river.Job[ReconciliationJobArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 2}, Args: ReconciliationJobArgs{RunID: runID}}
	if err := worker.Work(t.Context(), first); err == nil {
		t.Fatal("transient Work() error = nil")
	}
	run, err := service.GetReconciliationRun(t.Context(), runID)
	if err != nil || run.Status != "queued" {
		t.Fatalf("transient run = %+v, err = %v", run, err)
	}
	final := &river.Job[ReconciliationJobArgs]{JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 2}, Args: ReconciliationJobArgs{RunID: runID}}
	if err := worker.Work(t.Context(), final); err != nil {
		t.Fatalf("final Work() error = %v", err)
	}
	run, err = service.GetReconciliationRun(t.Context(), runID)
	if err != nil || run.Status != "failed" || run.ErrorMessage == "" {
		t.Fatalf("final run = %+v, err = %v", run, err)
	}
}

func TestReconciliationWorkerMarksFinalFailureAfterParentCancellation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	var runID int64
	if err := db.QueryRowContext(t.Context(), `SELECT start_credit_reconciliation(1, 86400, 'worker failure test')`).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	parent, cancel := context.WithCancel(t.Context())
	cancel()
	job := &river.Job[ReconciliationJobArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1}, Args: ReconciliationJobArgs{RunID: runID}}
	if err := NewReconciliationWorker(service).Work(parent, job); err != nil {
		t.Fatalf("final Work() error = %v", err)
	}
	run, err := service.GetReconciliationRun(t.Context(), runID)
	if err != nil || run.Status != ReconciliationRunFailed || run.ErrorMessage == "" {
		t.Fatalf("final run = %+v, err = %v", run, err)
	}
}

func TestReconciliationWorkerBoundsFinalFailureContextAndJoinsErrors(t *testing.T) {
	executeErr := errors.New("execute reconciliation")
	failErr := errors.New("mark reconciliation failed")
	service := &reconciliationWorkerServiceStub{executeErr: executeErr, failErr: failErr}
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	job := &river.Job[ReconciliationJobArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1}, Args: ReconciliationJobArgs{RunID: 42}}

	err := NewReconciliationWorker(service).Work(parent, job)
	if !errors.Is(err, executeErr) || !errors.Is(err, failErr) {
		t.Fatalf("Work() error = %v, want joined execute and fail errors", err)
	}
	if service.failContextErr != nil {
		t.Fatalf("FailReconciliation() context error = %v", service.failContextErr)
	}
	if service.failDeadline.IsZero() {
		t.Fatal("FailReconciliation() context has no deadline")
	}
	remaining := time.Until(service.failDeadline)
	if remaining <= 0 || remaining > reconciliationFinalFailureTimeout {
		t.Fatalf("FailReconciliation() deadline remaining = %v", remaining)
	}
}

type reconciliationWorkerServiceStub struct {
	executeErr     error
	failErr        error
	failContextErr error
	failDeadline   time.Time
}

func (s *reconciliationWorkerServiceStub) ExecuteReconciliation(context.Context, int64) error {
	return s.executeErr
}

func (s *reconciliationWorkerServiceStub) FailReconciliation(ctx context.Context, _ int64, _ error) error {
	s.failContextErr = ctx.Err()
	s.failDeadline, _ = ctx.Deadline()
	return s.failErr
}

func TestReconciliationScanDoesNotWaitOnNormalWalletHotRows(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	if _, err := service.Grant(t.Context(), GrantCommand{UserID: 99, Amount: mustCreditAmount(t, 20), Metadata: testMetadata("nonblock:grant", "nonblock-grant")}); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), `SELECT id FROM credit_accounts WHERE wallet_user_id = 99 ORDER BY id FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := pool.QueryRow(t.Context(), `SELECT start_credit_reconciliation(1, 86400, 'hot row test')`).Scan(&runID); err != nil {
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

func createReconciliationTaskFixture(t *testing.T, db *sql.DB, service *Service, taskID, status, errorMessage string, userID, priceID, priceVersion int64) string {
	t.Helper()
	hold, err := service.Hold(t.Context(), HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("hold:"+taskID, taskID)})
	if err != nil {
		t.Fatal(err)
	}
	completedAt := "NULL"
	if status == "completed" || status == "failed" {
		completedAt = "clock_timestamp()"
	}
	query := `INSERT INTO tasks (task_id,user_id,prompt,user_prompt,params,status,error_msg,result,completed_at) VALUES ($1,$2,'p','p','{}',$3,$4,'',` + completedAt + `)`
	if _, err := db.ExecContext(t.Context(), query, taskID, userID, status, errorMessage); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO task_billing_snapshots(task_id,user_id,feature_slug,model,sku,display_name,price_version_id,price_version,unit_credits,quantity,total_credits,quoted_at,hold_transaction_id)
VALUES ($1,$2,'test','test','test:reconciliation','Reconciliation',$3,$4,5,1,5,clock_timestamp(),$5)`, taskID, userID, priceID, priceVersion, hold.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO task_billing_settlements(task_id,state) VALUES ($1,'held')`, taskID); err != nil {
		t.Fatal(err)
	}
	return hold.TransactionID
}

func taskReconciliationMetadata(key, taskID string) CommandMetadata {
	return CommandMetadata{IdempotencyKey: key, Actor: Actor{Type: ActorService, ID: "reconciliation-test"}, Source: "test", Reason: "reconciliation matrix", BusinessReference: BusinessReference{Type: "task", ID: taskID}}
}

func reconciliationStartInput(t testing.TB, staleHoldAfter time.Duration) StartReconciliationInput {
	return StartReconciliationInput{Action: testsupport.AdminAction(t, 1, "integration reconciliation"), StaleHoldAfter: staleHoldAfter}
}

func countRows(t testing.TB, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	return count
}
