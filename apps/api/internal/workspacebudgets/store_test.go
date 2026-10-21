// Workspace shared budget focused tests (#945).
//
// Usage is derived from immutable settlement facts, never stored: task
// snapshots/settlements attributed by submit-time quoted_at plus vision
// operations by created_at (both UTC), settled captures net of linked
// refunds (credit_charges.refunded_amount, so a cross-period refund
// corrects its original period and never grants current quota), plus
// pending Holds. The settlement-transaction check locks the budget row,
// so concurrent submits serialize and the loser observes the winner's
// committed Hold.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

package workspacebudgets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/consumptionread"
	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type budgetEnv struct {
	pool    *pgxpool.Pool
	store   *Store
	credits *credits.Service
	ws      string
	wsID    int64
	account int64
	userA   int64
	userB   int64
	priceID int64
	version int64
}

var budgetTestSeq int

func openBudgetEnv(t *testing.T) *budgetEnv {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	budgetTestSeq++

	uidA := fmt.Sprintf("G%04dA", budgetTestSeq)
	uidB := fmt.Sprintf("G%04dB", budgetTestSeq)
	var userA, userB int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('budget-user-a', $1, 'user', 1) RETURNING id`, uidA).Scan(&userA); err != nil {
		t.Fatalf("seed budget user a: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('budget-user-b', $1, 'user', 1) RETURNING id`, uidB).Scan(&userB); err != nil {
		t.Fatalf("seed budget user b: %v", err)
	}
	ws := fmt.Sprintf("ws_budget%04d", budgetTestSeq)
	var wsID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'budget-ws') RETURNING internal_id`, ws, userA).Scan(&wsID); err != nil {
		t.Fatalf("seed budget workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'member')`, wsID, userA, userB); err != nil {
		t.Fatalf("seed budget memberships: %v", err)
	}
	// Per-workspace billing account; the insert trigger provisions the
	// wallet, so funds arrive only through Grants below.
	var account int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, $2) RETURNING internal_id`, fmt.Sprintf("ba_budget%04d", budgetTestSeq), wsID).Scan(&account); err != nil {
		t.Fatalf("seed budget billing account: %v", err)
	}
	creditService := credits.NewService(pool)
	grant := func(amount int64, key string) {
		t.Helper()
		creditAmount, err := creditsdomain.NewCreditAmount(amount)
		if err != nil {
			t.Fatalf("budget grant amount: %v", err)
		}
		if _, err := creditService.Grant(ctx, creditsports.GrantCommand{BillingAccountID: account, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorSystem, ID: "test"},
			Source:         "test", Reason: "budget seed",
			BusinessReference: creditsdomain.BusinessReference{Type: "test", ID: key},
		}}); err != nil {
			t.Fatalf("grant budget funds: %v", err)
		}
	}
	grant(100000, fmt.Sprintf("budget-seed-%d", budgetTestSeq))

	// Minimal catalog rows so task snapshots satisfy their price FKs.
	if _, err := pool.Exec(ctx, `INSERT INTO billing_skus (sku, display_name, description, enabled, created_by, updated_by) VALUES ('ai-image:render', 'Render', '', TRUE, 1, 1)`); err != nil {
		t.Fatalf("seed budget sku: %v", err)
	}
	var priceID, version int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by) VALUES ('ai-image:render', '1k', 1, 10, 1) RETURNING price_version_id, version`).Scan(&priceID, &version); err != nil {
		t.Fatalf("seed budget price: %v", err)
	}
	return &budgetEnv{pool: pool, store: NewStore(pool), credits: creditService, ws: ws, wsID: wsID, account: account, userA: userA, userB: userB, priceID: priceID, version: version}
}

