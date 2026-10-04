package tests

// Control audit atomicity: the storage profile repository runs its
// writes through dbutil.WithTx, which joins the ambient guard
// transaction staged in the context instead of opening a new
// transaction (no commit or rollback on the borrowed path; the guard
// owns its lifecycle).
//
// What is proven here, against the isolated fixture database:
//   - SaveProfile on a carrier context lands inside the carrier
//     transaction: the profile row is readable from the still-open
//     carrier and invisible on the pool, then durable after the
//     carrier commits (an owned commit would already be visible on
//     the pool, so the pair proves no new transaction was opened).
//   - A carrier rollback leaves no profile row behind.
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing storage suites are that proof; they are not duplicated
//     here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// storageCarrierQuerier is satisfied by both pgx.Tx
// (same-transaction reads) and *pgxpool.Pool (outside reads) for the
// visibility assertions below.
type storageCarrierQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func beginStorageCarrier(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, context.Context) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	return tx, dbutil.ContextWithAmbientTx(ctx, tx)
}

func countStorageProfiles(t *testing.T, ctx context.Context, querier storageCarrierQuerier, id string) int {
	t.Helper()
	var count int
	if err := querier.QueryRow(ctx,
		`SELECT count(*) FROM storage_profiles WHERE profile_id = $1`, id).Scan(&count); err != nil {
		t.Fatalf("count storage profiles: %v", err)
	}
	return count
}

func TestStorageSaveProfileJoinsAmbientCarrierTx(t *testing.T) {
	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	ctx := context.Background()

	tx, carrier := beginStorageCarrier(t, ctx, db)
	if err := repo.SaveProfile(carrier, StorageProfile{
		ID: "carrier-local", Name: "carrier local",
		Provider: StorageProviderLocal, Enabled: true,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SaveProfile() on carrier error = %v", err)
	}
	// Same-transaction visibility: the row must be readable from
	// INSIDE the still-open carrier ...
	if got := countStorageProfiles(t, carrier, tx, "carrier-local"); got != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("profiles visible inside carrier = %d, want 1", got)
	}
	// ... and invisible outside it before the carrier commits.
	if got := countStorageProfiles(t, ctx, db, "carrier-local"); got != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("profiles visible on pool before commit = %d, want 0", got)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if got := countStorageProfiles(t, ctx, db, "carrier-local"); got != 1 {
		t.Fatalf("profiles on pool after commit = %d, want 1", got)
	}
}

func TestStorageSaveProfileRollsBackWithAmbientCarrier(t *testing.T) {
	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	ctx := context.Background()

	tx, carrier := beginStorageCarrier(t, ctx, db)
	if err := repo.SaveProfile(carrier, StorageProfile{
		ID: "carrier-local-rollback", Name: "carrier local rollback",
		Provider: StorageProviderLocal, Enabled: true,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SaveProfile() on carrier error = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if got := countStorageProfiles(t, ctx, db, "carrier-local-rollback"); got != 0 {
		t.Fatalf("profiles after carrier rollback = %d, want 0", got)
	}
}
