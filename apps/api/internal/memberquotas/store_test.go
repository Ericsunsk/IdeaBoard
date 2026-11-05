// Member cross-workspace quota focused tests (#946).
//
// Usage is derived from immutable settlement facts, never stored: task
// snapshots/settlements attributed by submit time (quoted_at) plus vision
// operations by reservation time (created_at), both keyed by the real
// operator (Billing Account + User) across every Workspace sharing the
// account. Settled captures net linked refunds
// (credit_charges.refunded_amount, so a cross-period refund corrects its
// original period and never widens the current quota), plus pending
// Holds. The settlement-transaction check locks the quota row, so
// concurrent submits by one member serialize and the loser observes the
// winner's committed Hold. Quota rows and history survive membership
// leave/rejoin: rejoining never re-issues.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

package memberquotas

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

type quotaEnv struct {
	pool     *pgxpool.Pool
	store    *Store
	credits  *credits.Service
	ws       string
	wsID     int64
	ws2      string
	ws2ID    int64
	account  int64
	account2 int64
	userA    int64
	userB    int64
	priceID  int64
	version  int64
}

var quotaTestSeq int

func openQuotaEnv(t *testing.T) *quotaEnv {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	quotaTestSeq++

	uidA := fmt.Sprintf("Q%04dA", quotaTestSeq)
	uidB := fmt.Sprintf("Q%04dB", quotaTestSeq)
	var userA, userB int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('quota-user-a', $1, 'user', 1) RETURNING id`, uidA).Scan(&userA); err != nil {
		t.Fatalf("seed quota user a: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('quota-user-b', $1, 'user', 1) RETURNING id`, uidB).Scan(&userB); err != nil {
		t.Fatalf("seed quota user b: %v", err)
	}
	// Org-backed (#975: MemberQuotas is Org-only): one Organization with
	// two workspaces sharing the Organization pool account, plus an
	// independent second Organization pool for the isolation test. No
	// organization memberships are seeded (both users read as non-owners,
	// so the normal cap path is exercised).
	var orgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (public_id, name) VALUES ($1, 'quota-org') RETURNING internal_id`, fmt.Sprintf("org_quota%04d", quotaTestSeq)).Scan(&orgID); err != nil {
		t.Fatalf("seed quota organization: %v", err)
	}
	ws := fmt.Sprintf("ws_quota%04d", quotaTestSeq)
	var wsID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (public_id, organization_id, owner_user_id, name) VALUES ($1, $2, NULL, 'quota-ws') RETURNING internal_id`, ws, orgID).Scan(&wsID); err != nil {
		t.Fatalf("seed quota workspace: %v", err)
	}
	// Second workspace sharing the same Organization pool: the member's
	// consumption in both spaces accumulates into one quota.
	ws2 := fmt.Sprintf("ws_quota2%04d", quotaTestSeq)
	var ws2ID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (public_id, organization_id, owner_user_id, name) VALUES ($1, $2, NULL, 'quota-ws-2') RETURNING internal_id`, ws2, orgID).Scan(&ws2ID); err != nil {
		t.Fatalf("seed quota workspace 2: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'member'), ($4, $2, 'owner'), ($4, $3, 'member')`, wsID, userA, userB, ws2ID); err != nil {
		t.Fatalf("seed quota memberships: %v", err)
	}
	// Organization pool account; the insert trigger provisions the
	// wallet, so funds arrive only through Grants below.
	var account int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id, organization_id) VALUES ($1, NULL, $2) RETURNING internal_id`, fmt.Sprintf("ba_quota%04d", quotaTestSeq), orgID).Scan(&account); err != nil {
		t.Fatalf("seed quota billing account: %v", err)
	}
	// Independent second Organization pool for the isolation test.
	var org2ID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (public_id, name) VALUES ($1, 'quota-org-2') RETURNING internal_id`, fmt.Sprintf("org_quotaB%04d", quotaTestSeq)).Scan(&org2ID); err != nil {
		t.Fatalf("seed quota organization 2: %v", err)
	}
	var account2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id, organization_id) VALUES ($1, NULL, $2) RETURNING internal_id`, fmt.Sprintf("ba_quotaB%04d", quotaTestSeq), org2ID).Scan(&account2); err != nil {
		t.Fatalf("seed quota billing account 2: %v", err)
	}
	creditService := credits.NewService(pool)
	grant := func(accountID, amount int64, key string) {
		t.Helper()
		creditAmount, err := creditsdomain.NewCreditAmount(amount)
		if err != nil {
			t.Fatalf("quota grant amount: %v", err)
		}
		if _, err := creditService.Administrative().Grant(ctx, creditsports.GrantCommand{BillingAccountID: accountID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorSystem, ID: "test"},
			Source:         "test", Reason: "quota seed",
			BusinessReference: creditsdomain.BusinessReference{Type: "test", ID: key},
		}}); err != nil {
			t.Fatalf("grant quota funds: %v", err)
		}
	}
	grant(account, 100000, fmt.Sprintf("quota-seed-%d", quotaTestSeq))
	grant(account2, 100000, fmt.Sprintf("quota-seed2-%d", quotaTestSeq))

	// Minimal catalog rows so task snapshots satisfy their price FKs.
	if _, err := pool.Exec(ctx, `INSERT INTO billing_skus (sku, display_name, description, enabled, created_by, updated_by) VALUES ('ai-image:render', 'Render', '', TRUE, 1, 1)`); err != nil {
		t.Fatalf("seed quota sku: %v", err)
	}
	var priceID, version int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by) VALUES ('ai-image:render', '1k', 1, 10, 1) RETURNING price_version_id, version`).Scan(&priceID, &version); err != nil {
		t.Fatalf("seed quota price: %v", err)
	}
	return &quotaEnv{pool: pool, store: NewStore(pool), credits: creditService, ws: ws, wsID: wsID, ws2: ws2, ws2ID: ws2ID, account: account, account2: account2, userA: userA, userB: userB, priceID: priceID, version: version}
}