func (e *budgetEnv) hold(t *testing.T, tx pgx.Tx, taskID string, userID, amount int64, key string) string {
	t.Helper()
	ctx := context.Background()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("budget hold amount: %v", err)
	}
	var res creditsports.CommandResult
	if tx != nil {
		res, err = e.credits.InTx(tx).Hold(ctx, creditsports.HoldCommand{BillingAccountID: e.account, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "test", Reason: "budget hold",
			BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: taskID},
		}})
	} else {
		res, err = e.credits.Hold(ctx, creditsports.HoldCommand{BillingAccountID: e.account, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "test", Reason: "budget hold",
			BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: taskID},
		}})
	}
	if err != nil {
		t.Fatalf("budget hold: %v", err)
	}
	return res.TransactionID
}

func (e *budgetEnv) capture(t *testing.T, holdTxID string, userID, amount int64, key, refID string) string {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("budget capture amount: %v", err)
	}
	res, err := e.credits.Capture(context.Background(), creditsports.CaptureCommand{BillingAccountID: e.account, HoldTransactionID: holdTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
		Source:         "test", Reason: "budget capture",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}})
	if err != nil {
		t.Fatalf("budget capture: %v", err)
	}
	return res.TransactionID
}

func (e *budgetEnv) release(t *testing.T, holdTxID string, userID, amount int64, key, refID string) string {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("budget release amount: %v", err)
	}
	res, err := e.credits.Release(context.Background(), creditsports.ReleaseCommand{BillingAccountID: e.account, HoldTransactionID: holdTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
		Source:         "test", Reason: "budget release",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}})
	if err != nil {
		t.Fatalf("budget release: %v", err)
	}
	return res.TransactionID
}

func (e *budgetEnv) refund(t *testing.T, captureTxID string, amount int64, key, refID string) {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("budget refund amount: %v", err)
	}
	if _, err := e.credits.Refund(context.Background(), creditsports.RefundCommand{BillingAccountID: e.account, ChargeTransactionID: captureTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorAdmin, ID: "7"},
		Source:         "test", Reason: "budget linked refund",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}}); err != nil {
		t.Fatalf("budget refund: %v", err)
	}
}

// insertTaskSnapshot records one task billing fact pair. total is the
// quoted cap (unit 10 x quantity total/10, so total must be a positive
// multiple of 10); total 0 records a free task. quotedAt sets the
// submit-time period attribution. holdTxID is "" for free tasks.
func (e *budgetEnv) insertTaskSnapshot(t *testing.T, q consumptionread.Queryer, taskID string, userID int64, quotedAt time.Time, holdTxID string, total int64) {
	t.Helper()
	ctx := context.Background()
	quantity := int64(1)
	if total == 0 {
		if holdTxID != "" {
			t.Fatalf("free task %s must not carry a hold", taskID)
		}
	} else {
		if total%10 != 0 {
			t.Fatalf("task %s total %d must be a multiple of the 10-unit price", taskID, total)
		}
		quantity = total / 10
	}
	var hold any
	if holdTxID == "" {
		hold = nil
	} else {
		hold = holdTxID
	}
	exec := func(query string, args ...any) {
		t.Helper()
		switch v := q.(type) {
		case *pgxpool.Pool:
			if _, err := v.Exec(ctx, query, args...); err != nil {
				t.Fatalf("budget snapshot exec: %v", err)
			}
		case pgx.Tx:
			if _, err := v.Exec(ctx, query, args...); err != nil {
				t.Fatalf("budget snapshot exec: %v", err)
			}
		default:
			t.Fatalf("budget snapshot querier unsupported")
		}
	}
	exec(`INSERT INTO task_billing_snapshots (task_id, user_id, billing_account_id, workspace_id, service, sku, display_name, price_version_id, price_version, output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id)
VALUES ($1,$2,$3,$4,'render','ai-image:render','Render',$5,$6,'1k',10,$7,$8,$9,$10)`,
		taskID, userID, e.account, e.wsID, e.priceID, e.version, quantity, total, quotedAt, hold)
	state := "held"
	var settled any
	if total == 0 {
		state = "free"
		settled = time.Now().UTC()
	}
	exec(`INSERT INTO task_billing_settlements (task_id, state, settled_at) VALUES ($1,$2,$3)`, taskID, state, settled)
}

