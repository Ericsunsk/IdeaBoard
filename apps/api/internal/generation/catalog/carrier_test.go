package catalog

// Control audit atomicity: the catalog pool seams run through
// dbutil.WithTxDBTX, which joins the ambient guard transaction staged
// in the context instead of opening a new transaction (no commit or
// rollback on the borrowed path; the guard owns its lifecycle).
//
// What is proven here, against the isolated fixture database:
//   - ReplacePool on a carrier context lands inside the carrier
//     transaction: ListPool on the SAME carrier sees the uncommitted
//     version and entries, while the pool still reads version 0; after
//     the carrier commits the pool reads the new version. (An owned
//     commit would already be visible on the pool, so the pair proves
//     no new transaction was opened.)
//   - A carrier rollback leaves nothing behind: version stays 0 and
//     the pool reads empty.
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing catalog suite is that proof; it is not duplicated
//     here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func beginCatalogCarrier(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, context.Context) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	return tx, dbutil.ContextWithAmbientTx(ctx, tx)
}

func carrierPoolInputs() []PoolEntryInput {
	return []PoolEntryInput{
		{Provider: "b", Model: "second", Enabled: true, Resolutions: []string{"2k"}},
		{Provider: "a", Model: "first", Enabled: true, Resolutions: []string{"1k"}},
	}
}

func TestReplacePoolJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	r := NewRepository(pool)
	ctx := context.Background()

	tx, carrier := beginCatalogCarrier(t, ctx, pool)
	result, err := r.ReplacePool(carrier, "generation", 0, carrierPoolInputs())
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ReplacePool() on carrier error = %v", err)
	}
	if result.Version != 1 || !result.Changed {
		_ = tx.Rollback(ctx)
		t.Fatalf("result = %+v, want {Version:1 Changed:true}", result)
	}
	// Same-transaction visibility: ListPool on the carrier must see the
	// uncommitted write (a post-commit dual-write could never do this,
	// and a fresh transaction under READ COMMITTED could not see it
	// either).
	inside, err := r.ListPool(carrier, "generation")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ListPool() on carrier error = %v", err)
	}
	if inside.Version != 1 || len(inside.Entries) != 2 {
		_ = tx.Rollback(ctx)
		t.Fatalf("pool inside carrier = %+v, want version 1 with 2 entries", inside)
	}
	// ... and invisible outside it before the carrier commits.
	if version, err := r.GetVersion(ctx, "generation"); err != nil || version != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("GetVersion() on pool before commit = %d error=%v, want 0", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	version, err := r.GetVersion(ctx, "generation")
	if err != nil {
		t.Fatalf("GetVersion() after carrier commit error = %v", err)
	}
	if version != 1 {
		t.Fatalf("version after carrier commit = %d, want 1", version)
	}
}

func TestReplacePoolRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	r := NewRepository(pool)
	ctx := context.Background()

	tx, carrier := beginCatalogCarrier(t, ctx, pool)
	if _, err := r.ReplacePool(carrier, "generation", 0, carrierPoolInputs()); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ReplacePool() on carrier error = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if version, err := r.GetVersion(ctx, "generation"); err != nil || version != 0 {
		t.Fatalf("GetVersion() after carrier rollback = %d error=%v, want 0", version, err)
	}
	poolAfter, err := r.ListPool(ctx, "generation")
	if err != nil {
		t.Fatalf("ListPool() after carrier rollback error = %v", err)
	}
	if poolAfter.Version != 0 || len(poolAfter.Entries) != 0 {
		t.Fatalf("pool after carrier rollback = %+v, want empty version 0", poolAfter)
	}
}
