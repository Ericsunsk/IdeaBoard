package preparation

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/ingestion"
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
	beginRequest    ingestion.Request
	completedHandle ingestion.Handle
	completion      ingestion.Completion
	failedHandle    ingestion.Handle
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

func (r *reusableMediaRegistry) Begin(_ context.Context, request ingestion.Request) (ingestion.Handle, error) {
	r.beginRequest = request
	return ingestion.Handle{
		AssetID: r.registeredID, OwnerUserID: request.OwnerUserID, StorageProfile: request.StorageProfile,
		StorageKey: request.StorageKey, Origin: request.Origin,
	}, nil
}

func (r *reusableMediaRegistry) Complete(_ context.Context, handle ingestion.Handle, completion ingestion.Completion) error {
	r.completedHandle = handle
	r.completion = completion
	return nil
}

func (r *reusableMediaRegistry) Fail(_ context.Context, handle ingestion.Handle, _ error) error {
	r.failedHandle = handle
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
	if registry.beginRequest.StorageKey == "" || store.storeOption.ObjectKey != registry.beginRequest.StorageKey {
		t.Fatalf("stable storage key begin=%q store=%q", registry.beginRequest.StorageKey, store.storeOption.ObjectKey)
	}
	if registry.completedHandle.AssetID != 99 || registry.completion.ChecksumSHA256 != "" {
		t.Fatalf("completed handle=%#v completion=%#v", registry.completedHandle, registry.completion)
	}
	if prepared.Media[0].AssetID != 99 || prepared.Media[0].Source != "" {
		t.Fatalf("prepared media = %#v", prepared.Media[0])
	}
}

func TestPrepareGenerationMediaRetryReusesFailedAssetAndStableKey(t *testing.T) {
	store := &recordingMediaStore{}
	registry := &reusableMediaRegistry{ingesting: assets.PreparedGenerationMedia{
		AssetID: 77, StorageProfile: "profile-1", StorageKey: "uploads/tasks/1/prepare-action-1/input-0",
	}}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 1, "action-1", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "https://source.example/input.png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registry.beginRequest.OwnerUserID != 0 {
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
	if err == nil || registry.failedHandle.AssetID != 88 {
		t.Fatalf("err=%v failed handle=%#v", err, registry.failedHandle)
	}
}