func (e *budgetEnv) settleTaskCaptured(t *testing.T, holdTxID, captureTxID, releaseTxID, taskID string, outputs, captured, released int64) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE task_billing_settlements SET state='captured', actual_output_count=$2, captured_credits=$3, released_credits=$4, capture_transaction_id=$5, release_transaction_id=$6, settled_at=now() WHERE task_id=$1`,
		taskID, outputs, captured, released, nullIfEmpty(captureTxID), nullIfEmpty(releaseTxID)); err != nil {
		t.Fatalf("settle task captured: %v", err)
	}
}

func (e *budgetEnv) settleTaskReleased(t *testing.T, releaseTxID, taskID string, released int64) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE task_billing_settlements SET state='released', actual_output_count=0, captured_credits=0, released_credits=$2, release_transaction_id=$3, settled_at=now() WHERE task_id=$1`,
		taskID, released, nullIfEmpty(releaseTxID)); err != nil {
		t.Fatalf("settle task released: %v", err)
	}
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// insertVisionOp records one vision operation. holdTxID is "" only for
// terminal rows in tests that bypass the ledger (usage math only).
func (e *budgetEnv) insertVisionOp(t *testing.T, key string, userID int64, unit int64, createdAt time.Time, status string, holdTxID string) {
	t.Helper()
	var settled any
	if status == "succeeded" || status == "failed" {
		settled = time.Now().UTC()
	}
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO ai_search_operations (operation_id, user_id, workspace_id, billing_account_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id, status, created_at, settled_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'ai-search:vision', 1, $6, $7, $8, $9, $10)`,
		userID, e.wsID, e.account, key, strings.Repeat("a", 64), unit, holdTxID, status, createdAt, settled); err != nil {
		t.Fatalf("insert vision op: %v", err)
	}
}

func (e *budgetEnv) setMonthly(t *testing.T, limit int64) {
	t.Helper()
	v := limit
	if _, err := e.store.SetBudget(context.Background(), e.wsID, PeriodMonthly, &v, e.userA); err != nil {
		t.Fatalf("set monthly budget: %v", err)
	}
}

func (e *budgetEnv) usageNow(t *testing.T) int64 {
	t.Helper()
	start, end := CurrentMonthBounds(time.Now().UTC())
	used, err := e.store.UsageForPeriod(context.Background(), e.wsID, &start, &end)
	if err != nil {
		t.Fatalf("usage now: %v", err)
	}
	return used
}

func (e *budgetEnv) checkTx(t *testing.T, amount int64) error {
	t.Helper()
	tx, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return e.store.CheckBudgetInTx(context.Background(), tx, e.ws, amount, time.Now().UTC())
}

func TestBudgetUnlimitedDefault(t *testing.T) {
	e := openBudgetEnv(t)
	period, limit, found, err := e.store.GetBudgetRow(context.Background(), e.wsID)
	if err != nil {
		t.Fatalf("GetBudgetRow: %v", err)
	}
	if found || period != PeriodUnlimited || limit != nil {
		t.Fatalf("default budget = (%q, %v, %v), want unlimited absent", period, limit, found)
	}
	if err := e.checkTx(t, 1_000_000_000); err != nil {
		t.Fatalf("unlimited check: %v", err)
	}
	if got := e.usageNow(t); got != 0 {
		t.Fatalf("empty usage = %d, want 0", got)
	}
}

func TestBudgetCrossMemberSum(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	// Two members hold 30 + 40 in the same workspace and period.
	holdA := e.hold(t, nil, "task_budgetA0001", e.userA, 30, "budget-hold-a1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetA0001", e.userA, now, holdA, 30)
	holdB := e.hold(t, nil, "task_budgetB0001", e.userB, 40, "budget-hold-b1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetB0001", e.userB, now, holdB, 40)
	if got := e.usageNow(t); got != 70 {
		t.Fatalf("cross-member usage = %d, want 70", got)
	}
	// 70 + 40 exceeds the 100 cap across members; 70 + 30 fits.
	if err := e.checkTx(t, 40); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("check over cap error = %v, want ErrBudgetExhausted", err)
	}
	if err := e.checkTx(t, 30); err != nil {
		t.Fatalf("check within cap: %v", err)
	}
}

func TestBudgetTaskVisionMix(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	holdA := e.hold(t, nil, "task_budgetM0001", e.userA, 30, "budget-hold-m1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetM0001", e.userA, now, holdA, 30)
	// Vision pending hold (real ledger hold, operation row pending).
	visionHold, err := e.credits.Hold(context.Background(), creditsports.HoldCommand{BillingAccountID: e.account, Amount: mustAmount(t, 20), Metadata: visionHoldMetadata(t, e.userB, "budget-vision-h1", "vision-op-mix-1")})
	if err != nil {
		t.Fatalf("vision hold: %v", err)
	}
	e.insertVisionOp(t, "budget-vision-m1", e.userB, 20, now, "pending", visionHold.TransactionID)
	if got := e.usageNow(t); got != 50 {
		t.Fatalf("mixed usage = %d, want 50", got)
	}
	if err := e.checkTx(t, 60); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("mixed check over cap error = %v, want ErrBudgetExhausted", err)
	}
	if err := e.checkTx(t, 50); err != nil {
		t.Fatalf("mixed check within cap: %v", err)
	}
}

func TestBudgetSettleConvergesToActual(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, "task_budgetS0001", e.userA, 30, "budget-hold-s1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetS0001", e.userA, now, holdTx, 30)
	if got := e.usageNow(t); got != 30 {
		t.Fatalf("held usage = %d, want 30", got)
	}
	// Success settles the actual 20 and releases the 10 remainder: usage
	// converges 30 -> 20 (not 0, not 30).
	captureTx := e.capture(t, holdTx, e.userA, 20, "budget-capture-s1", "task_budgetS0001")
	releaseTx := e.release(t, holdTx, e.userA, 10, "budget-release-s1", "task_budgetS0001")
	e.settleTaskCaptured(t, holdTx, captureTx, releaseTx, "task_budgetS0001", 2, 20, 10)
	if got := e.usageNow(t); got != 20 {
		t.Fatalf("captured usage = %d, want 20", got)
	}
}

func TestBudgetFailReleases(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, "task_budgetF0001", e.userA, 30, "budget-hold-f1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetF0001", e.userA, now, holdTx, 30)
	releaseTx := e.release(t, holdTx, e.userA, 30, "budget-release-f1", "task_budgetF0001")
	e.settleTaskReleased(t, releaseTx, "task_budgetF0001", 30)
	if got := e.usageNow(t); got != 0 {
		t.Fatalf("released usage = %d, want 0", got)
	}
	if err := e.checkTx(t, 100); err != nil {
		t.Fatalf("check after release: %v", err)
	}
}

func TestBudgetRefundCorrectsOriginalPeriod(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	priorStart, _ := CurrentMonthBounds(now)
	priorQuoted := priorStart.AddDate(0, -1, 0).Add(2 * time.Hour)
	// Prior-period capture of 50 settles in its own period.
	holdTx := e.hold(t, nil, "task_budgetR0001", e.userA, 30, "budget-hold-r1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetR0001", e.userA, priorQuoted, holdTx, 30)
	captureTx := e.capture(t, holdTx, e.userA, 30, "budget-capture-r1", "task_budgetR0001")
	e.settleTaskCaptured(t, holdTx, captureTx, "", "task_budgetR0001", 3, 30, 0)
	priorUsed := func() int64 {
		t.Helper()
		s, en := priorStart.AddDate(0, -1, 0), priorStart
		used, err := e.store.UsageForPeriod(context.Background(), e.wsID, &s, &en)
		if err != nil {
			t.Fatalf("prior usage: %v", err)
		}
		return used
	}
	if got := priorUsed(); got != 30 {
		t.Fatalf("prior usage = %d, want 30", got)
	}
	if got := e.usageNow(t); got != 0 {
		t.Fatalf("current usage = %d, want 0", got)
	}
	// Linked consumption refund corrects the ORIGINAL period; the current
	// period gains no quota.
	e.refund(t, captureTx, 30, "budget-refund-r1", "task_budgetR0001")
	if got := priorUsed(); got != 0 {
		t.Fatalf("prior usage after refund = %d, want 0", got)
	}
	if got := e.usageNow(t); got != 0 {
		t.Fatalf("current usage after cross-period refund = %d, want 0", got)
	}
}

func TestBudgetPeriodResetKeepsHistory(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	periodStart, _ := CurrentMonthBounds(now)
	priorQuoted := periodStart.AddDate(0, -1, 0).Add(2 * time.Hour)
	holdOld := e.hold(t, nil, "task_budgetP0001", e.userA, 30, "budget-hold-p1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetP0001", e.userA, priorQuoted, holdOld, 30)
	captureOld := e.capture(t, holdOld, e.userA, 30, "budget-capture-p1", "task_budgetP0001")
	e.settleTaskCaptured(t, holdOld, captureOld, "", "task_budgetP0001", 3, 30, 0)
	holdNew := e.hold(t, nil, "task_budgetP0002", e.userA, 30, "budget-hold-p2")
	e.insertTaskSnapshot(t, e.pool, "task_budgetP0002", e.userA, now, holdNew, 30)
	// Current period counts only the new hold; history rows survive.
	if got := e.usageNow(t); got != 30 {
		t.Fatalf("current usage = %d, want 30", got)
	}
	var snapshotCount int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM task_billing_snapshots WHERE workspace_id = $1`, e.wsID).Scan(&snapshotCount); err != nil || snapshotCount != 2 {
		t.Fatalf("history snapshots = %d/err %v, want 2 retained", snapshotCount, err)
	}
	if err := e.checkTx(t, 80); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("check over reset cap error = %v, want ErrBudgetExhausted", err)
	}
	if err := e.checkTx(t, 70); err != nil {
		t.Fatalf("check within reset cap: %v", err)
	}
}

