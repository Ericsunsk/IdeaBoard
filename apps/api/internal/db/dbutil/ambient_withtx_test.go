package dbutil

// Ambient-aware WithTx / WithTxDBTX (control audit atomicity).
//
// What is proven here, against the isolated fixture database plus one
// pool-less stub:
//   - With a carrier in ctx, WithTxDBTX joins it: fn runs on the
//     carrier (same-transaction read-back visible), the pool opens no
//     new connection (acquire count unchanged), and the helper neither
//     commits (row invisible on the pool before the guard commits) nor
//     rolls back.
//   - With a carrier, WithTx (the Queries-only variant) joins the same
//     way; it delegates to WithTxDBTX, and this pins that delegation.
//   - fn error on a carrier propagates unchanged and leaves the carrier
//     usable: no rollback (partial write still visible inside) and no
//     commit (invisible outside); the guard later rolls back cleanly.
//   - A pool-less stub carrier proves the lifecycle iron law without a
//     database: neither Commit nor Rollback is ever invoked, on success
//     or on fn error, and no pool is required.
//   - With no carrier the legacy owned-transaction path commits as
//     before (the wider suite exercises every caller; this pins the
//     commit at the helper level).

import (
	"context"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func beginCarrier(t *testing.T, ctx context.Context, db *pgxpool.Pool) (pgx.Tx, context.Context, func()) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	committed := false
	t.Cleanup(func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	})
	return tx, ContextWithAmbientTx(ctx, tx), func() { committed = true }
}

func acquireCount(db *pgxpool.Pool) int64 {
	return db.Stat().AcquireCount()
}

func TestWithTxDBTXJoinsAmbientCarrier(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const key = "dbutil ambient join key"

	tx, carrier, markDone := beginCarrier(t, ctx, db)
	defer markDone()

	before := acquireCount(db)
	ran := false
	if err := WithTxDBTX(carrier, db, func(q *dbsqlc.Queries, raw pgx.Tx) error {
		ran = true
		if raw == nil {
			t.Error("WithTxDBTX carrier tx handle is nil")
		}
		// Write through the helper-bound Queries (proves q targets the
		// carrier), read back through the raw handle (proves both
		// handles share one transaction).
		if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: "carrier-value"}); err != nil {
			t.Fatalf("upsert inside carrier: %v", err)
		}
		var inside int
		if err := raw.QueryRow(ctx, `SELECT count(*) FROM system_configs WHERE config_key = $1`, key).Scan(&inside); err != nil {
			t.Fatalf("count configs inside carrier: %v", err)
		}
		if inside != 1 {
			t.Fatalf("config rows visible inside carrier = %d, want 1", inside)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTxDBTX on carrier error = %v", err)
	}
	if !ran {
		t.Fatal("WithTxDBTX fn did not run")
	}
	if delta := acquireCount(db) - before; delta != 0 {
		t.Fatalf("pool acquires during carrier join = %d, want 0 (no new transaction)", delta)
	}
	// The helper must not have committed: invisible on the pool ...
	if _, err := dbsqlc.New(db).GetSystemConfig(ctx, key); err == nil {
		t.Fatal("GetSystemConfig saw the row before the carrier committed, want not found")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	markDone()
	// ... durable after the guard commits.
	cfg, err := dbsqlc.New(db).GetSystemConfig(ctx, key)
	if err != nil {
		t.Fatalf("GetSystemConfig after carrier commit error = %v", err)
	}
	if cfg.Value != "carrier-value" {
		t.Fatalf("config value after carrier commit = %q, want %q", cfg.Value, "carrier-value")
	}
}

func TestWithTxJoinsAmbientCarrier(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const key = "dbutil ambient join (WithTx) key"

	tx, carrier, markDone := beginCarrier(t, ctx, db)
	defer markDone()

	before := acquireCount(db)
	if err := WithTx(carrier, db, func(q *dbsqlc.Queries) error {
		return q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: "carrier-value"})
	}); err != nil {
		t.Fatalf("WithTx on carrier error = %v", err)
	}
	if delta := acquireCount(db) - before; delta != 0 {
		t.Fatalf("pool acquires during carrier join = %d, want 0 (no new transaction)", delta)
	}
	var inside int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM system_configs WHERE config_key = $1`, key).Scan(&inside); err != nil {
		t.Fatalf("count configs inside carrier: %v", err)
	}
	if inside != 1 {
		t.Fatalf("config rows visible inside carrier = %d, want 1", inside)
	}
	if _, err := dbsqlc.New(db).GetSystemConfig(ctx, key); err == nil {
		t.Fatal("GetSystemConfig saw the row before the carrier committed, want not found")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	markDone()
}

func TestWithTxDBTXAmbientErrorPropagatesWithoutRollback(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const key = "dbutil ambient error key"
	sentinel := errors.New("dbutil ambient fn boom")

	tx, carrier, markDone := beginCarrier(t, ctx, db)
	defer markDone()

	err := WithTxDBTX(carrier, db, func(q *dbsqlc.Queries, _ pgx.Tx) error {
		if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: "doomed-value"}); err != nil {
			t.Fatalf("upsert inside carrier: %v", err)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTxDBTX carrier fn error = %v, want sentinel", err)
	}
	// No rollback: the partial write is still visible inside the carrier
	// (the guard, not the helper, decides the transaction's fate).
	var inside int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM system_configs WHERE config_key = $1`, key).Scan(&inside); err != nil {
		t.Fatalf("count configs inside carrier after fn error: %v", err)
	}
	if inside != 1 {
		t.Fatalf("config rows visible inside carrier after fn error = %d, want 1 (no rollback)", inside)
	}
	// No commit either: still invisible outside.
	if _, err := dbsqlc.New(db).GetSystemConfig(ctx, key); err == nil {
		t.Fatal("GetSystemConfig saw the row after fn error, want not found (no commit)")
	}
	// The guard rolls the whole unit back; nothing survives.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if _, err := dbsqlc.New(db).GetSystemConfig(ctx, key); err == nil {
		t.Fatal("GetSystemConfig saw the row after carrier rollback, want not found")
	}
}