func (e *quotaEnv) hold(t *testing.T, tx pgx.Tx, accountID int64, taskID string, userID, amount int64, key string) string {
	t.Helper()
	ctx := context.Background()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("quota hold amount: %v", err)
	}
	var res creditsports.CommandResult
	if tx != nil {
		res, err = e.credits.SettlementInTx(tx).Hold(ctx, creditsports.HoldCommand{BillingAccountID: accountID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "test", Reason: "quota hold",
			BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: taskID},
		}})
	} else {
		res, err = e.credits.Settlement().Hold(ctx, creditsports.HoldCommand{BillingAccountID: accountID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "test", Reason: "quota hold",
			BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: taskID},
		}})
	}
	if err != nil {
		t.Fatalf("quota hold: %v", err)
	}
	return res.TransactionID
}

func (e *quotaEnv) capture(t *testing.T, holdTxID string, userID, amount int64, key, refID string) string {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("quota capture amount: %v", err)
	}
	res, err := e.credits.Settlement().Capture(context.Background(), creditsports.CaptureCommand{BillingAccountID: e.account, HoldTransactionID: holdTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
		Source:         "test", Reason: "quota capture",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}})
	if err != nil {
		t.Fatalf("quota capture: %v", err)
	}
	return res.TransactionID
}

func (e *quotaEnv) release(t *testing.T, holdTxID string, userID, amount int64, key, refID string) string {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("quota release amount: %v", err)
	}
	res, err := e.credits.Settlement().Release(context.Background(), creditsports.ReleaseCommand{BillingAccountID: e.account, HoldTransactionID: holdTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", userID)},
		Source:         "test", Reason: "quota release",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}})
	if err != nil {
		t.Fatalf("quota release: %v", err)
	}
	return res.TransactionID
}

func (e *quotaEnv) refund(t *testing.T, captureTxID string, amount int64, key, refID string) {
	t.Helper()
	creditAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatalf("quota refund amount: %v", err)
	}
	if _, err := e.credits.Administrative().Refund(context.Background(), creditsports.RefundCommand{BillingAccountID: e.account, ChargeTransactionID: captureTxID, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorAdmin, ID: "7"},
		Source:         "test", Reason: "quota linked refund",
		BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: refID},
	}}); err != nil {
		t.Fatalf("quota refund: %v", err)
	}
}

