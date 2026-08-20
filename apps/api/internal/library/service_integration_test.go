package library

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestServiceSaveAssetIsIdempotentAndMovesExistingAsset(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	ctx := context.Background()
	input := integrationAsset("idempotent")

	first, err := service.SaveAsset(ctx, SaveAssetCommand{OwnerUserID: 1, Asset: input})
	if err != nil {
		t.Fatalf("first SaveAsset returned error: %v", err)
	}
	folder, err := service.CreateFolder(ctx, 1, "Travel")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	second, err := service.SaveAsset(ctx, SaveAssetCommand{OwnerUserID: 1, FolderSelection: &FolderSelection{Kind: FolderSelectionUser, ID: folder.ID}, Asset: input})
	if err != nil {
		t.Fatalf("second SaveAsset returned error: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second asset id = %d, want original id %d", second.ID, first.ID)
	}
	if second.FolderID != folder.ID {
		t.Fatalf("second folder id = %d, want %d", second.FolderID, folder.ID)
	}

	items, err := service.ListAssets(ctx, 1, FolderSelection{Kind: FolderSelectionUser, ID: folder.ID}, "", 10)
	if err != nil {
		t.Fatalf("ListAssets returned error: %v", err)
	}
	if len(items.Items) != 1 || items.Items[0].ID != first.ID {
		t.Fatalf("folder assets = %#v, want one original asset", items.Items)
	}
}

func TestServiceOwnerIsolation(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	ctx := context.Background()
	asset, err := service.SaveAsset(ctx, SaveAssetCommand{OwnerUserID: 1, Asset: integrationAsset("private")})
	if err != nil {
		t.Fatalf("SaveAsset returned error: %v", err)
	}
	if _, err := service.MoveAsset(ctx, 3, asset.ID, asset.FolderID); !errors.Is(err, ErrInvalidTargetFolder) {
		t.Fatalf("cross-owner MoveAsset error = %v, want %v", err, ErrInvalidTargetFolder)
	}
	if err := service.DeleteAsset(ctx, 3, asset.ID); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("cross-owner DeleteAsset error = %v, want %v", err, ErrAssetNotFound)
	}
	items, err := service.ListAssets(ctx, 3, FolderSelection{Kind: FolderSelectionAll}, "", 10)
	if err != nil {
		t.Fatalf("other owner ListAssets returned error: %v", err)
	}
	if len(items.Items) != 0 {
		t.Fatalf("other owner saw %#v", items.Items)
	}
}

func TestServiceDeleteFolderMovesAssetsToQuickSaves(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	ctx := context.Background()
	folder, err := service.CreateFolder(ctx, 1, "To Delete")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	asset, err := service.SaveAsset(ctx, SaveAssetCommand{OwnerUserID: 1, FolderSelection: &FolderSelection{Kind: FolderSelectionUser, ID: folder.ID}, Asset: integrationAsset("move-on-delete")})
	if err != nil {
		t.Fatalf("SaveAsset returned error: %v", err)
	}
	if err := service.DeleteFolder(ctx, 1, folder.ID); err != nil {
		t.Fatalf("DeleteFolder returned error: %v", err)
	}
	if _, err := service.repo.GetFolder(ctx, 1, folder.ID); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("deleted folder lookup error = %v, want %v", err, ErrFolderNotFound)
	}
	items, err := service.ListAssets(ctx, 1, FolderSelection{Kind: FolderSelectionAll}, "", 10)
	if err != nil {
		t.Fatalf("ListAssets returned error: %v", err)
	}
	if len(items.Items) != 1 || items.Items[0].ID != asset.ID || items.Items[0].FolderID == folder.ID {
		t.Fatalf("assets after folder delete = %#v, want asset in Quick Saves", items.Items)
	}
}

func TestServiceListAssetsUsesCursorPagination(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	ctx := context.Background()
	for index := 0; index < 3; index++ {
		if _, err := service.SaveAsset(ctx, SaveAssetCommand{OwnerUserID: 1, Asset: integrationAsset(fmt.Sprintf("page-%d", index))}); err != nil {
			t.Fatalf("SaveAsset(%d) returned error: %v", index, err)
		}
		time.Sleep(time.Millisecond)
	}
	first, err := service.ListAssets(ctx, 1, FolderSelection{Kind: FolderSelectionAll}, "", 2)
	if err != nil {
		t.Fatalf("first ListAssets returned error: %v", err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %#v, want two items and next cursor", first)
	}
	second, err := service.ListAssets(ctx, 1, FolderSelection{Kind: FolderSelectionAll}, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second ListAssets returned error: %v", err)
	}
	if len(second.Items) != 1 || second.HasMore || second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID {
		t.Fatalf("second page = %#v, want remaining item", second)
	}
}

func integrationAsset(id string) AssetInput {
	return AssetInput{
		Provider:    ProviderUnsplash,
		SourceID:    id,
		SourceURL:   "https://unsplash.com/photos/" + id,
		PreviewURL:  "https://images.unsplash.com/" + id + "?w=800",
		OriginalURL: "https://images.unsplash.com/" + id,
		Title:       id,
		Width:       1200,
		Height:      800,
	}
}
