package consumptionread_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/consumptionread"
	"ideaboard/api/internal/credits"
	cd "ideaboard/api/internal/credits/domain"
	cp "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type usageFacts struct {
	pool                                                                     *pgxpool.Pool
	credits                                                                  *credits.Service
	user, otherUser, workspace, otherWorkspace, account, otherAccount, price int64
	seq                                                                      int
}

func openUsageFacts(t *testing.T) *usageFacts {
	t.Helper()
	e := &usageFacts{pool: testdb.Open(t)}
	e.credits = credits.NewService(e.pool)
	e.row(t, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('projection user','PROJ01','user',1) RETURNING id`, &e.user)
	e.row(t, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('other user','PROJ02','user',1) RETURNING id`, &e.otherUser)
	e.row(t, `INSERT INTO workspaces (public_id,owner_user_id,name) VALUES ('ws_projection',$1,'projection') RETURNING internal_id`, &e.workspace, e.user)
	e.row(t, `INSERT INTO workspaces (public_id,owner_user_id,name) VALUES ('ws_projection2',$1,'other') RETURNING internal_id`, &e.otherWorkspace, e.user)
	e.row(t, `INSERT INTO billing_accounts (public_id,workspace_id) VALUES ('ba_projection',$1) RETURNING internal_id`, &e.account, e.workspace)
	e.row(t, `INSERT INTO billing_accounts (public_id,workspace_id) VALUES ('ba_projection2',$1) RETURNING internal_id`, &e.otherAccount, e.otherWorkspace)
	for _, account := range []int64{e.account, e.otherAccount} {
		_, err := e.credits.Grant(t.Context(), cp.GrantCommand{BillingAccountID: account, Amount: amount(t, 100000), Metadata: e.meta("test", "seed", e.user)})
		if err != nil {
			t.Fatal(err)
		}
	}
	e.exec(t, `INSERT INTO billing_skus (sku,display_name,enabled,created_by,updated_by) VALUES ('ai-image:projection','Projection',true,1,1)`)
	e.row(t, `INSERT INTO billing_price_versions (sku,output_resolution,version,unit_credits,created_by) VALUES ('ai-image:projection','1k',1,10,1) RETURNING price_version_id`, &e.price)
	return e
}
func (e *usageFacts) row(t *testing.T, sql string, dest any, args ...any) {
	t.Helper()
	if err := e.pool.QueryRow(t.Context(), sql, args...).Scan(dest); err != nil {
		t.Fatal(err)
	}
}
func (e *usageFacts) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func amount(t *testing.T, n int64) cd.CreditAmount {
	t.Helper()
	v, err := cd.NewCreditAmount(n)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func (e *usageFacts) meta(kind, id string, user int64) cd.CommandMetadata {
	e.seq++
	return cd.CommandMetadata{IdempotencyKey: fmt.Sprintf("projection-%d", e.seq), Actor: cd.Actor{Type: cd.ActorUser, ID: fmt.Sprint(user)}, Source: "test", Reason: "projection fixture", BusinessReference: cd.BusinessReference{Type: kind, ID: id}}
}
func (e *usageFacts) hold(t *testing.T, account, user int64, kind, id string, n int64) string {
	t.Helper()
	r, err := e.credits.Hold(t.Context(), cp.HoldCommand{BillingAccountID: account, Amount: amount(t, n), Metadata: e.meta(kind, id, user)})
	if err != nil {
		t.Fatal(err)
	}
	return r.TransactionID
}
func (e *usageFacts) capture(t *testing.T, account, user int64, hold, kind, id string, n int64) string {
	t.Helper()
	r, err := e.credits.Capture(t.Context(), cp.CaptureCommand{BillingAccountID: account, HoldTransactionID: hold, Amount: amount(t, n), Metadata: e.meta(kind, id, user)})
	if err != nil {
		t.Fatal(err)
	}
	return r.TransactionID
}
func (e *usageFacts) refund(t *testing.T, account, user int64, capture string, n int64) {
	t.Helper()
	m := e.meta("test", "refund", user)
	m.Actor = cd.Actor{Type: cd.ActorAdmin, ID: fmt.Sprint(user)}
	_, err := e.credits.Refund(t.Context(), cp.RefundCommand{BillingAccountID: account, ChargeTransactionID: capture, Amount: amount(t, n), Metadata: m})
	if err != nil {
		t.Fatal(err)
	}
}
func (e *usageFacts) task(t *testing.T, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, account, workspace, user int64, at time.Time, n int64) string {
	t.Helper()
	id := fmt.Sprintf("projection-task-%d", e.seq)
	hold := e.hold(t, account, user, "task", id, n)
	_, err := q.Exec(t.Context(), `INSERT INTO task_billing_snapshots (task_id,user_id,billing_account_id,workspace_id,service,sku,display_name,price_version_id,price_version,unit_credits,quantity,total_credits,quoted_at,hold_transaction_id) VALUES ($1,$2,$3,$4,'render','ai-image:projection','Projection',$5,1,10,$6/10,$6,$7,$8)`, id, user, account, workspace, e.price, n, at, hold)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.Exec(t.Context(), `INSERT INTO task_billing_settlements(task_id,state) VALUES ($1,'held')`, id); err != nil {
		t.Fatal(err)
	}
	return id
}
func (e *usageFacts) vision(t *testing.T, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, account, workspace, user int64, at time.Time, n int64, status string) (string, string) {
	t.Helper()
	e.seq++
	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", e.seq)
	hold := e.hold(t, account, user, "ai_search_operation", id, n)
	var settled any
	if status == "succeeded" || status == "failed" {
		settled = at.Add(time.Hour)
	}
	var origin any = workspace
	if workspace == 0 {
		origin = nil
	}
	_, err := q.Exec(t.Context(), `INSERT INTO ai_search_operations(operation_id,user_id,workspace_id,billing_account_id,idempotency_key,request_hash,sku,price_version_id,unit_credits,hold_transaction_id,status,created_at,settled_at) VALUES ($1::uuid,$2,$3,$4,$1::text,$5,'ai-search:vision',1,$6,$7,$8,$9,$10)`, id, user, origin, account, strings.Repeat("a", 64), n, hold, status, at, settled)
	if err != nil {
		t.Fatal(err)
	}
	if status == "failed" {
		_, err := e.credits.Release(t.Context(), cp.ReleaseCommand{BillingAccountID: account, HoldTransactionID: hold, Amount: amount(t, n), Metadata: e.meta("ai_search_operation", id, user)})
		if err != nil {
			t.Fatal(err)
		}
	}
	return id, hold
}

func TestUsedCreditsProjectionScopesAndReservationWindows(t *testing.T) {
	e := openUsageFacts(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	e.task(t, e.pool, e.account, e.workspace, e.user, start, 100)
	e.task(t, e.pool, e.account, e.otherWorkspace, e.user, start, 20)
	e.task(t, e.pool, e.account, e.workspace, e.otherUser, start, 30)
	e.task(t, e.pool, e.otherAccount, e.otherWorkspace, e.user, start, 40)
	// Bounds include start and exclude end. An older pending reservation stays
	// in its original window regardless of the terminal operation's clock.
	e.task(t, e.pool, e.account, e.workspace, e.user, start.Add(-time.Millisecond), 50)
	e.task(t, e.pool, e.account, e.workspace, e.user, end, 60)
	e.vision(t, e.pool, e.account, e.workspace, e.user, start, 11, "pending")
	e.vision(t, e.pool, e.account, e.workspace, e.user, start, 13, "reconciliation_pending")
	e.vision(t, e.pool, e.account, e.workspace, e.user, start, 17, "failed")
	id, hold := e.vision(t, e.pool, e.account, e.workspace, e.user, start, 19, "succeeded")
	capture := e.capture(t, e.account, e.user, hold, "ai_search_operation", id, 19)
	e.refund(t, e.account, e.user, capture, 7)
	// A capture with a mismatched account and the same operation reference
	// must not supply this operation's refund attribution.
	wrongHold := e.hold(t, e.otherAccount, e.user, "ai_search_operation", id, 19)
	wrongCapture := e.capture(t, e.otherAccount, e.user, wrongHold, "ai_search_operation", id, 19)
	e.refund(t, e.otherAccount, e.user, wrongCapture, 19)
	e.vision(t, e.pool, e.account, e.otherWorkspace, e.user, start, 23, "pending")
	e.vision(t, e.pool, e.account, e.workspace, e.otherUser, start, 29, "pending")
	e.vision(t, e.pool, e.otherAccount, e.otherWorkspace, e.user, start, 31, "pending")
	e.vision(t, e.pool, e.account, 0, e.user, start, 37, "pending")
	// Unknown ledger origins never become cap usage without a persisted fact.
	e.hold(t, e.account, e.user, "task", "unknown-origin", 500)
	assertUsed(t, 195, func() (int64, error) {
		return consumptionread.UsedCreditsForWorkspace(t.Context(), e.pool, e.workspace, &start, &end)
	})
	assertUsed(t, 216, func() (int64, error) {
		return consumptionread.UsedCreditsForMember(t.Context(), e.pool, e.account, e.user, &start, &end)
	})
	batch, err := consumptionread.UsedCreditsForMembers(t.Context(), e.pool, e.account,
		[]int64{e.user, e.otherUser, e.user, 0, -1, 999999}, &start, &end)
	if err != nil || len(batch) != 2 || batch[e.user] != 216 || batch[e.otherUser] != 59 {
		t.Fatalf("batch usage = %v, %v; want scoped per-user totals", batch, err)
	}

	assertUsed(t, 71, func() (int64, error) {
		return consumptionread.UsedCreditsForMember(t.Context(), e.pool, e.otherAccount, e.user, &start, &end)
	})
	assertUsed(t, 305, func() (int64, error) {
		return consumptionread.UsedCreditsForWorkspace(t.Context(), e.pool, e.workspace, nil, nil)
	})
	// Unknown positive scopes remain empty rather than widening to all facts.
	assertUsed(t, 0, func() (int64, error) {
		return consumptionread.UsedCreditsForWorkspace(t.Context(), e.pool, 999999, nil, nil)
	})
	assertUsed(t, 0, func() (int64, error) {
		return consumptionread.UsedCreditsForMember(t.Context(), e.pool, e.account, 999999, nil, nil)
	})
}
func assertUsed(t *testing.T, want int64, read func() (int64, error)) {
	t.Helper()
	got, err := read()
	if err != nil || got != want {
		t.Fatalf("used=%d want=%d err=%v", got, want, err)
	}
}

func TestUsedCreditsProjectionIncludesLocalTransactionAndSettledActuals(t *testing.T) {
	e := openUsageFacts(t)
	at := time.Date(2026, 1, 31, 23, 59, 59, 999000000, time.UTC)
	id := e.task(t, e.pool, e.account, e.workspace, e.user, at, 100)
	var hold string
	e.row(t, `SELECT hold_transaction_id FROM task_billing_snapshots WHERE task_id=$1`, &hold, id)
	capture := e.capture(t, e.account, e.user, hold, "task", id, 60)
	released, err := e.credits.Release(t.Context(), cp.ReleaseCommand{BillingAccountID: e.account, HoldTransactionID: hold, Amount: amount(t, 40), Metadata: e.meta("task", id, e.user)})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE task_billing_settlements SET state='captured',actual_output_count=6,captured_credits=60,released_credits=40,capture_transaction_id=$2,release_transaction_id=$3,settled_at=now() WHERE task_id=$1`, id, capture, released.TransactionID)
	e.refund(t, e.account, e.user, capture, 10)
	id = e.task(t, e.pool, e.account, e.workspace, e.user, at, 70)
	e.row(t, `SELECT hold_transaction_id FROM task_billing_snapshots WHERE task_id=$1`, &hold, id)
	released, err = e.credits.Release(t.Context(), cp.ReleaseCommand{BillingAccountID: e.account, HoldTransactionID: hold, Amount: amount(t, 70), Metadata: e.meta("task", id, e.user)})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE task_billing_settlements SET state='released',released_credits=70,release_transaction_id=$2,settled_at=now() WHERE task_id=$1`, id, released.TransactionID)
	assertUsed(t, 50, func() (int64, error) {
		return consumptionread.UsedCreditsForWorkspace(t.Context(), e.pool, e.workspace, nil, nil)
	})
	tx, err := e.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	e.task(t, tx, e.account, e.workspace, e.user, at, 30)
	e.vision(t, tx, e.account, e.workspace, e.user, at, 15, "reconciliation_pending")
	assertUsed(t, 95, func() (int64, error) {
		return consumptionread.UsedCreditsForWorkspace(t.Context(), tx, e.workspace, nil, nil)
	})
	assertUsed(t, 95, func() (int64, error) {
		return consumptionread.UsedCreditsForMember(t.Context(), tx, e.account, e.user, nil, nil)
	})
	assertUsed(t, 50, func() (int64, error) {
		return consumptionread.UsedCreditsForMember(t.Context(), e.pool, e.account, e.user, nil, nil)
	})
}

func TestUsedCreditsProjectionRejectsAbsentScopeAndInvalidWindow(t *testing.T) {
	// nil executor makes accidental unscoped queries fail loudly.
	for _, id := range []int64{0, -1} {
		if _, err := consumptionread.UsedCreditsForWorkspace(t.Context(), nil, id, nil, nil); err == nil {
			t.Fatal("invalid Workspace accepted")
		}
		if _, err := consumptionread.UsedCreditsForMember(t.Context(), nil, id, 1, nil, nil); err == nil {
			t.Fatal("invalid account accepted")
		}
		if _, err := consumptionread.UsedCreditsForMember(t.Context(), nil, 1, id, nil, nil); err == nil {
			t.Fatal("invalid User accepted")
		}
	}
	at := time.Now()
	if _, err := consumptionread.UsedCreditsForWorkspace(t.Context(), nil, 1, &at, &at); err == nil {
		t.Fatal("empty period accepted")
	}
}

// Observing the real executor keeps plan evidence at the public read seam.
// It also ensures a full usage read stays one statement, irrespective of scope.
type observedQuery struct {
	*pgxpool.Pool
	sql   string
	args  []any
	calls int
}

func (q *observedQuery) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	q.calls++
	q.sql = sql
	q.args = args
	return q.Pool.QueryRow(ctx, sql, args...)
}
func TestUsedCreditsProjectionScopedQueryPlans(t *testing.T) {
	e := openUsageFacts(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Bounded real reservations in both scope dimensions, with a minority in
	// the requested scope. No planner setting or index/scan hint is forced.
	for i := 0; i < 96; i++ {
		account, workspace, user := e.otherAccount, e.otherWorkspace, e.otherUser
		if i%8 == 0 {
			account, workspace, user = e.account, e.workspace, e.user
		}
		e.task(t, e.pool, account, workspace, user, at, 10)
		e.vision(t, e.pool, account, workspace, user, at, 10, "pending")
	}
	e.exec(t, `ANALYZE task_billing_snapshots`)
	e.exec(t, `ANALYZE task_billing_settlements`)
	e.exec(t, `ANALYZE ai_search_operations`)
	for _, scope := range []string{"Workspace", "Member"} {
		t.Run(scope, func(t *testing.T) {
			q := &observedQuery{Pool: e.pool}
			assertUsed(t, 240, func() (int64, error) {
				if scope == "Workspace" {
					return consumptionread.UsedCreditsForWorkspace(t.Context(), q, e.workspace, nil, nil)
				}
				return consumptionread.UsedCreditsForMember(t.Context(), q, e.account, e.user, nil, nil)
			})
			if q.calls != 1 {
				t.Fatalf("usage statements=%d want=1", q.calls)
			}
			rows, err := e.pool.Query(t.Context(), `EXPLAIN (ANALYZE, BUFFERS) `+q.sql, q.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan []string
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, line)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			t.Log(strings.Join(plan, "\n"))
		})
	}
}