func TestWithTxDBTXLegacyCommitsWithoutCarrier(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const key = "dbutil legacy commit key"

	before := acquireCount(db)
	if err := WithTxDBTX(ctx, db, func(q *dbsqlc.Queries, raw pgx.Tx) error {
		if raw == nil {
			t.Error("WithTxDBTX legacy tx handle is nil")
		}
		return q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: "legacy-value"})
	}); err != nil {
		t.Fatalf("WithTxDBTX without carrier error = %v", err)
	}
	if delta := acquireCount(db) - before; delta < 1 {
		t.Fatalf("pool acquires for owned transaction = %d, want >= 1", delta)
	}
	cfg, err := dbsqlc.New(db).GetSystemConfig(ctx, key)
	if err != nil {
		t.Fatalf("GetSystemConfig after owned commit error = %v", err)
	}
	if cfg.Value != "legacy-value" {
		t.Fatalf("config value after owned commit = %q, want %q", cfg.Value, "legacy-value")
	}
}

// ambientTxStub tracks lifecycle calls on a borrowed transaction. It
// embeds pgx.Tx so only Commit/Rollback need overrides; the stub body
// is never used for statements in these tests.
type ambientTxStub struct {
	pgx.Tx
	commits   int
	rollbacks int
}

func (s *ambientTxStub) Commit(context.Context) error {
	s.commits++
	return nil
}

func (s *ambientTxStub) Rollback(context.Context) error {
	s.rollbacks++
	return nil
}

func TestWithTxDBTXAmbientStubNeverTouchesLifecycle(t *testing.T) {
	ctx := context.Background()
	stub := &ambientTxStub{}
	carrier := ContextWithAmbientTx(ctx, stub)

	// Success: fn runs, no pool needed, lifecycle untouched.
	ran := false
	if err := WithTxDBTX(carrier, nil, func(_ *dbsqlc.Queries, raw pgx.Tx) error {
		ran = true
		if raw != stub {
			t.Error("WithTxDBTX did not hand the carrier tx to fn")
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTxDBTX on stub carrier error = %v", err)
	}
	if !ran {
		t.Fatal("WithTxDBTX fn did not run")
	}

	// Error: propagates, lifecycle still untouched.
	sentinel := errors.New("dbutil stub fn boom")
	if err := WithTxDBTX(carrier, nil, func(_ *dbsqlc.Queries, _ pgx.Tx) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("WithTxDBTX stub fn error = %v, want sentinel", err)
	}

	// WithTx variant joins the same way.
	if err := WithTx(carrier, nil, func(_ *dbsqlc.Queries) error { return nil }); err != nil {
		t.Fatalf("WithTx on stub carrier error = %v", err)
	}

	if stub.commits != 0 || stub.rollbacks != 0 {
		t.Fatalf("carrier lifecycle touched: commits=%d rollbacks=%d, want 0/0 (guard-owned)", stub.commits, stub.rollbacks)
	}
}