// insertTaskSnapshot records one task billing fact pair on one account.
// total is the quoted cap (unit 10 x quantity); quotedAt sets the
// submit-time period attribution. holdTxID is "" for free tasks.
func (e *quotaEnv) insertTaskSnapshot(t *testing.T, q consumptionread.Queryer, accountID, wsID int64, taskID string, userID int64, quotedAt time.Time, holdTxID string, total int64) {
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
				t.Fatalf("quota snapshot exec: %v", err)
			}
		case pgx.Tx:
			if _, err := v.Exec(ctx, query, args...); err != nil {
				t.Fatalf("quota snapshot exec: %v", err)
			}
		default:
			t.Fatalf("quota snapshot querier unsupported")
		}
	}
	exec(`INSERT INTO task_billing_snapshots (task_id, user_id, billing_account_id, workspace_id, service, sku, display_name, price_version_id, price_version, output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id)
VALUES ($1,$2,$3,$4,'render','ai-image:render','Render',$5,$6,'1k',10,$7,$8,$9,$10)`,
		taskID, userID, accountID, wsID, e.priceID, e.version, quantity, total, quotedAt, hold)
	state := "held"
	var settled any
	if total == 0 {
		state = "free"
		settled = time.Now().UTC()
	}
	exec(`INSERT INTO task_billing_settlements (task_id, state, settled_at) VALUES ($1,$2,$3)`, taskID, state, settled)
}

func (e *quotaEnv) settleTaskCaptured(t *testing.T, captureTxID, releaseTxID, taskID string, outputs, captured, released int64) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE task_billing_settlements SET state='captured', actual_output_count=$2, captured_credits=$3, released_credits=$4, capture_transaction_id=$5, release_transaction_id=$6, settled_at=now() WHERE task_id=$1`,
		taskID, outputs, captured, released, nullIfEmptyQuota(captureTxID), nullIfEmptyQuota(releaseTxID)); err != nil {
		t.Fatalf("settle task captured: %v", err)
	}
}

func (e *quotaEnv) settleTaskReleased(t *testing.T, releaseTxID, taskID string, released int64) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE task_billing_settlements SET state='released', actual_output_count=0, captured_credits=0, released_credits=$2, release_transaction_id=$3, settled_at=now() WHERE task_id=$1`,
		taskID, released, nullIfEmptyQuota(releaseTxID)); err != nil {
		t.Fatalf("settle task released: %v", err)
	}
}

