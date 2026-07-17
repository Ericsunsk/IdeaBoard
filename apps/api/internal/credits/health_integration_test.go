package credits

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestCreditHealthUsesCurrentFindingsAndExactLedgerRestrictions(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	health, err := service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthUnchecked {
		t.Fatalf("initial health = %+v, err = %v", health, err)
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

	health, err = service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthHealthy || health.LatestSuccessfulRun == nil || health.LatestSuccessfulRun.ID != runID {
		t.Fatalf("healthy read model = %+v, err = %v", health, err)
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
	health, err = service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthHealthy || health.ActiveRun == nil || health.ActiveRun.ID != activeRunID ||
		health.LatestSuccessfulRun == nil || health.LatestSuccessfulRun.ID != runID {
		t.Fatalf("active run health = %+v, err = %v", health, err)
	}

	if _, err := service.Grant(ctx, GrantCommand{
		UserID:   94,
		Amount:   mustCreditAmount(t, 10),
		Metadata: testMetadata("health:grant:94", "health-grant-94"),
	}); err != nil {
		t.Fatal(err)
	}
	var warningFindingID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO credit_reconciliation_findings (
  fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id,
  first_seen_run_id, last_seen_run_id
) VALUES ($1, 'stale_hold', 'warning', 'low', 94, 'credit_hold', 'hold-94', $2, $2)
RETURNING id`, strings.Repeat("a", 64), runID).Scan(&warningFindingID); err != nil {
		t.Fatal(err)
	}

	health, err = service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthAttention || health.OpenFindings.Total != 1 || health.RestrictedWalletCount != 0 {
		t.Fatalf("attention read model = %+v, err = %v", health, err)
	}

	if _, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        94,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: strconv.FormatInt(warningFindingID, 10)},
		Reason:        "health integration exact restriction",
		Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Grant(ctx, GrantCommand{
		UserID:   95,
		Amount:   mustCreditAmount(t, 10),
		Metadata: testMetadata("health:grant:95", "health-grant-95"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        95,
		Type:          "manual_review",
		Source:        "admin",
		RelatedObject: BusinessReference{Type: "case", ID: "unrelated"},
		Reason:        "unrelated restriction must not affect ledger health count",
		Actor:         Actor{Type: ActorAdmin, ID: "1"},
	}); err != nil {
		t.Fatal(err)
	}

	health, err = service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthCritical || health.RestrictedWalletCount != 1 {
		t.Fatalf("critical read model = %+v, err = %v", health, err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE credit_reconciliation_findings
SET status = 'resolved', resolved_at = transaction_timestamp()
WHERE id = $1`, warningFindingID); err != nil {
		t.Fatal(err)
	}
	health, err = service.GetCreditHealth(ctx)
	if err != nil || health.Status != CreditHealthCritical || health.OpenFindings.Total != 0 || health.RestrictedWalletCount != 1 {
		t.Fatalf("restriction drift health = %+v, err = %v", health, err)
	}
}
