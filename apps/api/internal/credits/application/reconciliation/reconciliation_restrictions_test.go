package reconciliation_test

import (
	"errors"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"
	"strconv"
	"testing"
)

func TestReconciliationRestrictionsTrackIndependentFindingsAndRecoverCommands(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()

	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 100), Metadata: testhelpers.TestMetadata("reconcile:grant", "reconcile-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	type accountProjection struct {
		id      int64
		balance int64
	}
	accounts := make(map[string]accountProjection)
	rows, err := pool.Query(ctx, `
SELECT account_role, id, balance
FROM credit_accounts
WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(94)) AND account_role IN ('user_available', 'user_reserved')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var role string
		var account accountProjection
		if err := rows.Scan(&role, &account.id, &account.balance); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		accounts[role] = account
	}
	rows.Close()
	if len(accounts) != 2 {
		t.Fatalf("accounts = %+v, want available and reserved", accounts)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE credit_accounts
SET balance = balance + CASE account_role WHEN 'user_available' THEN 7 ELSE 3 END
WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(94)) AND account_role IN ('user_available', 'user_reserved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}

	ledgerBefore := countRows(t, pool, `SELECT COUNT(*) FROM credit_transactions`)
	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatalf("StartReconciliation() error = %v", err)
	}
	if run.Trigger != ports.ReconciliationTriggerManual || run.RequestedBy == nil || *run.RequestedBy != 1 {
		t.Fatalf("manual run audit = %+v", run)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatalf("ExecuteReconciliation() error = %v", err)
	}
	run, err = service.GetReconciliationRun(ctx, run.ID)
	if err != nil || run.Status != "completed" || run.CriticalCount == 0 || run.IsolatedWalletCount != 1 {
		t.Fatalf("completed run = %+v, err = %v", run, err)
	}
	wallet, err := service.GetWallet(ctx, testhelpers.MustBillingAccount(t, pool, 94))
	if err != nil || wallet.Status != domain.WalletStatusActive || len(wallet.ActiveRestrictions) != 2 {
		t.Fatalf("wallet = %+v, err = %v", wallet, err)
	}
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 50})
	if err != nil || len(page.Findings) == 0 {
		t.Fatalf("findings = %+v, err = %v", page, err)
	}
	projectionFindings := make(map[string]ports.ReconciliationFinding)
	for _, finding := range page.Findings {
		if finding.FindingType == "wallet_projection_mismatch" {
			projectionFindings[finding.SubjectID] = finding
		}
	}
	if len(projectionFindings) != 2 {
		t.Fatalf("projection findings = %+v, want two independent account findings", projectionFindings)
	}
	globalOpen, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 1})
	if err != nil || len(globalOpen.Findings) != 1 || globalOpen.NextBeforeID == 0 {
		t.Fatalf("global open findings first page = %+v, err = %v", globalOpen, err)
	}
	globalOpenNext, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{
		BeforeID: globalOpen.NextBeforeID,
		Limit:    1,
	})
	if err != nil || len(globalOpenNext.Findings) != 1 || globalOpenNext.Findings[0].ID >= globalOpen.Findings[0].ID {
		t.Fatalf("global open findings next page = %+v, err = %v", globalOpenNext, err)
	}
	availableFinding := projectionFindings[strconv.FormatInt(accounts["user_available"].id, 10)]
	reservedFinding := projectionFindings[strconv.FormatInt(accounts["user_reserved"].id, 10)]
	for _, finding := range []ports.ReconciliationFinding{availableFinding, reservedFinding} {
		if finding.ID == 0 || !finding.Isolated || finding.Risk != ports.ReconciliationRiskHigh {
			t.Fatalf("projection finding = %+v", finding)
		}
	}
	if got := countRows(t, pool, `
SELECT COUNT(*)
FROM credit_wallet_restrictions
WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(94))
  AND restriction_type = 'ledger_integrity'
  AND source = 'reconciliation'
  AND related_object_type = 'finding'
  AND released_at IS NULL`); got != 2 {
		t.Fatalf("active reconciliation restriction count = %d, want 2", got)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_transactions`); got != ledgerBefore {
		t.Fatalf("ledger transactions changed from %d to %d", ledgerBefore, got)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("reconcile:blocked-grant", "reconcile-blocked-grant")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Grant() error = %v, want domain.ErrWalletRestricted", err)
	}
	deductMetadata := testhelpers.TestMetadata("reconcile:blocked-deduct", "reconcile-blocked-deduct")
	deductMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "1"}
	if _, err := service.Deduct(ctx, ports.DeductionCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: deductMetadata}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Deduct() error = %v, want domain.ErrWalletRestricted", err)
	}
	if _, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("reconcile:blocked-hold", "reconcile-blocked-hold")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Hold() error = %v, want domain.ErrWalletRestricted", err)
	}

	second, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	for _, finding := range []ports.ReconciliationFinding{availableFinding, reservedFinding} {
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_findings WHERE fingerprint = $1`, finding.Fingerprint); got != 1 {
			t.Fatalf("deduplicated finding count = %d, want 1", got)
		}
		if got := countRows(t, pool, `SELECT occurrence_count FROM credit_reconciliation_findings WHERE id = $1`, finding.ID); got != 2 {
			t.Fatalf("occurrence count = %d, want 2", got)
		}
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE finding_id = $1`, finding.ID); got != 2 {
			t.Fatalf("run history links = %d, want 2", got)
		}
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE related_object_type = 'finding' AND related_object_id = $1 AND released_at IS NULL`, strconv.FormatInt(finding.ID, 10)); got != 1 {
			t.Fatalf("active restriction count for finding %d = %d, want 1", finding.ID, got)
		}
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, accounts["user_available"].id, accounts["user_available"].balance); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	third, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM credit_reconciliation_findings WHERE id = $1`, availableFinding.ID).Scan(&status); err != nil || status != "resolved" {
		t.Fatalf("resolved status = %q, err = %v", status, err)
	}
	globalResolved, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingResolved, ports.ListReconciliationInput{Limit: 50})
	if err != nil || len(globalResolved.Findings) != 1 || globalResolved.Findings[0].ID != availableFinding.ID || globalResolved.Findings[0].ResolvedAt == nil {
		t.Fatalf("global resolved findings = %+v, err = %v", globalResolved, err)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(94)) AND released_at IS NULL`); got != 1 {
		t.Fatalf("active restriction count after one repair = %d, want 1", got)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE related_object_id = $1 AND released_at IS NULL`, strconv.FormatInt(reservedFinding.ID, 10)); got != 1 {
		t.Fatalf("reserved finding active restriction count = %d, want 1", got)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("reconcile:still-blocked", "reconcile-still-blocked")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Grant() after one repair error = %v, want domain.ErrWalletRestricted", err)
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, accounts["user_reserved"].id, accounts["user_reserved"].balance); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	fourth, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, fourth.ID); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user(94)) AND released_at IS NULL`); got != 0 {
		t.Fatalf("active restriction count after all repairs = %d, want 0", got)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("reconcile:recovered", "reconcile-recovered")}); err != nil {
		t.Fatalf("Grant() after all repairs error = %v", err)
	}

	var recoveredAvailableBalance int64
	if err := pool.QueryRow(ctx, `SELECT balance FROM credit_accounts WHERE id = $1`, accounts["user_available"].id).Scan(&recoveredAvailableBalance); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, accounts["user_available"].id, recoveredAvailableBalance+7); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	fifth, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, fifth.ID); err != nil {
		t.Fatal(err)
	}
	var reopenedStatus string
	var reopenedOccurrence int64
	if err := pool.QueryRow(ctx, `SELECT status, occurrence_count FROM credit_reconciliation_findings WHERE id = $1`, availableFinding.ID).Scan(&reopenedStatus, &reopenedOccurrence); err != nil {
		t.Fatal(err)
	}
	if reopenedStatus != string(ports.ReconciliationFindingOpen) || reopenedOccurrence != 3 {
		t.Fatalf("reopened finding status=%q occurrence=%d, want open/3", reopenedStatus, reopenedOccurrence)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE related_object_id = $1`, strconv.FormatInt(availableFinding.ID, 10)); got != 2 {
		t.Fatalf("restriction history count after reopen = %d, want 2", got)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE related_object_id = $1 AND released_at IS NULL`, strconv.FormatInt(availableFinding.ID, 10)); got != 1 {
		t.Fatalf("active restriction count after reopen = %d, want 1", got)
	}

	var observedStatus string
	var observedIsolated bool
	if err := pool.QueryRow(ctx, `
SELECT observed_status, observed_isolated
FROM credit_reconciliation_run_findings
WHERE run_id = $1 AND finding_id = $2`, run.ID, availableFinding.ID).Scan(&observedStatus, &observedIsolated); err != nil {
		t.Fatal(err)
	}
	if observedStatus != string(ports.ReconciliationFindingOpen) || !observedIsolated {
		t.Fatalf("historical run snapshot status=%q isolated=%v", observedStatus, observedIsolated)
	}
}

func TestReconciliationStaleHoldOnlyReportsWithoutIsolation(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95), Amount: testhelpers.MustCreditAmount(t, 20), Metadata: testhelpers.TestMetadata("stale:grant", "stale-grant")}); err != nil {
		t.Fatal(err)
	}
	hold, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95), Amount: testhelpers.MustCreditAmount(t, 5), Metadata: testhelpers.TestMetadata("stale:hold", "stale-hold")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_holds SET created_at = clock_timestamp() - INTERVAL '25 hours' WHERE hold_transaction_id = $1`, hold.TransactionID); err != nil {
		t.Fatal(err)
	}
	run, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{})
	if err != nil {
		t.Fatal(err)
	}
	var staleFinding ports.ReconciliationFinding
	for _, finding := range page.Findings {
		if finding.FindingType == "stale_hold" {
			staleFinding = finding
			if finding.Risk != "low" || finding.Severity != "warning" || finding.Isolated {
				t.Fatalf("stale finding = %+v", finding)
			}
		}
	}
	if staleFinding.ID == 0 {
		t.Fatal("stale_hold finding not reported")
	}
	wallet, err := service.GetWallet(ctx, testhelpers.MustBillingAccount(t, pool, 95))
	if err != nil || wallet.Status != domain.WalletStatusActive {
		t.Fatalf("wallet = %+v, err = %v", wallet, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE credit_reconciliation_findings SET risk = 'high', isolated = TRUE WHERE id = $1`, staleFinding.ID); err != nil {
		t.Fatal(err)
	}
	restriction, err := service.RestrictWallet(ctx, ports.RestrictWalletCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95),
		Type:             "ledger_integrity",
		Source:           "reconciliation",
		RelatedObject: domain.BusinessReference{
			Type: "finding",
			ID:   strconv.FormatInt(staleFinding.ID, 10),
		},
		Reason: "test prior high-risk reconciliation isolation",
		Actor:  domain.Actor{Type: domain.ActorSystem, ID: "credits-reconciliation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.StartReconciliation(ctx, reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	var risk string
	var isolated bool
	if err := pool.QueryRow(ctx, `SELECT risk, isolated FROM credit_reconciliation_findings WHERE id = $1`, staleFinding.ID).Scan(&risk, &isolated); err != nil {
		t.Fatal(err)
	}
	if risk != string(ports.ReconciliationRiskLow) || isolated {
		t.Fatalf("downgraded stale finding risk=%q isolated=%v, want low/false", risk, isolated)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE restriction_id = $1 AND released_at IS NOT NULL`, restriction.RestrictionID); got != 1 {
		t.Fatalf("released downgraded restriction count = %d, want 1", got)
	}
}

