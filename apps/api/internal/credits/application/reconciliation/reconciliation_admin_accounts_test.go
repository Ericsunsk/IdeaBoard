package reconciliation_test

// Focused Admin-account isolation tests (#923): Reconciliation, Health, and
// Finding operate on the real funds identity, automatic restriction and
// release affect only the anomalous account plus its own cause, and linked
// Refund/Correction stay available as repair without new refund policy.
//
// These tests run against isolated PostgreSQL schemas (testdb) and use only
// typed Credit Commands plus the typed restriction seam: no direct balance
// or Ledger writes outside the corruption fixture itself.

import (
	"errors"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ensureOrgPoolAccount creates one organization plus its shared pool account
// and provisions the account wallet, returning the internal account id.
func ensureOrgPoolAccount(t testing.TB, pool *pgxpool.Pool, orgPublicID, accountPublicID string, ownerUserID int64) int64 {
	t.Helper()
	ctx := t.Context()
	testhelpers.EnsureAdminOperationUser(t, pool, ownerUserID)
	var orgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (public_id, name) VALUES ($1, $2) RETURNING internal_id`, orgPublicID, "admin-account-fixture").Scan(&orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	var accountID int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, organization_id) VALUES ($1, $2) RETURNING internal_id`, accountPublicID, orgID).Scan(&accountID); err != nil {
		t.Fatalf("insert org pool account: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT provision_billing_account_wallet($1)`, accountID); err != nil {
		t.Fatalf("provision org pool wallet: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO billing_account_members (billing_account_id, user_id, role) VALUES ($1, $2, 'owner')`, accountID, ownerUserID); err != nil {
		t.Fatalf("seed org pool owner: %v", err)
	}
	return accountID
}

// corruptProjection shifts both balance projections of one account off their
// Ledger truth, the way a stuck trigger would.
func corruptProjection(t testing.TB, pool *pgxpool.Pool, accountID int64, availableShift, reservedShift int64) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`)
	})
	if _, err := pool.Exec(ctx, `
UPDATE credit_accounts
SET balance = balance + CASE account_role WHEN 'user_available' THEN $2::BIGINT ELSE $3::BIGINT END
WHERE billing_account_id = $1 AND account_role IN ('user_available', 'user_reserved')`, accountID, availableShift, reservedShift); err != nil {
		t.Fatal(err)
	}
}

// accountBalances captures both projection balances of one account.
type accountBalances struct {
	availableID int64
	reservedID  int64
	available   int64
	reserved    int64
}

func readBalances(t testing.TB, pool *pgxpool.Pool, accountID int64) accountBalances {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id, account_role, balance FROM credit_accounts WHERE billing_account_id = $1 AND account_role IN ('user_available', 'user_reserved')`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var balances accountBalances
	for rows.Next() {
		var id, balance int64
		var role string
		if err := rows.Scan(&id, &role, &balance); err != nil {
			t.Fatal(err)
		}
		if role == "user_available" {
			balances.availableID, balances.available = id, balance
		} else {
			balances.reservedID, balances.reserved = id, balance
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if balances.availableID == 0 || balances.reservedID == 0 {
		t.Fatalf("balances for account %d = %+v, want both roles", accountID, balances)
	}
	return balances
}

// repairProjection restores both balance projections of one account to the
// captured truth.
func repairProjection(t testing.TB, pool *pgxpool.Pool, balances accountBalances) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection`)
	})
	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, balances.availableID, balances.available); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = $2 WHERE id = $1`, balances.reservedID, balances.reserved); err != nil {
		t.Fatal(err)
	}
}

func activeRestrictionCount(t testing.TB, pool *pgxpool.Pool, accountID int64) int64 {
	t.Helper()
	return countRows(t, pool, `SELECT COUNT(*) FROM credit_wallet_restrictions WHERE billing_account_id = $1 AND released_at IS NULL`, accountID)
}

func TestReconciliationIsolatesAnomalousOrgAccountOnly(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()

	const ownerUserID int64 = 201
	const otherUserID int64 = 202
	ownerPersonal := testhelpers.MustBillingAccount(t, pool, ownerUserID)
	otherPersonal := testhelpers.MustBillingAccount(t, pool, otherUserID)
	orgAccount := ensureOrgPoolAccount(t, pool, "org_admin923iso", "ba_admin923orgiso", ownerUserID)

	for account, key := range map[int64]string{orgAccount: "iso:org", ownerPersonal: "iso:owner", otherPersonal: "iso:other"} {
		if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 100), Metadata: testhelpers.TestMetadata(key+":grant", key+"-grant")}); err != nil {
			t.Fatalf("Grant(%d) error = %v", account, err)
		}
	}

	orgTruth := readBalances(t, pool, orgAccount)
	corruptProjection(t, pool, orgAccount, 7, 3)
	executeReconciliationRun(t, service)

	run, err := service.ListReconciliationRuns(ctx, ports.ListReconciliationInput{Limit: 1})
	if err != nil || len(run.Runs) != 1 || run.Runs[0].Status != "completed" || run.Runs[0].IsolatedWalletCount != 1 {
		t.Fatalf("completed run = %+v, err = %v", run, err)
	}
	page, err := service.ListGlobalReconciliationFindings(ctx, ports.ReconciliationFindingOpen, ports.ListReconciliationInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	projectionFindings := 0
	for _, finding := range page.Findings {
		if finding.FindingType != "wallet_projection_mismatch" {
			continue
		}
		projectionFindings++
		if finding.BillingAccountID == nil || *finding.BillingAccountID != orgAccount {
			t.Fatalf("projection finding account = %v, want %d", finding.BillingAccountID, orgAccount)
		}
		if finding.BillingAccountPublicID != "ba_admin923orgiso" {
			t.Fatalf("projection finding billing_account_id = %q, want ba_admin923orgiso", finding.BillingAccountPublicID)
		}
		if !finding.Isolated || finding.Risk != ports.ReconciliationRiskHigh {
			t.Fatalf("projection finding = %+v, want isolated high-risk", finding)
		}
	}
	if projectionFindings != 2 {
		t.Fatalf("projection findings = %d, want 2 (available + reserved)", projectionFindings)
	}
	if got := activeRestrictionCount(t, pool, orgAccount); got != 2 {
		t.Fatalf("org active restrictions = %d, want 2", got)
	}

	// Same Owner, other accounts: untouched and spendable.
	for account, key := range map[int64]string{ownerPersonal: "iso:owner-after", otherPersonal: "iso:other-after"} {
		if got := activeRestrictionCount(t, pool, account); got != 0 {
			t.Fatalf("account %d active restrictions = %d, want 0", account, got)
		}
		if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata(key+":grant", key+"-grant")}); err != nil {
			t.Fatalf("Grant(%d) after org anomaly error = %v, want nil", account, err)
		}
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: orgAccount, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("iso:org-blocked", "iso-org-blocked")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Grant(org) error = %v, want domain.ErrWalletRestricted", err)
	}

	// Duplicate scan reuses the Findings without new restrictions.
	executeReconciliationRun(t, service)
	if got := activeRestrictionCount(t, pool, orgAccount); got != 2 {
		t.Fatalf("org active restrictions after rescan = %d, want 2", got)
	}
	for _, finding := range page.Findings {
		if finding.FindingType != "wallet_projection_mismatch" {
			continue
		}
		if got := countRows(t, pool, `SELECT COUNT(*) FROM credit_reconciliation_findings WHERE fingerprint = $1`, finding.Fingerprint); got != 1 {
			t.Fatalf("finding %s rows = %d, want 1", finding.Fingerprint, got)
		}
	}

	// Repair converges precisely: only the org restriction releases.
	repairProjection(t, pool, orgTruth)
	executeReconciliationRun(t, service)
	if got := activeRestrictionCount(t, pool, orgAccount); got != 0 {
		t.Fatalf("org active restrictions after repair = %d, want 0", got)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: orgAccount, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("iso:org-recovered", "iso-org-recovered")}); err != nil {
		t.Fatalf("Grant(org) after repair error = %v", err)
	}
}

