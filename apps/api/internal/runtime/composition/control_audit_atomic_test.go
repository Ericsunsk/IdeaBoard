package composition

// Control audit atomicity tests (step 2).
//
// The guard-level tests prove the guard begins/commits/rolls back
// around the participant writer. These prove the OTHER half: that the
// participant write actually shares one fate with a business write in
// the same pgx transaction, against REAL postgres rows:
//
//   - commit: a business row plus the audit row become visible together,
//     and the audit row is already visible INSIDE the transaction
//     (same-transaction visibility, not a post-commit dual-write).
//   - business rollback: the audit row does not exist (impossible in
//     step 1, where the detached write survived the business rollback).
//   - audit failure (CHECK violation): the business row rolls back with
//     it (impossible in step 1, where the failure was warn-swallowed).
//
// The "business fact" is a TEMP probe table on one acquired connection
// (unique per test, dropped on cleanup): no migration, no pollution of
// shared tables, and the temp scope keeps concurrent package tests from
// observing probe rows.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/platform/audit"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stubControlAtomicTx is a non-nil transaction carrier for sink unit
// tests: it embeds the interface so no method needs a body (the store
// fakes never touch the transaction), while still failing the sink's
// nil-transaction wiring check if left unset.
type stubControlAtomicTx struct {
	pgx.Tx
}

// openControlAtomicProbe stages one session-private business probe
// table and returns the held connection: the temp table, the business
// INSERT, and the audit INSERT must all ride this one session, or the
// atomicity proof is void.
func openControlAtomicProbe(t *testing.T, pool *pgxpool.Pool, table string) (context.Context, *pgxpool.Conn) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire probe connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
		conn.Release()
	})
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	return ctx, conn
}

func controlAtomicTable(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("control_atomic_probe_%d", time.Now().UnixNano())
	return strings.ReplaceAll(strings.ToLower(name), "-", "_")
}

func controlAtomicParams(operatorNumeric int64, requestID string) audit.RecordParams {
	return audit.RecordParams{
		OperatorUserID: operatorNumeric,
		OperatorType:   "human_session",
		Operation:      "users.ban",
		Capability:     "control:users",
		TargetType:     "route",
		TargetID:       "POST /users/{user_id}/ban",
		Outcome:        audit.OutcomeSuccess,
		RequestID:      requestID,
		IdempotencyKey: "atomic-key-1",
		Route:          "POST /users/{user_id}/ban",
		Status:         http.StatusOK,
	}
}

func countControlAuditByRequestID(t *testing.T, pool *pgxpool.Pool, requestID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	return count
}

func TestControlAuditAtomicCommitSharesOneFate(t *testing.T) {
	pool := testdb.Open(t)
	operatorNumeric, _ := seedControlAuditUser(t, pool, "ctl-atomic-operator", "admin")
	table := controlAtomicTable(t)
	ctx, conn := openControlAtomicProbe(t, pool, table)
	requestID := fmt.Sprintf("req-atomic-commit-%d", time.Now().UnixNano())

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe tx: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+table+` (id, note) VALUES ('biz-1', 'business fact')`); err != nil {
		t.Fatalf("insert business probe: %v", err)
	}
	store := audit.NewStore(pool)
	if err := store.AppendRecordTx(ctx, tx, controlAtomicParams(operatorNumeric, requestID)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append audit in tx: %v", err)
	}
	// Same-transaction visibility: both rows must be readable from
	// INSIDE the still-open transaction (a post-commit dual-write
	// could never satisfy this).
	var insideBiz int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = 'biz-1'`).Scan(&insideBiz); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("read business probe inside tx: %v", err)
	}
	if insideBiz != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("business rows visible inside tx = %d, want 1", insideBiz)
	}
	var inside int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("read audit inside tx: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("audit rows visible inside tx = %d, want 1", inside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit probe tx: %v", err)
	}
	// The temp probe table is session-private to the acquired
	// connection, so post-commit business visibility is proven by the
	// inside-tx assertion above, not re-queried here.
	if got := countControlAuditByRequestID(t, pool, requestID); got != 1 {
		t.Fatalf("audit rows after commit = %d, want 1 (business and audit commit together)", got)
	}
}

func TestControlAuditAtomicRollbackRemovesBoth(t *testing.T) {
	pool := testdb.Open(t)
	operatorNumeric, _ := seedControlAuditUser(t, pool, "ctl-atomic-rollback", "admin")
	table := controlAtomicTable(t)
	ctx, conn := openControlAtomicProbe(t, pool, table)
	requestID := fmt.Sprintf("req-atomic-rollback-%d", time.Now().UnixNano())

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe tx: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+table+` (id, note) VALUES ('biz-2', 'doomed fact')`); err != nil {
		t.Fatalf("insert business probe: %v", err)
	}
	store := audit.NewStore(pool)
	if err := store.AppendRecordTx(ctx, tx, controlAtomicParams(operatorNumeric, requestID)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append audit in tx: %v", err)
	}
	// The business fails: the whole transaction, audit row included,
	// rolls back. Step 1 could not do this (its detached write
	// survived the business rollback as an orphan row).
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe tx: %v", err)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 0 {
		t.Fatalf("audit rows after business rollback = %d, want 0 (no orphan audit rows)", got)
	}
}

func TestControlAuditAtomicAuditFailureRollsBackBusiness(t *testing.T) {
	pool := testdb.Open(t)
	operatorNumeric, _ := seedControlAuditUser(t, pool, "ctl-atomic-failure", "admin")
	table := controlAtomicTable(t)
	ctx, conn := openControlAtomicProbe(t, pool, table)
	requestID := fmt.Sprintf("req-atomic-failure-%d", time.Now().UnixNano())

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe tx: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+table+` (id, note) VALUES ('biz-3', 'fact awaiting audit')`); err != nil {
		t.Fatalf("insert business probe: %v", err)
	}
	// A CHECK-violating audit write (empty capability and target):
	// the participant MUST report it, and the caller (the guard in
	// production, this test here) rolls the business write back with
	// it. Step 1 swallowed this exact failure with slog.Warn and kept
	// the business commit.
	bad := controlAtomicParams(operatorNumeric, requestID)
	bad.Capability = ""
	bad.TargetType = ""
	bad.TargetID = ""
	store := audit.NewStore(pool)
	if err := store.AppendRecordTx(ctx, tx, bad); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("CHECK-violating audit write succeeded, want error")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe tx: %v", err)
	}
	var business int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = 'biz-3'`).Scan(&business); err != nil {
		t.Fatalf("count business probe after rollback: %v", err)
	}
	if business != 0 {
		t.Fatalf("business probe rows after audit failure = %d, want 0 (audit failure rolls the business back)", business)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 0 {
		t.Fatalf("audit rows after failure = %d, want 0", got)
	}
}