func TestBudgetAdjustKeepsUsage(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, "task_budgetJ0001", e.userA, 30, "budget-hold-j1")
	e.insertTaskSnapshot(t, e.pool, "task_budgetJ0001", e.userA, now, holdTx, 30)
	holdB := e.hold(t, nil, "task_budgetJ0002", e.userB, 30, "budget-hold-j2")
	e.insertTaskSnapshot(t, e.pool, "task_budgetJ0002", e.userB, now, holdB, 30)
	// Lowering below usage blocks everything; usage itself is untouched.
	lower := int64(50)
	if _, err := e.store.SetBudget(context.Background(), e.wsID, PeriodMonthly, &lower, e.userA); err != nil {
		t.Fatalf("lower budget: %v", err)
	}
	if got := e.usageNow(t); got != 60 {
		t.Fatalf("usage after lower = %d, want 60 retained", got)
	}
	if err := e.checkTx(t, 1); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("check after lower error = %v, want ErrBudgetExhausted", err)
	}
	// Raising re-opens headroom against the same retained usage.
	raise := int64(200)
	if _, err := e.store.SetBudget(context.Background(), e.wsID, PeriodMonthly, &raise, e.userA); err != nil {
		t.Fatalf("raise budget: %v", err)
	}
	if got := e.usageNow(t); got != 60 {
		t.Fatalf("usage after raise = %d, want 60 retained", got)
	}
	if err := e.checkTx(t, 100); err != nil {
		t.Fatalf("check after raise: %v", err)
	}
	// Switching to unlimited clears the gate but keeps the facts.
	if _, err := e.store.SetBudget(context.Background(), e.wsID, PeriodUnlimited, nil, e.userA); err != nil {
		t.Fatalf("unlimit budget: %v", err)
	}
	if err := e.checkTx(t, 1_000_000); err != nil {
		t.Fatalf("check after unlimit: %v", err)
	}
}

