package preparation

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testdb"
)

type recordingMediaStore struct {
	storeCalls  int
	storeError  error
	storeOption storage.StorageStoreOptions
	checksum    string
	deleteCalls int
	deletedKey  string
	profile     string
}

func (s *recordingMediaStore) PrepareUploadRef(_ context.Context, _ string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	return &storage.StoredUploadRef{URL: "/" + opts.ObjectKey, StorageKey: opts.ObjectKey, StorageProfile: s.storageProfile()}, nil
}

func (s *recordingMediaStore) StoreMediaRefAtProfile(_ context.Context, _ string, _ string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	s.storeCalls++
	s.storeOption = opts
	if s.storeError != nil {
		return nil, s.storeError
	}
	return &storage.StoredUploadRef{
		URL:            "https://media.example.com/" + opts.ObjectKey,
		StorageKey:     opts.ObjectKey,
		StorageProfile: s.storageProfile(),
		MIMEType:       "image/png",
		ChecksumSHA256: s.checksum,
	}, nil
}

func (s *recordingMediaStore) storageProfile() string {
	if s.profile != "" {
		return s.profile
	}
	return "profile-1"
}

func (s *recordingMediaStore) DeleteObjectByStorageProfile(_ context.Context, _ string, storageKey string) error {
	s.deleteCalls++
	s.deletedKey = storageKey
	return nil
}

type reusableMediaRegistry struct {
	reusableAssetID  int64
	registeredID     int64
	ingesting        assets.PreparedGenerationMedia
	beginRequest     ingestion.Request
	completedHandle  ingestion.Handle
	completion       ingestion.Completion
	failedHandle     ingestion.Handle
	checksumAssetID  int64
	checksumAssetIDs []int64
	checksumLookups  int
	completeError    error
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

func (r *reusableMediaRegistry) FindReadyOwnedMediaAssetIDByChecksum(context.Context, int64, string, string) (int64, bool, error) {
	assetID := r.checksumAssetID
	if r.checksumLookups < len(r.checksumAssetIDs) {
		assetID = r.checksumAssetIDs[r.checksumLookups]
	}
	r.checksumLookups++
	return assetID, assetID > 0, nil
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
	return r.completeError
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

func TestPrepareGenerationMediaReusesOwnedReadyAssetByChecksum(t *testing.T) {
	store := &recordingMediaStore{checksum: "same-content"}
	registry := &reusableMediaRegistry{registeredID: 99, checksumAssetID: 42}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 7, "action-2", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{
			InputIndex: 0,
			Role:       tasks.MediaRoleInput,
			Source:     "https://signed-storage.example/input.png?signature=rotating",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Media[0].AssetID; got != 42 {
		t.Fatalf("prepared asset ID = %d, want checksum-equivalent asset 42", got)
	}
	if registry.completedHandle.AssetID != 0 {
		t.Fatalf("duplicate provisional asset was completed: %#v", registry.completedHandle)
	}
	if registry.failedHandle.AssetID != 99 || store.deleteCalls != 1 || store.deletedKey != registry.beginRequest.StorageKey {
		t.Fatalf("duplicate cleanup failed: handle=%#v deleteCalls=%d deletedKey=%q want=%q", registry.failedHandle, store.deleteCalls, store.deletedKey, registry.beginRequest.StorageKey)
	}
}

func TestPrepareGenerationMediaRecoversChecksumCompletionRace(t *testing.T) {
	store := &recordingMediaStore{checksum: "same-content"}
	registry := &reusableMediaRegistry{
		registeredID:     99,
		checksumAssetIDs: []int64{0, 42},
		completeError: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "uq_media_assets_owned_checksum",
		},
	}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(t.Context(), 7, "action-race", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{
			InputIndex: 0,
			Role:       tasks.MediaRoleInput,
			Source:     "https://signed-storage.example/input.png?signature=rotating",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Media[0].AssetID; got != 42 {
		t.Fatalf("prepared asset ID = %d, want checksum conflict winner 42", got)
	}
	if registry.checksumLookups != 2 || registry.completedHandle.AssetID != 99 {
		t.Fatalf("race recovery lookups=%d completed=%#v", registry.checksumLookups, registry.completedHandle)
	}
	if registry.failedHandle.AssetID != 99 || store.deleteCalls != 1 || store.deletedKey != registry.beginRequest.StorageKey {
		t.Fatalf("race cleanup failed: handle=%#v deleteCalls=%d deletedKey=%q want=%q", registry.failedHandle, store.deleteCalls, store.deletedKey, registry.beginRequest.StorageKey)
	}
}

func TestPrepareGenerationMediaReusesReadyChecksumWithPostgres(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	owner := &users.User{DisplayName: "prepared-checksum-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := assets.NewMediaAssetRegistry(db)
	existingRequest := ingestion.Request{
		OwnerUserID: owner.ID, StorageProfile: "builtin-local",
		StorageKey: "uploads/tasks/1/prepare-variation/input-0.png",
		URL:        "/uploads/tasks/1/prepare-variation/input-0.png",
		SourceURL:  "https://signed-storage.example/input.png?signature=variation",
		Origin: ingestion.GenerationPreparationFacts{
			RefID: "variation-action", Role: tasks.MediaRoleInput, InputIndex: 0,
		},
	}
	existingHandle, err := registry.Begin(ctx, existingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Complete(ctx, existingHandle, storage.StoredUploadRef{
		URL: existingRequest.URL, StorageKey: existingRequest.StorageKey, StorageProfile: existingRequest.StorageProfile,
		MIMEType: "image/png", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "same-content",
	}.IngestionCompletion(existingRequest.SourceURL)); err != nil {
		t.Fatal(err)
	}

	store := &recordingMediaStore{checksum: "same-content", profile: "builtin-local"}
	prepared, err := NewService(store, registry).PrepareGenerationMedia(ctx, owner.ID, "cutout-action", &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{
			InputIndex: 0, Role: tasks.MediaRoleInput,
			Source: "https://signed-storage.example/input.png?signature=cutout",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Media[0].AssetID; got != existingHandle.AssetID {
		t.Fatalf("prepared asset ID = %d, want existing asset %d", got, existingHandle.AssetID)
	}
	var readyCount, failedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE status = 'ready'), COUNT(*) FILTER (WHERE status = 'failed') FROM media_assets WHERE owner_user_id = $1`, owner.ID).Scan(&readyCount, &failedCount); err != nil {
		t.Fatal(err)
	}
	if readyCount != 1 || failedCount != 1 || store.deleteCalls != 1 {
		t.Fatalf("ready=%d failed=%d deleteCalls=%d, want 1/1/1", readyCount, failedCount, store.deleteCalls)
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
