// Vision Search workspace budget tests (#945).
//
// The budget check joins the Vision reservation transaction after Quote
// and before Hold: an over-cap request fails closed with
// ErrVisionBudgetExhausted (403 workspace_budget_exhausted) and leaves
// no operation row and no Hold. In-flight operations converge on their
// submit-time payer and period facts, so terminal Capture/Release needs
// no re-check.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"testing"
	"time"

	searchdomain "ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/workspacebudgets"
)

// TestVisionBudgetExhaustedBlocksReserve proves Task and Vision share one
// workspace cap: a succeeded Vision Search consumes budget, and the next
// request over the cap fails before any ledger or operation write while
// the funded wallet stays untouched by the block.
func TestVisionBudgetExhaustedBlocksReserve(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 25)
	ctx := context.Background()
	owner := env.createUser(t, "vision-budget-owner")
	member := env.createUser(t, "vision-budget-member")
	ws := "ws_visionbudg01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	var wsInternal int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, ws).Scan(&wsInternal); err != nil {
		t.Fatal(err)
	}
	budgets := workspacebudgets.NewStore(env.pool)
	limit := int64(30)
	if _, err := budgets.SetBudget(ctx, wsInternal, workspacebudgets.PeriodMonthly, &limit, owner); err != nil {
		t.Fatalf("set vision budget: %v", err)
	}

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "budget dunes"}})
	service.SetBudgetChecker(budgets)

	first, err := service.Extract(ctx, ws, member, "vision-budget-1", visionTestInput())
	if err != nil {
		t.Fatalf("first Extract() error = %v", err)
	}
	if first.Status != string(VisionOperationSucceeded) {
		t.Fatalf("first status = %q, want succeeded", first.Status)
	}
	if got := env.operationCount(t, ws, "vision-budget-1"); got != 1 {
		t.Fatalf("first operation rows = %d, want 1", got)
	}

	// 25 consumed + 25 requested exceeds the 30 cap on a funded wallet:
	// budget exhaustion, never insufficiency, with no residue.
	_, err = service.Extract(ctx, ws, member, "vision-budget-2", visionTestInput())
	if !errors.Is(err, ErrVisionBudgetExhausted) {
		t.Fatalf("second Extract() error = %v, want ErrVisionBudgetExhausted", err)
	}
	if got := env.operationCount(t, ws, "vision-budget-2"); got != 0 {
		t.Fatalf("blocked operation rows = %d, want 0 (no residue)", got)
	}
	if holds, captures := env.ledgerCount(t, "hold", account), env.ledgerCount(t, "capture", account); holds != 1 || captures != 1 {
		t.Fatalf("ledger holds=%d captures=%d, want 1/1 (first only)", holds, captures)
	}
	wallet, err := env.credits.GetWallet(ctx, account)
	if err != nil {
		t.Fatalf("read vision wallet: %v", err)
	}
	if wallet.Available != 1000-25 || wallet.Reserved != 0 {
		t.Fatalf("wallet available=%d reserved=%d, want 975/0", wallet.Available, wallet.Reserved)
	}
}

// Keep the quote's authoritative reservation timestamp distinct from both the
// transaction-start timestamp and the wall clock at either cap check.
type visionReservationTimeCatalog struct {
	*billingpostgres.Store
	at time.Time
}

func (s visionReservationTimeCatalog) QuoteAtResolutionInTx(ctx context.Context, tx pgx.Tx, sku string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	quote, err := s.Store.QuoteAtResolutionInTx(ctx, tx, sku, resolution, quantity)
	quote.QuotedAt = s.at
	return quote, err
}

type visionReservationTimeChecks struct {
	budgetAt time.Time
	quotaAt  time.Time
}

func (c *visionReservationTimeChecks) CheckBudgetByInternalIDInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, reservedAt time.Time) error {
	c.budgetAt = reservedAt
	return nil
}

func (c *visionReservationTimeChecks) CheckQuotaInTx(_ context.Context, _ pgx.Tx, _, _ int64, _ int64, reservedAt time.Time) error {
	c.quotaAt = reservedAt
	return nil
}

func TestVisionPersistsTheReservationPeriodCheckedByBothCaps(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 25)
	reservedAt := time.Date(2025, 1, 31, 23, 59, 59, 999000000, time.UTC)
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{visionTestDefinition()})
	if err != nil {
		t.Fatal(err)
	}
	env.catalog = billingapplication.NewBillableItemCatalog(registry, visionReservationTimeCatalog{Store: billingpostgres.NewStore(env.pool), at: reservedAt})
	user := env.createUser(t, "reservation-period")
	const workspace = "ws_vision_period"
	env.createPersonalWorkspace(t, workspace, user, nil)
	env.fundAccount(t, env.billingAccountFor(t, workspace), 1000)
	checks := &visionReservationTimeChecks{}
	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "dunes at dusk"}})
	service.SetBudgetChecker(checks)
	service.SetQuotaChecker(checks)
	result, err := service.Extract(context.Background(), workspace, user, "reservation-period", visionTestInput())
	if err != nil {
		t.Fatal(err)
	}
	var persisted time.Time
	if err := env.pool.QueryRow(context.Background(), `SELECT created_at FROM ai_search_operations WHERE operation_id=$1::uuid`, result.OperationID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if !checks.budgetAt.Equal(reservedAt) || !checks.quotaAt.Equal(reservedAt) || !persisted.Equal(reservedAt) {
		t.Fatalf("reservation=%s budget=%s quota=%s persisted=%s", reservedAt, checks.budgetAt, checks.quotaAt, persisted)
	}
}
