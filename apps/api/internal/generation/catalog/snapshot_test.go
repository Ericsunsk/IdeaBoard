package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func randomSuffix(t *testing.T, nbytes int) string {
	t.Helper()
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random suffix failed: %v", err)
	}
	return hex.EncodeToString(buf)
}

// seedSnapshotTask inserts a minimal task row (user, workspace, workspace
// billing account, pending task) and returns its public id. Snapshot
// scope is the task public id; workspace scoping stays at the API layer.
func seedSnapshotTask(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	var userID int64
	if err := db.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('snapshot-test-user', $1, 'user', 1) RETURNING id`,
		"S"+strings.ToUpper(randomSuffix(t, 3))[:5],
	).Scan(&userID); err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	var workspaceID int64
	workspacePublicID := "ws_snap" + randomSuffix(t, 6)
	if err := db.QueryRow(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'snapshot-test-ws') RETURNING internal_id`,
		workspacePublicID, userID,
	).Scan(&workspaceID); err != nil {
		t.Fatalf("seed workspace failed: %v", err)
	}
	var billingAccountID int64
	if err := db.QueryRow(ctx,
		`INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, $2) RETURNING internal_id`,
		"ba_snap"+randomSuffix(t, 6), workspaceID,
	).Scan(&billingAccountID); err != nil {
		t.Fatalf("seed billing account failed: %v", err)
	}
	taskPublicID := "task_snap" + randomSuffix(t, 6)
	_, err := dbsqlc.New(db).CreateWorkspaceTask(ctx, dbsqlc.CreateWorkspaceTaskParams{
		PublicID:         taskPublicID,
		WorkspaceID:      workspaceID,
		SubmitterUserID:  userID,
		BillingAccountID: billingAccountID,
		Service:          "generation",
		WorkflowInput:    []byte(`{"snapshot":"test"}`),
		Priority:         2,
	})
	if err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	return taskPublicID
}

// mintSnapshot builds a genuine resolver snapshot over the catalog so
// the round-trip covers the real selection hash, not a hand-made fact.
func mintSnapshot(t *testing.T, db *pgxpool.Pool, service, resolution string) routing.RoutingSnapshot {
	t.Helper()
	resolver := routing.NewRoutingResolver(NewRepository(db))
	snap, err := resolver.Resolve(context.Background(), service, resolution)
	if err != nil {
		t.Fatalf("Resolve(%q, %q) failed: %v", service, resolution, err)
	}
	return snap
}

func seedPool(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	repo := NewRepository(db)
	if _, err := repo.ReplacePool(context.Background(), "generation", 0, []PoolEntryInput{
		{Provider: "a", Model: "first", Enabled: true, Resolutions: []string{"1k", "2k"}},
		{Provider: "b", Model: "second", Enabled: true, Resolutions: []string{"2k", "4k"}},
	}); err != nil {
		t.Fatalf("seed pool failed: %v", err)
	}
}

func TestSnapshotSaveLoadRoundTrip(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	seedPool(t, db)
	taskID := seedSnapshotTask(t, db)
	snap := mintSnapshot(t, db, "generation", "2k")
	if len(snap.EligibleModels) != 2 {
		t.Fatalf("eligible = %v, want both models for 2k", snap.EligibleModels)
	}

	if err := SaveRoutingSnapshot(ctx, dbsqlc.New(db), taskID, snap); err != nil {
		t.Fatalf("SaveRoutingSnapshot failed: %v", err)
	}
	loaded, err := LoadRoutingSnapshot(ctx, dbsqlc.New(db), taskID)
	if err != nil {
		t.Fatalf("LoadRoutingSnapshot failed: %v", err)
	}
	if loaded.Service != snap.Service ||
		loaded.RequestedResolution != snap.RequestedResolution ||
		loaded.SelectedModel != snap.SelectedModel ||
		loaded.RetryCandidate != snap.RetryCandidate ||
		loaded.SelectionSeed != snap.SelectionSeed ||
		loaded.PoolVersion != snap.PoolVersion ||
		len(loaded.EligibleModels) != len(snap.EligibleModels) {
		t.Fatalf("loaded = %+v, want %+v", loaded, snap)
	}
	for i := range loaded.EligibleModels {
		if loaded.EligibleModels[i] != snap.EligibleModels[i] {
			t.Fatalf("eligible[%d] = %q, want %q", i, loaded.EligibleModels[i], snap.EligibleModels[i])
		}
	}
}

func TestSnapshotSaveInsideSubmitTransaction(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	seedPool(t, db)
	taskID := seedSnapshotTask(t, db)
	snap := mintSnapshot(t, db, "generation", "1k")

	// The submit transaction pins the snapshot with its own Queries;
	// a concurrent reader outside the tx sees it only after commit.
	if err := dbutil.WithTxDBTX(ctx, db, func(q *dbsqlc.Queries, _ pgx.Tx) error {
		return SaveRoutingSnapshot(ctx, q, taskID, snap)
	}); err != nil {
		t.Fatalf("tx SaveRoutingSnapshot failed: %v", err)
	}
	loaded, err := LoadRoutingSnapshot(ctx, dbsqlc.New(db), taskID)
	if err != nil {
		t.Fatalf("LoadRoutingSnapshot failed: %v", err)
	}
	if loaded.SelectedModel != snap.SelectedModel || loaded.PoolVersion != snap.PoolVersion {
		t.Fatalf("loaded = %+v, want %+v", loaded, snap)
	}
}

