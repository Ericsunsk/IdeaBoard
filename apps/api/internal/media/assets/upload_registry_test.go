package assets

import (
	"context"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/testdb"
)

func TestMediaAssetRegistryRegistersBoardUploadForInitialReclamation(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{Username: "board-upload-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatalf("create upload owner: %v", err)
	}
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	registry := NewMediaAssetRegistry(db)
	registry.now = func() time.Time { return now }

	mediaAssetID, err := registry.RegisterBoardUpload(ctx, BoardUploadAsset{
		OwnerUserID: owner.ID,
		BoardID:     "shared-board",
		Stored: storage.StoredUploadRef{
			URL:            "/uploads/canvas/OWNER/shared-board/content/image.png",
			StorageKey:     "uploads/canvas/OWNER/shared-board/content/image.png",
			StorageProfile: "builtin-local",
			MIMEType:       "image/png",
			Width:          640,
			Height:         480,
			SizeBytes:      2048,
		},
	})
	if err != nil {
		t.Fatalf("register board upload: %v", err)
	}

	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil {
		t.Fatalf("load registered media asset: %v", err)
	}
	if asset.OwnerUserID != owner.ID || asset.StorageProfileID != "builtin-local" {
		t.Fatalf("registered asset ownership/profile = %#v", asset)
	}
	if !asset.ReclaimAfter.Valid || !asset.ReclaimAfter.Time.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("reclaim_after = %v, want %v", asset.ReclaimAfter, now.Add(24*time.Hour))
	}
	if asset.ReclaimGeneration != 1 {
		t.Fatalf("reclaim_generation = %d, want 1", asset.ReclaimGeneration)
	}
}