func TestBudgetConcurrentSubmitsFenced(t *testing.T) {
	e := openBudgetEnv(t)
	e.setMonthly(t, 100)
	now := time.Now().UTC()

	tx1, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(context.Background())
	if err := e.store.CheckBudgetInTx(context.Background(), tx1, e.ws, 60, time.Now().UTC()); err != nil {
		t.Fatalf("first check: %v", err)
	}
	holdTx := e.hold(t, tx1, "task_budgetC0001", e.userA, 60, "budget-hold-c1")
	e.insertTaskSnapshot(t, tx1, "task_budgetC0001", e.userA, now, holdTx, 60)

	// The loser starts while the winner is still uncommitted: its budget
	// row lock waits, then it observes the winner's committed Hold and
	// fails closed instead of double-spending the cap.
	type checkOutcome struct {
		err error
	}
	loser := make(chan checkOutcome, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx2, err := e.pool.Begin(context.Background())
		if err != nil {
			loser <- checkOutcome{err: err}
			return
		}
		defer tx2.Rollback(context.Background())
		loser <- checkOutcome{err: e.store.CheckBudgetInTx(context.Background(), tx2, e.ws, 60, time.Now().UTC())}
	}()
	// Let the loser block on the row lock, then commit the winner.
	time.Sleep(300 * time.Millisecond)
	if err := tx1.Commit(context.Background()); err != nil {
		t.Fatalf("commit winner: %v", err)
	}
	wg.Wait()
	out := <-loser
	if !errors.Is(out.err, ErrBudgetExhausted) {
		t.Fatalf("loser check error = %v, want ErrBudgetExhausted", out.err)
	}
	if got := e.usageNow(t); got != 60 {
		t.Fatalf("fenced usage = %d, want exactly the winner's 60", got)
	}
}

