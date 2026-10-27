package workspacemedia

import (
	"context"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func TestCompensationArchivePostgresProtectsDurableReferences(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	// Provenance survives raw Task retention and must still protect its asset.
	if _, err := pool.Exec(ctx, `INSERT INTO media_asset_task_origins
 (workspace_id, task_public_id, output_index, media_asset_id, submitter_user_id)
 SELECT workspace_id, 'task_retained_origin', 0, internal_id, owner_user_id FROM media_assets WHERE public_id=$1`, asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.media.ArchiveUnreferencedAsset(ctx, env.ws, asset.ID); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("archive referenced asset = %v, want conflict", err)
	}
	current, err := env.media.GetAsset(ctx, env.ws, asset.ID)
	if err != nil || current.IsArchived() {
		t.Fatalf("referenced asset changed: %+v, %v", current, err)
	}
	unreferenced := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	for range 2 {
		archived, err := env.media.ArchiveUnreferencedAsset(ctx, env.ws, unreferenced.ID)
		if err != nil || !archived.IsArchived() {
			t.Fatalf("archive unreferenced = %+v, %v", archived, err)
		}
	}
}

func TestCompensationArchiveWaitsForConcurrentReference(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// This is the exact media lock used by output association before it writes
	// task_outputs/provenance, not a snapshot-only read of an unreferenced asset.
	if _, err := dbsqlc.New(tx).GetWorkspaceMediaAssetID(ctx, dbsqlc.GetWorkspaceMediaAssetIDParams{PublicID: asset.ID, WorkspacePublicID: env.ws}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		_, err := env.media.ArchiveUnreferencedAsset(cleanupCtx, env.ws, asset.ID)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("compensation bypassed pending association lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if _, err := tx.Exec(ctx, `INSERT INTO media_asset_task_origins
 (workspace_id, task_public_id, output_index, media_asset_id, submitter_user_id)
 SELECT workspace_id, 'task_concurrent_origin', 0, internal_id, owner_user_id FROM media_assets WHERE public_id=$1`, asset.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrStateConflict) {
			t.Fatalf("concurrent reference archive = %v, want conflict", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("compensation did not resume after association commit")
	}
	current, err := env.media.GetAsset(ctx, env.ws, asset.ID)
	if err != nil || current.IsArchived() {
		t.Fatalf("concurrently referenced asset changed: %+v, %v", current, err)
	}
}