func nullIfEmptyQuota(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// insertVisionOp records one vision operation on one account. holdTxID is
// "" only for terminal rows in tests that bypass the ledger (usage math
// only).
func (e *quotaEnv) insertVisionOp(t *testing.T, accountID, wsID int64, key string, userID int64, unit int64, createdAt time.Time, status string, holdTxID string) {
	t.Helper()
	var settled any
	if status == "succeeded" || status == "failed" {
		settled = time.Now().UTC()
	}
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO ai_search_operations (operation_id, user_id, workspace_id, billing_account_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id, status, created_at, settled_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'ai-search:vision', 1, $6, $7, $8, $9, $10)`,
		userID, wsID, accountID, key, strings.Repeat("a", 64), unit, holdTxID, status, createdAt, settled); err != nil {
		t.Fatalf("insert vision op: %v", err)
	}
}

func (e *quotaEnv) setFixed(t *testing.T, userID, limit int64) {
	t.Helper()
	v := limit
	if _, err := e.store.SetQuota(context.Background(), e.account, userID, PeriodFixed, &v, e.userA); err != nil {
		t.Fatalf("set fixed quota: %v", err)
	}
}

func (e *quotaEnv) setMonthly(t *testing.T, userID, limit int64) {
	t.Helper()
	v := limit
	if _, err := e.store.SetQuota(context.Background(), e.account, userID, PeriodMonthly, &v, e.userA); err != nil {
		t.Fatalf("set monthly quota: %v", err)
	}
}

func (e *quotaEnv) usageMonthly(t *testing.T, userID int64) int64 {
	t.Helper()
	start, end := CurrentMonthBounds(time.Now().UTC())
	used, err := e.store.UsageForPeriod(context.Background(), e.account, userID, &start, &end)
	if err != nil {
		t.Fatalf("usage monthly: %v", err)
	}
	return used
}

func (e *quotaEnv) usageFixed(t *testing.T, userID int64) int64 {
	t.Helper()
	used, err := e.store.UsageForPeriod(context.Background(), e.account, userID, nil, nil)
	if err != nil {
		t.Fatalf("usage fixed: %v", err)
	}
	return used
}

func (e *quotaEnv) checkTx(t *testing.T, userID, amount int64) error {
	t.Helper()
	tx, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return e.store.CheckQuotaInTx(context.Background(), tx, e.account, userID, amount, time.Now().UTC())
}

func TestQuotaUnlimitedDefault(t *testing.T) {
	e := openQuotaEnv(t)
	period, limit, found, err := e.store.GetQuotaRow(context.Background(), e.account, e.userA)
	if err != nil {
		t.Fatalf("GetQuotaRow: %v", err)
	}
	if found || period != PeriodUnlimited || limit != nil {
		t.Fatalf("default quota = (%q, %v, %v), want unlimited absent", period, limit, found)
	}
	if err := e.checkTx(t, e.userA, 1_000_000_000); err != nil {
		t.Fatalf("unlimited check: %v", err)
	}
	if got := e.usageMonthly(t, e.userA); got != 0 {
		t.Fatalf("empty usage = %d, want 0", got)
	}
}

func TestQuotaCrossWorkspaceSum(t *testing.T) {
	e := openQuotaEnv(t)
	e.setFixed(t, e.userA, 100)
	now := time.Now().UTC()
	// One member holds 30 in the first space and 40 in the second space
	// sharing the account: both accumulate into the one quota.
	holdA := e.hold(t, nil, e.account, "task_quotaA0001", e.userA, 30, "quota-hold-a1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaA0001", e.userA, now, holdA, 30)
	holdA2 := e.hold(t, nil, e.account, "task_quotaA0002", e.userA, 40, "quota-hold-a2")
	e.insertTaskSnapshot(t, e.pool, e.account, e.ws2ID, "task_quotaA0002", e.userA, now, holdA2, 40)
	if got := e.usageFixed(t, e.userA); got != 70 {
		t.Fatalf("cross-workspace usage = %d, want 70", got)
	}
	// The other member's usage is independent.
	holdB := e.hold(t, nil, e.account, "task_quotaB0001", e.userB, 40, "quota-hold-b1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaB0001", e.userB, now, holdB, 40)
	if got := e.usageFixed(t, e.userB); got != 40 {
		t.Fatalf("other member usage = %d, want 40", got)
	}
	// 70 + 40 exceeds userA's 100 cap; 70 + 30 fits. userB (unlimited)
	// still passes any amount.
	if err := e.checkTx(t, e.userA, 40); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("check over cap error = %v, want ErrQuotaExhausted", err)
	}
	if err := e.checkTx(t, e.userA, 30); err != nil {
		t.Fatalf("check within cap: %v", err)
	}
	if err := e.checkTx(t, e.userB, 1_000_000); err != nil {
		t.Fatalf("unlimited member check: %v", err)
	}
}

func TestQuotaCrossAccountIsolation(t *testing.T) {
	e := openQuotaEnv(t)
	e.setFixed(t, e.userA, 100)
	now := time.Now().UTC()
	// Same user consumes 60 on the independent second account: the first
	// account's quota must not observe it.
	holdTx, err := func() (string, error) {
		creditAmount, err := creditsdomain.NewCreditAmount(60)
		if err != nil {
			return "", err
		}
		res, err := e.credits.Settlement().Hold(context.Background(), creditsports.HoldCommand{BillingAccountID: e.account2, Amount: creditAmount, Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey: "quota-hold-iso1",
			Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprintf("%d", e.userA)},
			Source:         "test", Reason: "quota isolation hold",
			BusinessReference: creditsdomain.BusinessReference{Type: "task", ID: "task_quotaI0001"},
		}})
		if err != nil {
			return "", err
		}
		return res.TransactionID, nil
	}()
	if err != nil {
		t.Fatalf("isolation hold: %v", err)
	}
	e.insertTaskSnapshot(t, e.pool, e.account2, e.ws2ID, "task_quotaI0001", e.userA, now, holdTx, 60)
	if got := e.usageFixed(t, e.userA); got != 0 {
		t.Fatalf("isolated usage = %d, want 0 (other account invisible)", got)
	}
	other, err := e.store.UsageForPeriod(context.Background(), e.account2, e.userA, nil, nil)
	if err != nil {
		t.Fatalf("other account usage: %v", err)
	}
	if other != 60 {
		t.Fatalf("other account usage = %d, want 60", other)
	}
}

func TestQuotaTaskVisionMix(t *testing.T) {
	e := openQuotaEnv(t)
	e.setMonthly(t, e.userA, 100)
	now := time.Now().UTC()
	holdA := e.hold(t, nil, e.account, "task_quotaM0001", e.userA, 30, "quota-hold-m1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaM0001", e.userA, now, holdA, 30)
	// Vision pending hold in the second space (same member, same account).
	visionHold, err := e.credits.Settlement().Hold(context.Background(), creditsports.HoldCommand{BillingAccountID: e.account, Amount: mustQuotaAmount(t, 20), Metadata: visionQuotaHoldMetadata(t, e.userA, "quota-vision-h1", "vision-op-mix-1")})
	if err != nil {
		t.Fatalf("vision hold: %v", err)
	}
	e.insertVisionOp(t, e.account, e.ws2ID, "quota-vision-m1", e.userA, 20, now, "pending", visionHold.TransactionID)
	if got := e.usageMonthly(t, e.userA); got != 50 {
		t.Fatalf("mixed usage = %d, want 50", got)
	}
	if err := e.checkTx(t, e.userA, 60); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("mixed check over cap error = %v, want ErrQuotaExhausted", err)
	}
	if err := e.checkTx(t, e.userA, 50); err != nil {
		t.Fatalf("mixed check within cap: %v", err)
	}
}

func TestQuotaSettleConvergesToActual(t *testing.T) {
	e := openQuotaEnv(t)
	e.setMonthly(t, e.userA, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, e.account, "task_quotaS0001", e.userA, 30, "quota-hold-s1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaS0001", e.userA, now, holdTx, 30)
	if got := e.usageMonthly(t, e.userA); got != 30 {
		t.Fatalf("held usage = %d, want 30", got)
	}
	// Success settles the actual 20 and releases the 10 remainder: usage
	// converges 30 -> 20 (not 0, not 30).
	captureTx := e.capture(t, holdTx, e.userA, 20, "quota-capture-s1", "task_quotaS0001")
	releaseTx := e.release(t, holdTx, e.userA, 10, "quota-release-s1", "task_quotaS0001")
	e.settleTaskCaptured(t, captureTx, releaseTx, "task_quotaS0001", 2, 20, 10)
	if got := e.usageMonthly(t, e.userA); got != 20 {
		t.Fatalf("captured usage = %d, want 20", got)
	}
}

func TestQuotaFailReleases(t *testing.T) {
	e := openQuotaEnv(t)
	e.setMonthly(t, e.userA, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, e.account, "task_quotaF0001", e.userA, 30, "quota-hold-f1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaF0001", e.userA, now, holdTx, 30)
	releaseTx := e.release(t, holdTx, e.userA, 30, "quota-release-f1", "task_quotaF0001")
	e.settleTaskReleased(t, releaseTx, "task_quotaF0001", 30)
	if got := e.usageMonthly(t, e.userA); got != 0 {
		t.Fatalf("released usage = %d, want 0", got)
	}
	if err := e.checkTx(t, e.userA, 100); err != nil {
		t.Fatalf("check after release: %v", err)
	}
}

func TestQuotaRefundCorrectsOriginalPeriod(t *testing.T) {
	e := openQuotaEnv(t)
	e.setMonthly(t, e.userA, 100)
	now := time.Now().UTC()
	priorStart, _ := CurrentMonthBounds(now)
	priorQuoted := priorStart.AddDate(0, -1, 0).Add(2 * time.Hour)
	// Prior-period capture of 30 settles in its own period.
	holdTx := e.hold(t, nil, e.account, "task_quotaR0001", e.userA, 30, "quota-hold-r1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaR0001", e.userA, priorQuoted, holdTx, 30)
	captureTx := e.capture(t, holdTx, e.userA, 30, "quota-capture-r1", "task_quotaR0001")
	e.settleTaskCaptured(t, captureTx, "", "task_quotaR0001", 3, 30, 0)
	priorUsed := func() int64 {
		t.Helper()
		s, en := priorStart.AddDate(0, -1, 0), priorStart
		used, err := e.store.UsageForPeriod(context.Background(), e.account, e.userA, &s, &en)
		if err != nil {
			t.Fatalf("prior usage: %v", err)
		}
		return used
	}
	if got := priorUsed(); got != 30 {
		t.Fatalf("prior usage = %d, want 30", got)
	}
	if got := e.usageMonthly(t, e.userA); got != 0 {
		t.Fatalf("current usage = %d, want 0", got)
	}
	// Linked consumption refund corrects the ORIGINAL period; the current
	// period gains no quota (the value returns to the account balance,
	// not to this window).
	e.refund(t, captureTx, 30, "quota-refund-r1", "task_quotaR0001")
	if got := priorUsed(); got != 0 {
		t.Fatalf("prior usage after refund = %d, want 0", got)
	}
	if got := e.usageMonthly(t, e.userA); got != 0 {
		t.Fatalf("current usage after cross-period refund = %d, want 0", got)
	}
}

func TestQuotaPeriodWindowsExcludeOutsideUsage(t *testing.T) {
	e := openQuotaEnv(t)
	now := time.Now().UTC()
	// Monthly quota: a capture settled in the prior month stays out of
	// window (no rollover of unused quota, no leakage of old spend).
	e.setMonthly(t, e.userA, 100)
	priorStart, _ := CurrentMonthBounds(now)
	priorQuoted := priorStart.AddDate(0, -1, 0).Add(2 * time.Hour)
	holdOld := e.hold(t, nil, e.account, "task_quotaP0001", e.userA, 30, "quota-hold-p1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaP0001", e.userA, priorQuoted, holdOld, 30)
	captureOld := e.capture(t, holdOld, e.userA, 30, "quota-capture-p1", "task_quotaP0001")
	e.settleTaskCaptured(t, captureOld, "", "task_quotaP0001", 3, 30, 0)
	if got := e.usageMonthly(t, e.userA); got != 0 {
		t.Fatalf("monthly usage = %d, want 0 (prior month excluded, no rollover)", got)
	}
	// A pending hold from the prior Monday-start week counts for the
	// month only when it falls in the same calendar month; it always
	// stays out of the current week and day windows.
	weekStart, _ := CurrentWeekBounds(now)
	monthStart, _ := CurrentMonthBounds(now)
	priorWeekQuoted := weekStart.AddDate(0, 0, -7).Add(2 * time.Hour)
	holdW := e.hold(t, nil, e.account, "task_quotaP0002", e.userA, 30, "quota-hold-p2")
	e.insertTaskSnapshot(t, e.pool, e.account, e.ws2ID, "task_quotaP0002", e.userA, priorWeekQuoted, holdW, 30)
	wantMonthly := int64(0)
	if !priorWeekQuoted.Before(monthStart) {
		wantMonthly = 30
	}
	if got := e.usageMonthly(t, e.userA); got != wantMonthly {
		t.Fatalf("monthly usage = %d, want %d (prior-week hold)", got, wantMonthly)
	}
	ws, we := CurrentWeekBounds(now)
	weekUsed, err := e.store.UsageForPeriod(context.Background(), e.account, e.userA, &ws, &we)
	if err != nil {
		t.Fatalf("weekly usage: %v", err)
	}
	if weekUsed != 0 {
		t.Fatalf("weekly usage = %d, want 0 (prior week excluded)", weekUsed)
	}
	ds, de := CurrentDayBounds(now)
	dayUsed, err := e.store.UsageForPeriod(context.Background(), e.account, e.userA, &ds, &de)
	if err != nil {
		t.Fatalf("daily usage: %v", err)
	}
	if dayUsed != 0 {
		t.Fatalf("daily usage = %d, want 0 (prior day excluded)", dayUsed)
	}
	// History rows survive the window reset.
	var snapshotCount int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM task_billing_snapshots WHERE billing_account_id = $1 AND user_id = $2`, e.account, e.userA).Scan(&snapshotCount); err != nil || snapshotCount != 2 {
		t.Fatalf("history snapshots = %d/err %v, want 2 retained", snapshotCount, err)
	}
}