func TestControlAuditAtomicSinkJoinsCallerTx(t *testing.T) {
	pool := testdb.Open(t)
	operatorNumeric, operatorUsr := seedControlAuditUser(t, pool, "ctl-atomic-sink", "admin")
	table := controlAtomicTable(t)
	ctx, conn := openControlAtomicProbe(t, pool, table)
	requestID := fmt.Sprintf("req-atomic-sink-%d", time.Now().UnixNano())

	repo := identitypostgres.NewRepository(pool)
	dir := httpapi.NewUsersRepositoryDirectory(repo)
	sink := newControlAuditAtomicSink(audit.NewStore(pool), dir)

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe tx: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+table+` (id, note) VALUES ('biz-4', 'sink fact')`); err != nil {
		t.Fatalf("insert business probe: %v", err)
	}
	// The record carries no domain target, so the step-1 route
	// placeholder must satisfy the target CHECK inside the
	// transaction too (no second normalization invented here).
	durable, err := sink.AppendControlAuditTx(ctx, tx, httpapi.ControlAuditRecord{
		OperatorType: "human_session",
		OperatorID:   operatorUsr,
		Capability:   "control:users",
		Operation:    "users.list",
		Route:        "GET /users",
		RequestID:    requestID,
		Outcome:      httpapi.ControlAuditOK,
		Status:       http.StatusOK,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("atomic sink join tx: %v", err)
	}
	if !durable {
		_ = tx.Rollback(ctx)
		t.Fatal("atomic sink reported slog-only for a resolvable operator, want durable=true")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit probe tx: %v", err)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 1 {
		t.Fatalf("audit rows after sink commit = %d, want 1", got)
	}
	if operatorNumeric <= 0 {
		t.Fatal("seeded operator must be positive")
	}
}

func TestControlAuditAtomicSinkFailureShapes(t *testing.T) {
	ctx := context.Background()

	t.Run("nil tx is a wiring error", func(t *testing.T) {
		sink := newControlAuditAtomicSink(&recordingControlAuditStore{}, stubControlAuditDirectory{id: 7})
		if _, err := sink.AppendControlAuditTx(ctx, nil, httpapi.ControlAuditRecord{
			OperatorType: "human_session",
			OperatorID:   "usr_0123456789abcdef0123456789abcdef",
			Capability:   "control:users",
			Operation:    "users.ban",
			Route:        "POST /users/{user_id}/ban",
			Outcome:      httpapi.ControlAuditOK,
			Status:       http.StatusOK,
		}); err == nil {
			t.Fatal("nil tx succeeded, want wiring error (the guard must fail, not commit unaudited)")
		}
	})

	t.Run("service subjects keep the slog trail only", func(t *testing.T) {
		store := &recordingControlAuditStore{}
		sink := newControlAuditAtomicSink(store, stubControlAuditDirectory{id: 7})
		durable, err := sink.AppendControlAuditTx(ctx, stubControlAtomicTx{}, httpapi.ControlAuditRecord{
			OperatorType: "service",
			OperatorID:   "control-plane",
			Capability:   "control:users",
			Operation:    "users.ban",
			Route:        "POST /users/{user_id}/ban",
			Outcome:      httpapi.ControlAuditOK,
			Status:       http.StatusOK,
		})
		if err != nil {
			t.Fatalf("service subject failed: %v", err)
		}
		if durable {
			t.Fatal("service subject must degrade to slog-only (durable=false), never fail or write")
		}
		if len(store.params) != 0 {
			t.Fatalf("service subject wrote %d rows, want 0", len(store.params))
		}
	})

	t.Run("store errors are returned, never swallowed", func(t *testing.T) {
		// Step-1 contrast: the observer sink slog.Warn-logs this same
		// failure and keeps the business commit. The participant must
		// hand it back so the guard rolls the mutation back.
		sink := newControlAuditAtomicSink(
			&failingControlAuditStore{err: errors.New("audit store down")},
			stubControlAuditDirectory{id: 11},
		)
		_, err := sink.AppendControlAuditTx(ctx, stubControlAtomicTx{}, httpapi.ControlAuditRecord{
			OperatorType: "human_session",
			OperatorID:   "usr_0123456789abcdef0123456789abcdef",
			Capability:   "control:users",
			Operation:    "users.ban",
			Route:        "POST /users/{user_id}/ban",
			RequestID:    "req-atomic-swallow",
			Outcome:      httpapi.ControlAuditOK,
			Status:       http.StatusOK,
		})
		if err == nil {
			t.Fatal("failing store was swallowed, want the error back (business must roll back)")
		}
	})
}
