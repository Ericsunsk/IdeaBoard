package postgres

// Control audit atomicity, step 3: the tenancy mutations join the
// ambient guard transaction instead of committing in their own.
//
// What is proven here, against the isolated fixture database:
//   - CreateWorkspace on a carrier context lands inside the carrier:
//     the workspace row is readable from the still-open carrier and
//     invisible on the pool, then durable after the carrier commits.
//   - A carrier rollback leaves no workspace behind (the step-2 gap:
//     the tenancy commit used to survive on its own).
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing tenancy suite is that proof; it is not duplicated here).

import (
	"context"
	"fmt"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testdb"
)

func TestCreateWorkspaceJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	store := NewStore(pool)
	ctx := context.Background()
	const ownerID = 4301
	if _, err := pool.Exec(ctx, `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`, ownerID, fmt.Sprintf("carrier-owner-%d", ownerID), fmt.Sprintf("C%05d", ownerID)); err != nil {
		t.Fatalf("seed workspace owner: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	created, err := store.CreateWorkspace(carrier, tenancy.CreateWorkspaceParams{
		OwnerUserID: ownerID,
		Name:        "carrier workspace",
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateWorkspace() on carrier error = %v", err)
	}
	// Same-transaction visibility ...
	var inside int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM workspaces WHERE public_id = $1`, created.PublicID).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count workspaces inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("workspace rows visible inside carrier = %d, want 1", inside)
	}
	// ... invisible outside before the carrier commits.
	if _, err := store.GetWorkspace(ctx, created.PublicID); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("GetWorkspace() saw the workspace before the carrier committed, want not found")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	got, err := store.GetWorkspace(ctx, created.PublicID)
	if err != nil {
		t.Fatalf("GetWorkspace() after carrier commit error = %v", err)
	}
	if got.Name != "carrier workspace" {
		t.Fatalf("workspace name after carrier commit = %q, want %q", got.Name, "carrier workspace")
	}
}

func TestCreateWorkspaceRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	store := NewStore(pool)
	ctx := context.Background()
	const ownerID = 4302
	if _, err := pool.Exec(ctx, `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`, ownerID, fmt.Sprintf("carrier-owner-%d", ownerID), fmt.Sprintf("C%05d", ownerID)); err != nil {
		t.Fatalf("seed workspace owner: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	created, err := store.CreateWorkspace(carrier, tenancy.CreateWorkspaceParams{
		OwnerUserID: ownerID,
		Name:        "doomed workspace",
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateWorkspace() on carrier error = %v", err)
	}
	// The business fails after the tenancy write: the guard rolls the
	// whole carrier back, workspace and membership included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if _, err := store.GetWorkspace(ctx, created.PublicID); err == nil {
		t.Fatal("GetWorkspace() saw the workspace after carrier rollback, want not found")
	}
}