func TestQuotaAdjustKeepsUsage(t *testing.T) {
	e := openQuotaEnv(t)
	e.setFixed(t, e.userA, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, e.account, "task_quotaJ0001", e.userA, 30, "quota-hold-j1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaJ0001", e.userA, now, holdTx, 30)
	holdB := e.hold(t, nil, e.account, "task_quotaJ0002", e.userA, 30, "quota-hold-j2")
	e.insertTaskSnapshot(t, e.pool, e.account, e.ws2ID, "task_quotaJ0002", e.userA, now, holdB, 30)
	// Lowering below usage blocks everything; usage itself is untouched.
	lower := int64(50)
	if _, err := e.store.SetQuota(context.Background(), e.account, e.userA, PeriodFixed, &lower, e.userA); err != nil {
		t.Fatalf("lower quota: %v", err)
	}
	if got := e.usageFixed(t, e.userA); got != 60 {
		t.Fatalf("usage after lower = %d, want 60 retained", got)
	}
	if err := e.checkTx(t, e.userA, 1); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("check after lower error = %v, want ErrQuotaExhausted", err)
	}
	// Switching windows keeps the facts: the same settled usage reads in
	// the new window, and the check enforces the new cap.
	monthly := int64(200)
	if _, err := e.store.SetQuota(context.Background(), e.account, e.userA, PeriodMonthly, &monthly, e.userA); err != nil {
		t.Fatalf("switch quota window: %v", err)
	}
	if got := e.usageMonthly(t, e.userA); got != 60 {
		t.Fatalf("usage after window switch = %d, want 60 retained", got)
	}
	if err := e.checkTx(t, e.userA, 100); err != nil {
		t.Fatalf("check after switch: %v", err)
	}
	// Switching to unlimited clears the gate but keeps the facts.
	if _, err := e.store.SetQuota(context.Background(), e.account, e.userA, PeriodUnlimited, nil, e.userA); err != nil {
		t.Fatalf("unlimit quota: %v", err)
	}
	if err := e.checkTx(t, e.userA, 1_000_000); err != nil {
		t.Fatalf("check after unlimit: %v", err)
	}
}