func TestSnapshotLoadMissing(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	q := dbsqlc.New(db)

	// Unknown task.
	if _, err := LoadRoutingSnapshot(ctx, q, "task_missing"); !errors.Is(err, ErrRoutingSnapshotMissing) {
		t.Fatalf("unknown task err = %v, want ErrRoutingSnapshotMissing", err)
	}
	// Existing task with NULL snapshot (pre-pool row).
	taskID := seedSnapshotTask(t, db)
	if _, err := LoadRoutingSnapshot(ctx, q, taskID); !errors.Is(err, ErrRoutingSnapshotMissing) {
		t.Fatalf("null snapshot err = %v, want ErrRoutingSnapshotMissing", err)
	}
	// Saving to an unknown task fails the same way (zero rows).
	seedPool(t, db)
	snap := mintSnapshot(t, db, "generation", "1k")
	if err := SaveRoutingSnapshot(ctx, q, "task_missing", snap); !errors.Is(err, ErrRoutingSnapshotMissing) {
		t.Fatalf("save-unknown err = %v, want ErrRoutingSnapshotMissing", err)
	}
}

func TestSnapshotSaveRejectsUnreplayable(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	seedPool(t, db)
	taskID := seedSnapshotTask(t, db)
	snap := mintSnapshot(t, db, "generation", "2k")

	// Tampering with the pinned decision must fail at save: the stored
	// fact would no longer replay without the live pool.
	snap.SelectedModel = "c/tampered"
	if err := SaveRoutingSnapshot(ctx, dbsqlc.New(db), taskID, snap); err == nil {
		t.Fatal("tampered snapshot saved, want Verify failure")
	}
	if _, err := LoadRoutingSnapshot(ctx, dbsqlc.New(db), taskID); !errors.Is(err, ErrRoutingSnapshotMissing) {
		t.Fatalf("rejected save must leave NULL, load err = %v", err)
	}
}

func TestSnapshotLoadCorruptFailsClosed(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	taskID := seedSnapshotTask(t, db)
	// Non-object JSONB cannot even be stored (chk_tasks_routing_snapshot
	// defense in depth); plant a JSON object that fails snapshot
	// validation instead.
	if _, err := db.Exec(ctx, `UPDATE tasks SET routing_snapshot = '{"bogus": 1}'::jsonb WHERE public_id = $1`, taskID); err != nil {
		t.Fatalf("plant corrupt snapshot failed: %v", err)
	}
	_, err := LoadRoutingSnapshot(ctx, dbsqlc.New(db), taskID)
	if err == nil || errors.Is(err, ErrRoutingSnapshotMissing) {
		t.Fatalf("corrupt load err = %v, want corrupt/validation failure (not missing)", err)
	}
}

func TestSnapshotExactJSONShape(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	seedPool(t, db)
	taskID := seedSnapshotTask(t, db)
	snap := mintSnapshot(t, db, "generation", "2k")
	if err := SaveRoutingSnapshot(ctx, dbsqlc.New(db), taskID, snap); err != nil {
		t.Fatalf("SaveRoutingSnapshot failed: %v", err)
	}

	// Wave 2 reads this exact key set: no envelope, seed as 32-char
	// lowercase hex, version as number. Assert on raw JSONB keys so a
	// struct-tag drift breaks here, not in submit.
	var raw map[string]any
	if err := db.QueryRow(ctx, `SELECT routing_snapshot FROM tasks WHERE public_id = $1`, taskID).Scan(&raw); err != nil {
		// pgx may decode jsonb into []byte depending on the plan; fall
		// back to a text scan for the key assertion.
		var text string
		if err := db.QueryRow(ctx, `SELECT routing_snapshot::text FROM tasks WHERE public_id = $1`, taskID).Scan(&text); err != nil {
			t.Fatalf("read raw snapshot failed: %v", err)
		}
		for _, key := range []string{`"service"`, `"requested_resolution"`, `"eligible_models"`, `"selected_model"`, `"retry_candidate"`, `"selection_seed"`, `"pool_version"`} {
			if !containsKey(text, key) {
				t.Fatalf("raw snapshot %s misses key %s", text, key)
			}
		}
		return
	}
	for _, key := range []string{"service", "requested_resolution", "eligible_models", "selected_model", "retry_candidate", "selection_seed", "pool_version"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("raw snapshot misses key %q: %v", key, raw)
		}
	}
	if len(raw) != 7 {
		t.Fatalf("raw snapshot has %d keys, want exactly 7: %v", len(raw), raw)
	}
}

func containsKey(text, key string) bool {
	for i := 0; i+len(key) <= len(text); i++ {
		if text[i:i+len(key)] == key {
			return true
		}
	}
	return false
}
