package health_test

import (
	"ideaboard/api/internal/credits/testhelpers"

	"ideaboard/api/internal/credits"

	"ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestCreditHealthUsesCurrentFindingsAndExactLedgerRestrictions(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()

	creditHealth, err := service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthUnchecked {
		t.Fatalf("initial health = %+v, err = %v", creditHealth, err)
	}

	completedAt := time.Now().UTC().Add(-time.Hour)
	var runID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO credit_reconciliation_runs (
  status, trigger_type, scheduled_for, requested_at, started_at, completed_at
) VALUES ('completed', 'scheduled', $1, $1, $1, $1)
RETURNING id`, completedAt).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	creditHealth, err = service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthHealthy || creditHealth.LatestSuccessfulRun == nil || creditHealth.LatestSuccessfulRun.ID != runID {
		t.Fatalf("healthy read model = %+v, err = %v", creditHealth, err)
	}
	var activeRunID int64
	scheduledFor := completedAt.Add(2 * time.Hour)
	if err := pool.QueryRow(ctx, `
INSERT INTO credit_reconciliation_runs (
  status, trigger_type, scheduled_for, requested_at
) VALUES ('queued', 'scheduled', $1, $1)
RETURNING id`, scheduledFor).Scan(&activeRunID); err != nil {
		t.Fatal(err)
	}
	creditHealth, err = service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthHealthy || creditHealth.ActiveRun == nil || creditHealth.ActiveRun.ID != activeRunID ||
		creditHealth.LatestSuccessfulRun == nil || creditHealth.LatestSuccessfulRun.ID != runID {
		t.Fatalf("active run health = %+v, err = %v", creditHealth, err)
	}

	if _, err := service.Grant(ctx, ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94),
		Amount:           testhelpers.MustCreditAmount(t, 10),
		Metadata:         testhelpers.TestMetadata("health:grant:94", "health-grant-94"),
	}); err != nil {
		t.Fatal(err)
	}
	var warningFindingID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO credit_reconciliation_findings (
  fingerprint, finding_type, severity, risk, wallet_user_id, billing_account_id, subject_type, subject_id,
  first_seen_run_id, last_seen_run_id
) VALUES ($1, 'stale_hold', 'warning', 'low', 94, (SELECT ensure_personal_billing_account_for_user(94)), 'credit_hold', 'hold-94', $2, $2)
RETURNING id`, strings.Repeat("a", 64), runID).Scan(&warningFindingID); err != nil {
		t.Fatal(err)
	}

	creditHealth, err = service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthAttention || creditHealth.OpenFindings.Total != 1 || creditHealth.RestrictedWalletCount != 0 {
		t.Fatalf("attention read model = %+v, err = %v", creditHealth, err)
	}

	if _, err := service.RestrictWallet(ctx, ports.RestrictWalletCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94),
		Type:             "ledger_integrity",
		Source:           "reconciliation",
		RelatedObject:    domain.BusinessReference{Type: "finding", ID: strconv.FormatInt(warningFindingID, 10)},
		Reason:           "health integration exact restriction",
		Actor:            domain.Actor{Type: domain.ActorSystem, ID: "reconciliation"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Grant(ctx, ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95),
		Amount:           testhelpers.MustCreditAmount(t, 10),
		Metadata:         testhelpers.TestMetadata("health:grant:95", "health-grant-95"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestrictWallet(ctx, ports.RestrictWalletCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95),
		Type:             "manual_review",
		Source:           "admin",
		RelatedObject:    domain.BusinessReference{Type: "case", ID: "unrelated"},
		Reason:           "unrelated restriction must not affect ledger health count",
		Actor:            domain.Actor{Type: domain.ActorAdmin, ID: "1"},
	}); err != nil {
		t.Fatal(err)
	}

	creditHealth, err = service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthCritical || creditHealth.RestrictedWalletCount != 1 {
		t.Fatalf("critical read model = %+v, err = %v", creditHealth, err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE credit_reconciliation_findings
SET status = 'resolved', resolved_at = transaction_timestamp()
WHERE id = $1`, warningFindingID); err != nil {
		t.Fatal(err)
	}
	creditHealth, err = service.GetCreditHealth(ctx)
	if err != nil || creditHealth.Status != health.CreditHealthCritical || creditHealth.OpenFindings.Total != 0 || creditHealth.RestrictedWalletCount != 1 {
		t.Fatalf("restriction drift health = %+v, err = %v", creditHealth, err)
	}
}