func TestQuotaRejoinKeepsCapAndUsage(t *testing.T) {
	e := openQuotaEnv(t)
	e.setMonthly(t, e.userA, 100)
	now := time.Now().UTC()
	holdTx := e.hold(t, nil, e.account, "task_quotaN0001", e.userA, 30, "quota-hold-n1")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, "task_quotaN0001", e.userA, now, holdTx, 30)
	ctx := context.Background()
	// The member leaves every workspace sharing the account...
	if _, err := e.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE user_id = $1 AND workspace_id IN ($2, $3)`, e.userA, e.wsID, e.ws2ID); err != nil {
		t.Fatalf("leave memberships: %v", err)
	}
	// ...and rejoins at full standing (no proration, no re-issue).
	if _, err := e.pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member'), ($3, $2, 'member')`, e.wsID, e.userA, e.ws2ID); err != nil {
		t.Fatalf("rejoin memberships: %v", err)
	}
	// The cap row and the accrued usage survive the round trip.
	period, limit, found, err := e.store.GetQuotaRow(ctx, e.account, e.userA)
	if err != nil || !found || period != PeriodMonthly || limit == nil || *limit != 100 {
		t.Fatalf("quota after rejoin = (%q, %v, %v, %v), want (monthly, 100, true, nil)", period, limit, found, err)
	}
	if got := e.usageMonthly(t, e.userA); got != 30 {
		t.Fatalf("usage after rejoin = %d, want 30 retained", got)
	}
	if err := e.checkTx(t, e.userA, 80); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("check after rejoin error = %v, want ErrQuotaExhausted", err)
	}
}

