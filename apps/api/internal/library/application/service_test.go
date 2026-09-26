package application

import (
	"context"
	"testing"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/infrastructure/postgres"
	"ideaboard/api/internal/library/ports"
)

// The production postgres Service must satisfy the application store
// seams (seam methods plus the concrete task-output/list operations).
var _ ManagedAssetStore = (*postgres.Service)(nil)
var _ MaterializationStore = (*postgres.Service)(nil)

type recordingStore struct {
	ports.WorkspaceLibraryStore
	calls []string
}

func (f *recordingStore) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error) {
	f.calls = append(f.calls, "CreateFolder")
	return &domain.V1Folder{WorkspacePublicID: workspacePublicID, PublicID: publicID, Name: name}, nil
}

func (f *recordingStore) SaveManagedMediaAsset(ctx context.Context, workspacePublicID, sourceID, mediaAssetID, publicID string) (*domain.V1Asset, bool, error) {
	f.calls = append(f.calls, "SaveManagedMediaAsset")
	return &domain.V1Asset{WorkspacePublicID: workspacePublicID, PublicID: publicID, SourceID: sourceID}, true, nil
}

func (f *recordingStore) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error) {
	f.calls = append(f.calls, "ListMaterializations")
	return []*domain.V1Materialization{}, nil
}

func TestFolderServiceDelegatesToStore(t *testing.T) {
	fake := &recordingStore{}
	svc := NewFolderService(fake)
	folder, err := svc.CreateFolder(context.Background(), "ws_x", "inspo", "lfolder_test0001")
	if err != nil {
		t.Fatal(err)
	}
	if folder.Name != "inspo" || len(fake.calls) != 1 || fake.calls[0] != "CreateFolder" {
		t.Fatalf("folder = %+v, calls = %v; want delegated CreateFolder", folder, fake.calls)
	}
}

func TestAssetServiceDelegatesManagedSave(t *testing.T) {
	fake := &recordingStore{}
	svc := NewAssetService(fake)
	asset, created, err := svc.SaveManagedMediaAsset(context.Background(), "ws_x", "task:t:0", "asset_x", "lasset_test0001")
	if err != nil || !created {
		t.Fatalf("managed save = %+v, %v, %v; want created asset", asset, created, err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "SaveManagedMediaAsset" {
		t.Fatalf("calls = %v; want delegated SaveManagedMediaAsset", fake.calls)
	}
}

func TestMaterializationServiceDelegatesList(t *testing.T) {
	fake := &recordingStore{}
	svc := NewMaterializationService(fake)
	all, err := svc.ListMaterializations(context.Background(), "ws_x")
	if err != nil || len(all) != 0 {
		t.Fatalf("list = %v, %v; want empty", all, err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "ListMaterializations" {
		t.Fatalf("calls = %v; want delegated ListMaterializations", fake.calls)
	}
}
