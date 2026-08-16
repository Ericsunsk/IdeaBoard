package preparation

import (
	"context"
	"testing"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/tasking/tasks"
)

type recordingMediaStore struct {
	storeCalls int
}

func (s *recordingMediaStore) StoreMediaRef(context.Context, string, storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	s.storeCalls++
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
}

func (*reusableMediaRegistry) FindPreparedGenerationMedia(context.Context, int64, string, string, int) (assets.PreparedGenerationMedia, bool, error) {
	return assets.PreparedGenerationMedia{}, false, nil
}

func (r *reusableMediaRegistry) FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error) {
	return r.reusableAssetID, r.reusableAssetID > 0, nil
}

func (r *reusableMediaRegistry) RegisterPreparedGenerationMedia(context.Context, assets.PreparedGenerationMedia) (int64, error) {
	return r.registeredID, nil
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