func TestQuotaConcurrentSubmitsFenced(t *testing.T) {
	e := openQuotaEnv(t)
	e.setFixed(t, e.userA, 100)
	now := time.Now().UTC()

	tx1, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(context.Background())
	if err := e.store.CheckQuotaInTx(context.Background(), tx1, e.account, e.userA, 60, time.Now().UTC()); err != nil {
		t.Fatalf("first check: %v", err)
	}
	holdTx := e.hold(t, tx1, e.account, "task_quotaC0001", e.userA, 60, "quota-hold-c1")
	e.insertTaskSnapshot(t, tx1, e.account, e.ws2ID, "task_quotaC0001", e.userA, now, holdTx, 60)

	// The loser starts while the winner is still uncommitted: its quota
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
		loser <- checkOutcome{err: e.store.CheckQuotaInTx(context.Background(), tx2, e.account, e.userA, 60, time.Now().UTC())}
	}()
	// Let the loser block on the row lock, then commit the winner.
	time.Sleep(300 * time.Millisecond)
	if err := tx1.Commit(context.Background()); err != nil {
		t.Fatalf("commit winner: %v", err)
	}
	wg.Wait()
	out := <-loser
	if !errors.Is(out.err, ErrQuotaExhausted) {
		t.Fatalf("loser check error = %v, want ErrQuotaExhausted", out.err)
	}
	if got := e.usageFixed(t, e.userA); got != 60 {
		t.Fatalf("fenced usage = %d, want exactly the winner's 60", got)
	}
}