func TestAdminLinkedRepairOnRestrictedAccount(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()

	const userID int64 = 203
	account := testhelpers.MustBillingAccount(t, pool, userID)
	grant, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 50), Metadata: testhelpers.TestMetadata("repair:grant", "repair-grant")})
	if err != nil {
		t.Fatal(err)
	}
	hold, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 20), Metadata: testhelpers.TestMetadata("repair:hold", "repair-hold")})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := service.Capture(ctx, ports.CaptureCommand{BillingAccountID: account, HoldTransactionID: hold.TransactionID, Amount: testhelpers.MustCreditAmount(t, 20), Metadata: testhelpers.TestMetadata("repair:capture", "repair-capture")})
	if err != nil {
		t.Fatal(err)
	}

	restriction, err := service.RestrictWallet(ctx, ports.RestrictWalletCommand{
		BillingAccountID: account,
		Type:             "spending",
		Source:           "control_review",
		RelatedObject:    domain.BusinessReference{Type: "finding", ID: "923"},
		Reason:           "manual admin review",
		Actor:            domain.Actor{Type: domain.ActorAdmin, ID: "11"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if restriction.RestrictionID == "" {
		t.Fatal("restriction id is empty")
	}

	// New spending stays blocked on the restricted account.
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("repair:blocked-grant", "repair-blocked-grant")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Grant() error = %v, want domain.ErrWalletRestricted", err)
	}
	deductMetadata := testhelpers.TestMetadata("repair:blocked-deduct", "repair-blocked-deduct")
	deductMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "11"}
	if _, err := service.Deduct(ctx, ports.DeductionCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: deductMetadata}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Deduct() error = %v, want domain.ErrWalletRestricted", err)
	}
	if _, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("repair:blocked-hold", "repair-blocked-hold")}); !errors.Is(err, domain.ErrWalletRestricted) {
		t.Fatalf("Hold() error = %v, want domain.ErrWalletRestricted", err)
	}

	// Linked repair on the same account keeps its duty: linked Refund of the
	// Capture and Correction of the admin Grant converge, with no new refund
	// policy (amounts stay bounded by their linked facts).
	refundMetadata := testhelpers.TestMetadata("repair:refund", "repair-refund")
	refundMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "11"}
	refund, err := service.Refund(ctx, ports.RefundCommand{BillingAccountID: account, ChargeTransactionID: capture.TransactionID, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: refundMetadata})
	if err != nil {
		t.Fatalf("Refund() on restricted account error = %v", err)
	}
	if refund.Amount != 5 {
		t.Fatalf("refund amount = %d, want 5", refund.Amount)
	}
	correctMetadata := testhelpers.TestMetadata("repair:correct", "repair-correct")
	correctMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "11"}
	if _, err := service.Correct(ctx, ports.CorrectionCommand{BillingAccountID: account, OriginalTransactionID: grant.TransactionID, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: correctMetadata}); err != nil {
		t.Fatalf("Correct() on restricted account error = %v", err)
	}

	// Cross-account references fail closed: another account cannot settle
	// this account's Charge or correct its Grant. Fresh idempotency keys
	// isolate the addressing verdict from key-reuse conflicts.
	const otherUserID int64 = 204
	otherAccount := testhelpers.MustBillingAccount(t, pool, otherUserID)
	crossRefundMetadata := testhelpers.TestMetadata("repair:cross-refund", "repair-cross-refund")
	crossRefundMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "11"}
	if _, err := service.Refund(ctx, ports.RefundCommand{BillingAccountID: otherAccount, ChargeTransactionID: capture.TransactionID, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: crossRefundMetadata}); err == nil {
		t.Fatal("cross-account Refund() succeeded, want failure")
	} else if !errors.Is(err, domain.ErrChargeNotFound) && !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("cross-account Refund() error = %v, want charge-not-found/invalid", err)
	}
	crossCorrectMetadata := testhelpers.TestMetadata("repair:cross-correct", "repair-cross-correct")
	crossCorrectMetadata.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "11"}
	if _, err := service.Correct(ctx, ports.CorrectionCommand{BillingAccountID: otherAccount, OriginalTransactionID: grant.TransactionID, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: crossCorrectMetadata}); err == nil {
		t.Fatal("cross-account Correct() succeeded, want failure")
	} else if !errors.Is(err, domain.ErrInvalidCommand) && !errors.Is(err, domain.ErrChargeNotFound) {
		t.Fatalf("cross-account Correct() error = %v, want invalid/charge-not-found", err)
	}

	// Precise release restores spending without touching other causes.
	release, err := service.ReleaseWalletRestriction(ctx, ports.ReleaseWalletRestrictionCommand{
		RestrictionID: restriction.RestrictionID,
		Actor:         domain.Actor{Type: domain.ActorAdmin, ID: "11"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if release.RestrictionID != restriction.RestrictionID {
		t.Fatalf("released = %q, want %q", release.RestrictionID, restriction.RestrictionID)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: account, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata("repair:recovered", "repair-recovered")}); err != nil {
		t.Fatalf("Grant() after release error = %v", err)
	}
}

func TestPersonalLifecycleFreezeIsolatesSoleOwned(t *testing.T) {
	pool := testdb.Open(t)
	service := newReconciliationService(t, pool, &reconciliationEnqueuerStub{})
	ctx := t.Context()

	const userID int64 = 205
	const otherUserID int64 = 206
	personal := testhelpers.MustBillingAccount(t, pool, userID)
	otherPersonal := testhelpers.MustBillingAccount(t, pool, otherUserID)
	orgAccount := ensureOrgPoolAccount(t, pool, "org_admin923life", "ba_admin923orglife", userID)

	freeze := func(frozen bool) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()
		if err := service.LifecycleInTx(tx).SetSoleOwnedWalletsFrozen(ctx, userID, frozen); err != nil {
			t.Fatalf("SetSoleOwnedWalletsFrozen(%v) error = %v", frozen, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		committed = true
	}

	freeze(true)
	if wallet, err := service.GetWallet(ctx, personal); err != nil || wallet.Status != domain.WalletStatusFrozen {
		t.Fatalf("personal wallet = %+v, err = %v, want frozen", wallet, err)
	}
	// Organization funds never freeze because one member was deactivated,
	// and other users' funds are untouched.
	if wallet, err := service.GetWallet(ctx, orgAccount); err != nil || wallet.Status != domain.WalletStatusActive {
		t.Fatalf("org wallet = %+v, err = %v, want active", wallet, err)
	}
	if wallet, err := service.GetWallet(ctx, otherPersonal); err != nil || wallet.Status != domain.WalletStatusActive {
		t.Fatalf("other wallet = %+v, err = %v, want active", wallet, err)
	}
	// Frozen personal wallets stay historically readable; unfreeze restores
	// spending without touching lifecycle-independent restrictions.
	freeze(false)
	if wallet, err := service.GetWallet(ctx, personal); err != nil || wallet.Status != domain.WalletStatusActive {
		t.Fatalf("personal wallet after unfreeze = %+v, err = %v, want active", wallet, err)
	}
}