func TestReconciliationSeparatesBillingSnapshotRiskFromTaskTraceAnomaly(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()
	const userID int64 = 96
	testhelpers.EnsureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), Amount: testhelpers.MustCreditAmount(t, 30), Metadata: testhelpers.TestMetadata("reconciliation-fact-split:grant", "reconciliation-fact-split-grant")}); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := pool.QueryRow(ctx, `SELECT price_version_id, version FROM publish_billing_price('test:reconciliation-fact-split', 'Reconciliation Fact Split', '', 5, 1)`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	const missingSnapshotTaskID = "task_reconcile_missing_billing_snapshot"
	missingSnapshotHold := createReconciliationTaskFixture(t, pool, service, missingSnapshotTaskID, "pending", "", userID, priceID, priceVersion)
	const missingTaskTraceID = "task_reconcile_missing_task_trace"
	createReconciliationTaskFixture(t, pool, service, missingTaskTraceID, "pending", "", userID, priceID, priceVersion)
	const brokenTaskTraceID = "task_reconcile_broken_task_trace"
	createReconciliationTaskFixture(t, pool, service, brokenTaskTraceID, "pending", "", userID, priceID, priceVersion)

	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_snapshots DISABLE TRIGGER task_billing_snapshots_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_settlements DISABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM task_billing_settlements WHERE task_id = $1`, missingSnapshotTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_settlements ENABLE TRIGGER task_billing_settlements_state_machine`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM task_billing_snapshots WHERE task_id = $1`, missingSnapshotTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE task_billing_snapshots ENABLE TRIGGER task_billing_snapshots_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tasks WHERE public_id = $1`, missingTaskTraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tasks WHERE public_id = $1`, brokenTaskTraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_transactions SET business_reference_id = 'wrong-task-trace' WHERE business_reference_type = 'task' AND business_reference_id = $1 AND command_type = 'hold'`, brokenTaskTraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions ENABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}

	first := executeReconciliationRun(t, service)
	findings, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var billingSnapshotFinding, taskTraceFinding, billingFactFinding ports.ReconciliationFinding
	brokenTaskWasReportedOnly := false
	for _, finding := range findings.Findings {
		switch finding.FindingType {
		case ports.ReconciliationFindingTaskHoldMissingBillingSnapshot:
			if finding.SubjectID == missingSnapshotHold {
				billingSnapshotFinding = finding
			}
		case ports.ReconciliationFindingTaskTraceMissing:
			if finding.SubjectID == missingTaskTraceID {
				taskTraceFinding = finding
			}
			if finding.SubjectID == brokenTaskTraceID {
				brokenTaskWasReportedOnly = true
			}
		case ports.ReconciliationFindingTaskBillingFactMissing:
			if finding.SubjectID == brokenTaskTraceID {
				billingFactFinding = finding
			}
		}
	}
	if billingSnapshotFinding.ID == 0 || billingSnapshotFinding.Risk != ports.ReconciliationRiskHigh || !billingSnapshotFinding.Isolated {
		t.Fatalf("Billing Snapshot finding = %+v, want isolated high-risk finding", billingSnapshotFinding)
	}
	if taskTraceFinding.ID == 0 || taskTraceFinding.Risk != ports.ReconciliationRiskLow || taskTraceFinding.Isolated {
		t.Fatalf("Task trace finding = %+v, want report-only low-risk finding", taskTraceFinding)
	}
	if billingFactFinding.ID == 0 || billingFactFinding.Risk != ports.ReconciliationRiskHigh || !billingFactFinding.Isolated {
		t.Fatalf("broken Task billing finding = %+v, want isolated high-risk finding", billingFactFinding)
	}
	if brokenTaskWasReportedOnly {
		t.Fatal("broken Task Billing facts must not be reported as a low-risk Task trace anomaly")
	}
	if got := countRows(t, pool, `
SELECT COUNT(*) FROM credit_wallet_restrictions
WHERE billing_account_id = (SELECT ensure_personal_billing_account_for_user($1)) AND restriction_type = 'ledger_integrity' AND source = 'reconciliation'
  AND related_object_type = 'finding' AND related_object_id = $2 AND released_at IS NULL`, userID, strconv.FormatInt(billingSnapshotFinding.ID, 10)); got != 1 {
		t.Fatalf("Billing Snapshot restriction count = %d, want 1", got)
	}
	if got := countRows(t, pool, `
SELECT COUNT(*) FROM credit_wallet_restrictions
WHERE related_object_type = 'finding' AND related_object_id = $1 AND released_at IS NULL`, strconv.FormatInt(taskTraceFinding.ID, 10)); got != 0 {
		t.Fatalf("Task trace restriction count = %d, want 0", got)
	}

	second := executeReconciliationRun(t, service)
	for _, finding := range []ports.ReconciliationFinding{billingSnapshotFinding, taskTraceFinding} {
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_findings WHERE fingerprint = $1`, finding.Fingerprint); got != 1 {
			t.Fatalf("finding %s count = %d, want one fingerprint", finding.FindingType, got)
		}
		if got := countRows(t, pool, `SELECT occurrence_count FROM credit_reconciliation_findings WHERE id = $1`, finding.ID); got != 2 {
			t.Fatalf("finding %s occurrence count = %d, want 2", finding.FindingType, got)
		}
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE finding_id = $1 AND run_id IN ($2, $3)`, finding.ID, first, second); got != 2 {
			t.Fatalf("finding %s run links = %d, want 2", finding.FindingType, got)
		}
	}

	insertReconciliationTaskBillingSnapshot(t, pool, missingSnapshotTaskID, userID, priceID, priceVersion, missingSnapshotHold)
	if _, err := pool.Exec(ctx, `INSERT INTO task_billing_settlements(task_id,state) VALUES ($1,'held')`, missingSnapshotTaskID); err != nil {
		t.Fatal(err)
	}
	third := executeReconciliationRun(t, service)
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE related_object_type = 'finding' AND related_object_id = $1 AND released_at IS NULL`, strconv.FormatInt(billingSnapshotFinding.ID, 10)); got != 0 {
		t.Fatalf("recovered Billing Snapshot restriction count = %d, want 0", got)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE finding_id = $1 AND run_id = $2`, taskTraceFinding.ID, third); got != 1 {
		t.Fatalf("Task trace finding should remain reported after unrelated recovery, got %d links", got)
	}

	insertReconciliationWorkspaceTask(t, pool, missingTaskTraceID, userID, "pending", "")
	executeReconciliationRun(t, service)
	for _, finding := range []ports.ReconciliationFinding{billingSnapshotFinding, taskTraceFinding} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM credit_reconciliation_findings WHERE id = $1`, finding.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != string(ports.ReconciliationFindingResolved) {
			t.Fatalf("recovered %s finding status = %q, want resolved", finding.FindingType, status)
		}
	}
}

func TestReconciliationTreatsCorruptTerminalFactsAsHighRiskInsteadOfTaskTrace(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()
	const userID int64 = 97
	testhelpers.EnsureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), Amount: testhelpers.MustCreditAmount(t, 20), Metadata: testhelpers.TestMetadata("terminal-fact-risk:grant", "terminal-fact-risk-grant")}); err != nil {
		t.Fatal(err)
	}
	var priceID, priceVersion int64
	if err := pool.QueryRow(ctx, `SELECT price_version_id, version FROM publish_billing_price('test:terminal-fact-risk', 'Terminal Fact Risk', '', 5, 1)`).Scan(&priceID, &priceVersion); err != nil {
		t.Fatal(err)
	}

	const capturedTaskID = "task_reconcile_corrupt_captured"
	capturedHold := createReconciliationTaskFixture(t, pool, service, capturedTaskID, "completed", "", userID, priceID, priceVersion)
	capture, err := service.Capture(ctx, ports.CaptureCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), HoldTransactionID: capturedHold, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("terminal-fact-risk:capture", capturedTaskID)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_billing_settlements SET state = 'captured', actual_output_count = 1, captured_credits = 5, capture_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, capturedTaskID, capture.TransactionID); err != nil {
		t.Fatal(err)
	}

	const releasedTaskID = "task_reconcile_corrupt_released"
	releasedHold := createReconciliationTaskFixture(t, pool, service, releasedTaskID, "failed", "provider failed", userID, priceID, priceVersion)
	release, err := service.Release(ctx, ports.ReleaseCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), HoldTransactionID: releasedHold, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("terminal-fact-risk:release", releasedTaskID)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_billing_settlements SET state = 'released', actual_output_count = 0, released_credits = 5, release_transaction_id = $2, settled_at = clock_timestamp() WHERE task_id = $1`, releasedTaskID, release.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tasks WHERE public_id IN ($1, $2)`, capturedTaskID, releasedTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_transactions SET command_amount = 4 WHERE transaction_id = $1`, capture.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_transactions ENABLE TRIGGER prevent_credit_transactions_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_postings DISABLE TRIGGER prevent_credit_postings_mutation`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_postings SET amount = CASE WHEN amount > 0 THEN 4 ELSE -4 END WHERE transaction_id = $1`, release.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_postings ENABLE TRIGGER prevent_credit_postings_mutation`); err != nil {
		t.Fatal(err)
	}

	executeReconciliationRun(t, service)
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var capturedFactRisk, releasedLedgerRisk ports.ReconciliationFinding
	for _, finding := range page.Findings {
		if finding.FindingType == ports.ReconciliationFindingTaskTraceMissing && (finding.SubjectID == capturedTaskID || finding.SubjectID == releasedTaskID) {
			t.Fatalf("corrupt terminal %q was incorrectly reported as low-risk Task trace", finding.SubjectID)
		}
		if finding.FindingType == ports.ReconciliationFindingTaskBillingFactMissing && finding.SubjectID == capturedTaskID {
			capturedFactRisk = finding
		}
		if finding.FindingType == ports.ReconciliationFindingLedgerPostingsInvalid && finding.SubjectID == release.TransactionID {
			releasedLedgerRisk = finding
		}
	}
	for _, finding := range []ports.ReconciliationFinding{capturedFactRisk, releasedLedgerRisk} {
		if finding.ID == 0 || finding.Risk != ports.ReconciliationRiskHigh || !finding.Isolated {
			t.Fatalf("corrupt terminal finding = %+v, want isolated high-risk finding", finding)
		}
	}
}
