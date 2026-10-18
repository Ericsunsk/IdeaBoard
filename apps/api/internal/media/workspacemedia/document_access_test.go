package workspacemedia

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func TestValidateDocumentAssetsPreconditions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateDocumentAssets(ctx, nil, "ws_test", 1, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation = %v", err)
	}

	if err := ValidateDocumentAssets(context.Background(), nil, "ws_test", 1, nil, nil); !errors.Is(err, errNotConfigured) {
		t.Fatalf("unconfigured validation = %v", err)
	}
}

func TestValidateDocumentAssetsPostgres(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	ownerAsset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	memberAsset := pgUploadComplete(t, ctx, env.media, env.ws, env.member, "")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := dbsqlc.New(tx)
	access := &stubBoardAccess{}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{memberAsset.ID, ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged private asset mixed with owned asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.owner, access, []string{"asset_missing00000001"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, "ws_other", env.owner, access, []string{ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-workspace asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{memberAsset.ID}); err != nil {
		t.Fatalf("owned asset = %v", err)
	}
	const sourceBoard = "board_source00000001"
	access.roles = map[string]boards.AccessRole{sourceBoard: boards.RoleViewer}
	access.refs = map[string][]string{ownerAsset.ID: {sourceBoard}}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); err != nil {
		t.Fatalf("readable existing reference = %v", err)
	}
	delete(access.roles, sourceBoard)
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked existing reference = %v, want ErrNotFound", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE media_assets SET owner_user_id = $1 WHERE public_id = $2`, env.member, ownerAsset.ID); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); err != nil {
		t.Fatalf("uncommitted ownership from accepting transaction = %v", err)
	}
}