func mustQuotaAmount(t *testing.T, v int64) creditsdomain.CreditAmount {
	t.Helper()
	amount, err := creditsdomain.NewCreditAmount(v)
	if err != nil {
		t.Fatalf("NewCreditAmount(%d): %v", v, err)
	}
	return amount
}

func visionQuotaHoldMetadata(t *testing.T, userID int64, key, refID string) creditsdomain.CommandMetadata {
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
	e := openQuotaEnv(t)
	ctx := context.Background()
	limit := int64(100)
	if _, err := e.store.SetQuota(ctx, e.account, e.userA, PeriodMonthly, &limit, e.userA); err != nil {
		t.Fatal(err)
	}
	reservedAt := time.Now().UTC()
	const taskID = "task_capture_race"
	holdID := e.hold(t, nil, e.account, taskID, e.userA, 100, "capture-race-hold")
	e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, taskID, e.userA, reservedAt, holdID, 100)
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
		capture, err := e.credits.SettlementInTx(terminal).Capture(ctx, creditsports.CaptureCommand{
			BillingAccountID: e.account, HoldTransactionID: holdID, Amount: mustQuotaAmount(t, 100),
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
	if err := e.store.CheckQuotaInTx(ctx, wrapped, e.account, e.userA, 100, reservedAt); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("existing 100/100 usage plus 100: got %v, want ErrQuotaExhausted", err)
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
		{"daily", PeriodDaily, time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)},
		{"weekly", PeriodWeekly, time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := openQuotaEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			limit := int64(100)
			if _, err := e.store.SetQuota(ctx, e.account, e.userA, tc.period, &limit, e.userA); err != nil {
				t.Fatal(err)
			}
			before := tc.boundary.Add(-time.Millisecond)
			const taskID = "task_period_boundary"
			holdID := e.hold(t, nil, e.account, taskID, e.userA, 100, "period-hold")
			e.insertTaskSnapshot(t, e.pool, e.account, e.wsID, taskID, e.userA, before, holdID, 100)
			if err := e.store.CheckQuotaForQuote(ctx, e.account, e.userA, 10, before); !errors.Is(err, ErrQuotaExhausted) {
				t.Fatalf("quote in original full period: %v", err)
			}
			if err := e.store.CheckQuotaForQuote(ctx, e.account, e.userA, 10, tc.boundary); err != nil {
				t.Fatalf("quote in new empty period: %v", err)
			}
			locker, err := e.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Rollback(context.Background())
			if _, err := locker.Exec(ctx, `SELECT user_id FROM billing_account_member_quotas WHERE billing_account_id=$1 AND user_id=$2 FOR UPDATE`, e.account, e.userA); err != nil {
				t.Fatal(err)
			}
			waiter, err := e.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer waiter.Rollback(context.Background())
			waiterPID := waiter.Conn().PgConn().PID()
			done := make(chan error, 1)
			go func() { done <- e.store.CheckQuotaInTx(ctx, waiter, e.account, e.userA, 10, before) }()
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
			if err := <-done; !errors.Is(err, ErrQuotaExhausted) {
				t.Fatalf("blocked reservation must use original full period: %v", err)
			}
			if err := e.store.CheckQuotaInTx(ctx, waiter, e.account, e.userA, 10, tc.boundary); err != nil {
				t.Fatalf("reservation in next empty period: %v", err)
			}
		})
	}
}
