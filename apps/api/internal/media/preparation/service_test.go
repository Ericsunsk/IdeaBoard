package preparation

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/tasking/tasks"
)

type recordingMediaStore struct {
	storeCalls  int
	storeError  error
	storeOption storage.StorageStoreOptions
}

func (*recordingMediaStore) PrepareUploadRef(_ context.Context, _ string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	return &storage.StoredUploadRef{URL: "/" + opts.ObjectKey, StorageKey: opts.ObjectKey, StorageProfile: "profile-1"}, nil
}

func (s *recordingMediaStore) StoreMediaRefAtProfile(_ context.Context, _ string, _ string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	s.storeCalls++
	s.storeOption = opts
	if s.storeError != nil {
		return nil, s.storeError
	}
	return &storage.StoredUploadRef{
		URL:            "https://media.example.com/uploads/tasks/1/prepared.png",
		StorageKey:     "uploads/tasks/1/prepared.png",
		StorageProfile: "profile-1",
		MIMEType:       "image/png",
	}, nil
}

func (*recordingMediaStore) DeleteObjectByStorageProfile(context.Context, string, string) error {
	return nil
}

type reusableMediaRegistry struct {
	reusableAssetID int64
	registeredID    int64
	ingesting       assets.PreparedGenerationMedia
	beginItem       assets.PreparedGenerationMedia
	completedItem   assets.PreparedGenerationMedia
	failedAssetID   int64
}

func (*reusableMediaRegistry) FindPreparedGenerationMedia(context.Context, int64, string, string, int) (assets.PreparedGenerationMedia, bool, error) {
	return assets.PreparedGenerationMedia{}, false, nil
}

func (r *reusableMediaRegistry) FindIngestingPreparedGenerationMedia(context.Context, int64, string, string, int) (assets.PreparedGenerationMedia, bool, error) {
	return r.ingesting, r.ingesting.AssetID > 0, nil
}

func (r *reusableMediaRegistry) FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error) {
	return r.reusableAssetID, r.reusableAssetID > 0, nil
}

func (r *reusableMediaRegistry) BeginPreparedGenerationMedia(_ context.Context, media assets.PreparedGenerationMedia) (int64, error) {
	r.beginItem = media
	return r.registeredID, nil
}

func (r *reusableMediaRegistry) CompletePreparedGenerationMedia(_ context.Context, media assets.PreparedGenerationMedia, _ int64) error {
	r.completedItem = media
	return nil
}

func (r *reusableMediaRegistry) FailPreparedGenerationMedia(_ context.Context, _ int64, assetID int64, _ error) error {
	r.failedAssetID = assetID
	return nil
}

func TestPrepareGenerationMediaReusesOwnedReadyAsset(t *testing.T) {
	store := &recordingMediaStore{}
	registry := &reusableMediaRegistry{reusableAssetID: 42, registeredID: 99}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 1, "action-1", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{
			InputIndex: 0,
			Role:       tasks.MediaRoleInput,
			Source:     "https://media.ideaboard.art/uploads/canvas/OWNER1/board-1/content/input.png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Media[0].AssetID; got != 42 {
		t.Fatalf("prepared asset ID = %d, want existing asset 42", got)
	}
	if store.storeCalls != 0 {
		t.Fatalf("storage copy calls = %d, want 0", store.storeCalls)
	}
}

func TestPrepareGenerationMediaRegistersBeforeStorageAndCompletesSameAsset(t *testing.T) {
	store := &recordingMediaStore{}
	registry := &reusableMediaRegistry{registeredID: 99}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 1, "action-1", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "https://source.example/input.png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registry.beginItem.StorageKey == "" || store.storeOption.ObjectKey != registry.beginItem.StorageKey {
		t.Fatalf("stable storage key begin=%q store=%q", registry.beginItem.StorageKey, store.storeOption.ObjectKey)
	}
	if registry.completedItem.AssetID != 99 || registry.completedItem.ChecksumSHA256 != "" {
		t.Fatalf("completed item = %#v", registry.completedItem)
	}
	if prepared.Media[0].AssetID != 99 || prepared.Media[0].Source != "" {
		t.Fatalf("prepared media = %#v", prepared.Media[0])
	}
}

func TestPrepareGenerationMediaRetryReusesFailedAssetAndStableKey(t *testing.T) {
	store := &recordingMediaStore{}
	registry := &reusableMediaRegistry{ingesting: assets.PreparedGenerationMedia{
		AssetID: 77, OwnerUserID: 1, StorageProfile: "profile-1", StorageKey: "uploads/tasks/1/prepare-action-1/input-0", Role: tasks.MediaRoleInput, InputIndex: 0, RefID: "action-1",
	}}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 1, "action-1", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "https://source.example/input.png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registry.beginItem.AssetID != 0 {
		t.Fatal("retry created a second asset")
	}
	if store.storeOption.ObjectKey != registry.ingesting.StorageKey || prepared.Media[0].AssetID != 77 {
		t.Fatalf("retry store=%q prepared=%#v", store.storeOption.ObjectKey, prepared.Media[0])
	}
}

func TestPrepareGenerationMediaMarksAssetFailedWhenStorageFails(t *testing.T) {
	store := &recordingMediaStore{storeError: errors.New("write failed")}
	registry := &reusableMediaRegistry{registeredID: 88}
	_, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 1, "action-1", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "https://source.example/input.png"}},
	})
	if err == nil || registry.failedAssetID != 88 {
		t.Fatalf("err=%v failed asset=%d", err, registry.failedAssetID)
	}
}