func mustAmount(t *testing.T, v int64) creditsdomain.CreditAmount {
	t.Helper()
	amount, err := creditsdomain.NewCreditAmount(v)
	if err != nil {
		t.Fatalf("NewCreditAmount(%d): %v", v, err)
	}
	return amount
}

func visionHoldMetadata(t *testing.T, userID int64, key, refID string) creditsdomain.CommandMetadata {
	t.Helper()
	return creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
		Source:         "ai_search_vision", Reason: "vision search hold",
		BusinessReference: creditsdomain.BusinessReference{Type: "ai_search_operation", ID: refID},
	}
}

// captureAfterUsageScan commits a real independent terminal transaction after
// the first usage read. Split captured/held reads lose the same consumption;
// one statement must retain either its Hold or its settled actual.
type captureAfterUsageScan struct {
	pgx.Tx
	after func()
}

func (q *captureAfterUsageScan) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := q.Tx.QueryRow(ctx, sql, args...)
	if q.after == nil || !strings.Contains(sql, "task_billing_") {
		return row
	}
	after := q.after
	q.after = nil
	return captureAfterScanRow{Row: row, after: after}
}

type captureAfterScanRow struct {
	pgx.Row
	after func()
}

func (r captureAfterScanRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	r.after()
	return nil
}

func TestConcurrentCaptureCannotHideUsage(t *testing.T) {
	e := openBudgetEnv(t)
	ctx := context.Background()
	limit := int64(100)
	if _, err := e.store.SetBudget(ctx, e.wsID, PeriodMonthly, &limit, e.userA); err != nil {
		t.Fatal(err)
	}
	reservedAt := time.Now().UTC()
	const taskID = "task_capture_race"
	holdID := e.hold(t, nil, taskID, e.userA, 100, "capture-race-hold")
	e.insertTaskSnapshot(t, e.pool, taskID, e.userA, reservedAt, holdID, 100)
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	captured := false
	wrapped := &captureAfterUsageScan{Tx: tx, after: func() {
		terminal, err := e.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer terminal.Rollback(ctx)
		capture, err := e.credits.InTx(terminal).Capture(ctx, creditsports.CaptureCommand{
			BillingAccountID: e.account, HoldTransactionID: holdID, Amount: mustAmount(t, 100),
			Metadata: creditsdomain.CommandMetadata{
				IdempotencyKey: "capture-race-capture",
				Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprint(e.userA)},
				Source:         "test", Reason: "capture during cap check",
				BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: taskID},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := terminal.Exec(ctx, `UPDATE task_billing_settlements SET state='captured', actual_output_count=10, captured_credits=100, capture_transaction_id=$2, settled_at=now() WHERE task_id=$1`, taskID, capture.TransactionID); err != nil {
			t.Fatal(err)
		}
		if err := terminal.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		captured = true
	}}
	if err := e.store.CheckBudgetByInternalIDInTx(ctx, wrapped, e.wsID, 100, reservedAt); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("existing 100/100 usage plus 100: got %v, want ErrBudgetExhausted", err)
	}
	if !captured {
		t.Fatal("terminal Capture did not interleave with usage read")
	}
}

// Historical boundaries make any accidental wall-clock recomputation observable
// without sleeping until midnight. A real row-lock wait must preserve the
// reservation's old full window, while a reservation in the next window fits.
func TestReservationPeriodSurvivesRowLockWait(t *testing.T) {
	for _, tc := range []struct {
		name     string
		period   string
		boundary time.Time
	}{
		{"monthly", PeriodMonthly, time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := openBudgetEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			limit := int64(100)
			if _, err := e.store.SetBudget(ctx, e.wsID, tc.period, &limit, e.userA); err != nil {
				t.Fatal(err)
			}
			before := tc.boundary.Add(-time.Millisecond)
			const taskID = "task_period_boundary"
			holdID := e.hold(t, nil, taskID, e.userA, 100, "period-hold")
			e.insertTaskSnapshot(t, e.pool, taskID, e.userA, before, holdID, 100)
			if err := e.store.CheckBudgetForQuote(ctx, e.wsID, 10, before); !errors.Is(err, ErrBudgetExhausted) {
				t.Fatalf("quote in original full period: %v", err)
			}
			if err := e.store.CheckBudgetForQuote(ctx, e.wsID, 10, tc.boundary); err != nil {
				t.Fatalf("quote in new empty period: %v", err)
			}
			locker, err := e.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Rollback(context.Background())
			if _, err := locker.Exec(ctx, `SELECT workspace_id FROM workspace_budgets WHERE workspace_id=$1 FOR UPDATE`, e.wsID); err != nil {
				t.Fatal(err)
			}
			waiter, err := e.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer waiter.Rollback(context.Background())
			waiterPID := waiter.Conn().PgConn().PID()
			done := make(chan error, 1)
			go func() { done <- e.store.CheckBudgetByInternalIDInTx(ctx, waiter, e.wsID, 10, before) }()
			// Verify the checker really blocks on the independent row lock.
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked bool
				if err := e.pool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1)) > 0`, waiterPID).Scan(&blocked); err != nil {
					_ = locker.Rollback(context.Background())
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case <-ctx.Done():
					_ = locker.Rollback(context.Background())
					t.Fatal("checker did not wait for cap row lock")
				case <-ticker.C:
				}
			}
			if err := locker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, ErrBudgetExhausted) {
				t.Fatalf("blocked reservation must use original full period: %v", err)
			}
			if err := e.store.CheckBudgetByInternalIDInTx(ctx, waiter, e.wsID, 10, tc.boundary); err != nil {
				t.Fatalf("reservation in next empty period: %v", err)
			}
		})
	}
}
