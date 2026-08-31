package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/testdb"
)

func TestBoardUploadIngestionTransitionsToReadyWithDerivativeOutbox(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "board-ingestion-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	upload := ingestion.Request{OwnerUserID: owner.ID, Origin: ingestion.BoardUploadFacts{BoardID: "board-ingestion"},
		StorageProfile: "builtin-local", URL: "/uploads/canvas/owner/board-ingestion/content/image.png", StorageKey: "uploads/canvas/owner/board-ingestion/content/image.png"}
	stored := storage.StoredUploadRef{
		URL: "/uploads/canvas/owner/board-ingestion/content/image.png", StorageKey: "uploads/canvas/owner/board-ingestion/content/image.png",
		StorageProfile: "builtin-local", MIMEType: "image/png",
	}
	handle, err := registry.Begin(ctx, upload)
	if err != nil {
		t.Fatalf("begin board upload: %v", err)
	}
	mediaAssetID := handle.AssetID
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil || asset.Status != "uploading" {
		t.Fatalf("uploading asset = %#v err=%v", asset, err)
	}
	if derivatives, err := registry.ListDerivatives(ctx, owner.ID, mediaAssetID); err != nil || len(derivatives) != 0 {
		t.Fatalf("uploading derivatives=%#v err=%v, want none", derivatives, err)
	}
	stored.Width = 640
	stored.Height = 480
	stored.SizeBytes = 2048
	stored.ChecksumSHA256 = "checksum"
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion("")); err != nil {
		t.Fatalf("complete board upload: %v", err)
	}
	asset, err = dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil || asset.Status != "ready" || asset.ChecksumSha256 != "checksum" {
		t.Fatalf("ready asset = %#v err=%v", asset, err)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, mediaAssetID)
	if err != nil {
		t.Fatal(err)
	}
	assertDerivativeTiers(t, derivatives, []string{"thumb", "preview", "processing-input"})
	for _, derivative := range derivatives {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivative_outbox WHERE derivative_id = $1`, derivative.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("derivative %d outbox count=%d err=%v", derivative.ID, count, err)
		}
	}
	if err := registry.EnsureDerivative(ctx, owner.ID, mediaAssetID, "large-preview"); err != nil {
		t.Fatalf("ensure large preview derivative: %v", err)
	}
	derivatives, err = registry.ListDerivatives(ctx, owner.ID, mediaAssetID)
	if err != nil {
		t.Fatal(err)
	}
	assertDerivativeTiers(t, derivatives, []string{"thumb", "preview", "processing-input", "large-preview"})
}

// This exercises the public ingestion/read seams rather than the concrete type. A
// workflow can rely on Begin and Complete committing the ready asset and its
// default derivative outbox together, without knowing the SQL implementation.
func TestRegistryContractCompletesBoardIngestionWithDerivativeOutbox(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "registry-contract-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	var registry interface {
		IngestionRegistry
		GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
	} = NewMediaAssetRegistry(db)
	request := ingestion.Request{
		OwnerUserID:    owner.ID,
		StorageProfile: "builtin-local",
		StorageKey:     "uploads/canvas/registry-contract/image.png",
		URL:            "/uploads/canvas/registry-contract/image.png",
		Origin:         ingestion.BoardUploadFacts{BoardID: "registry-contract"},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil {
		t.Fatalf("begin ingestion: %v", err)
	}
	completion := ingestion.Completion{
		StorageKey: request.StorageKey,
		URL:        request.URL,
		MIMEType:   "image/png",
		Width:      32,
		Height:     24,
		SizeBytes:  128,
	}
	if err := registry.Complete(ctx, handle, completion); err != nil {
		t.Fatalf("complete ingestion: %v", err)
	}

	asset, err := registry.GetByID(ctx, owner.ID, handle.AssetID)
	if err != nil || asset.Status != "ready" {
		t.Fatalf("ready asset = %#v err=%v", asset, err)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivative_outbox WHERE derivative_id IN (SELECT id FROM media_derivatives WHERE media_asset_id = $1)`, handle.AssetID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 3 {
		t.Fatalf("derivative outbox count=%d, want 3", outboxCount)
	}
}

func TestEnsureDerivativeKeepsOutboxWhenImmediateEnqueueFails(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "derivative-enqueue-fallback-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	request := ingestion.Request{
		OwnerUserID:    owner.ID,
		StorageProfile: "builtin-local", StorageKey: "uploads/canvas/fallback/image.png",
		URL:    "/uploads/canvas/fallback/image.png",
		Origin: ingestion.BoardUploadFacts{BoardID: "derivative-enqueue-fallback"},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil {
		t.Fatalf("begin upload: %v", err)
	}
	stored := storage.StoredUploadRef{
		URL: request.URL, StorageKey: request.StorageKey, StorageProfile: request.StorageProfile,
		MIMEType: "image/png", Width: 64, Height: 64, SizeBytes: 128, ChecksumSHA256: "fallback-checksum",
	}
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion("")); err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	registry.SetDerivativeEnqueuer(func(context.Context, int64) error {
		return errors.New("river unavailable")
	})
	if err := registry.EnsureDerivative(ctx, owner.ID, handle.AssetID, "large-preview"); err != nil {
		t.Fatalf("ensure derivative should tolerate immediate enqueue failure: %v", err)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivative_outbox WHERE derivative_id = (SELECT id FROM media_derivatives WHERE media_asset_id = $1 AND tier = 'large-preview')`, handle.AssetID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox count=%d, want 1", outboxCount)
	}
}

func TestLibraryMaterializationReusesManagedAssetAndQueuesDerivatives(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "library-materialization-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var folderID, libraryAssetID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO library_folders (owner_user_id, name) VALUES ($1, 'Saved') RETURNING id`, owner.ID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO library_assets (owner_user_id, folder_id, provider, source_id, source_url, preview_url, original_url) VALUES ($1, $2, 'unsplash', 'materialized', 'https://unsplash.com/photos/materialized', 'https://images.unsplash.com/materialized-preview', 'https://images.unsplash.com/materialized-original') RETURNING id`, owner.ID, folderID).Scan(&libraryAssetID); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	media := ingestion.Request{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/library/1/materialized/original", URL: "/uploads/library/1/materialized/original", Origin: ingestion.LibraryMaterializationFacts{LibraryAssetID: libraryAssetID, CanonicalSourceURL: "https://unsplash.com/photos/materialized", Provider: "unsplash", SourceID: "materialized"}}
	handle, err := registry.Begin(ctx, media)
	if err != nil {
		t.Fatalf("begin library materialization: %v", err)
	}
	mediaAssetID := handle.AssetID
	stored := storage.StoredUploadRef{URL: "/uploads/library/1/materialized/original.jpg", StorageKey: "uploads/library/1/materialized/original.jpg", StorageProfile: media.StorageProfile, MIMEType: "image/jpeg", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "library-checksum"}
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion("https://images.unsplash.com/materialized-original")); err != nil {
		t.Fatalf("complete library materialization: %v", err)
	}
	if _, found, err := registry.FindReadyLibraryMaterializedMedia(ctx, owner.ID, libraryAssetID); err != nil || !found {
		t.Fatalf("ready materialized asset found=%v err=%v", found, err)
	}
	asset, err := registry.GetByID(ctx, owner.ID, mediaAssetID)
	if err != nil {
		t.Fatalf("load materialized asset: %v", err)
	}
	if asset.StorageKey != stored.StorageKey || asset.Url != stored.URL {
		t.Fatalf("materialized storage locator = (%q, %q), want (%q, %q)", asset.StorageKey, asset.Url, stored.StorageKey, stored.URL)
	}
	if asset.SourceUrl != "https://images.unsplash.com/materialized-original" {
		t.Fatalf("media source URL = %q, want downloaded URL", asset.SourceUrl)
	}
	var provenance map[string]any
	if err := json.Unmarshal(asset.Metadata, &provenance); err != nil {
		t.Fatalf("decode materialization provenance: %v", err)
	}
	if provenance["source_url"] != "https://unsplash.com/photos/materialized" ||
		provenance["download_url"] != "https://images.unsplash.com/materialized-original" {
		t.Fatalf("materialization provenance = %s", asset.Metadata)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, mediaAssetID)
	if err != nil {
		t.Fatal(err)
	}
	assertDerivativeTiers(t, derivatives, []string{"thumb", "preview", "processing-input"})
}

func TestTaskResultIngestionTransitionsToReadyIdempotently(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "task-result-ingestion-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	request := ingestion.Request{
		OwnerUserID:    owner.ID,
		StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/task-result-ingestion/result-0.png", URL: "/uploads/tasks/1/task-result-ingestion/result-0.png", SourceURL: "https://provider.example/result.png",
		Origin: ingestion.TaskResultFacts{TaskID: "task-result-ingestion", ResultIndex: 0},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil || handle.ExistingReady {
		t.Fatalf("begin task result handle=%#v err=%v", handle, err)
	}
	assetID := handle.AssetID
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, assetID)
	if err != nil || asset.Status != "uploading" {
		t.Fatalf("uploading task result asset=%#v err=%v", asset, err)
	}
	stored := storage.StoredUploadRef{
		URL: request.URL, StorageKey: request.StorageKey, StorageProfile: request.StorageProfile,
		MIMEType: "image/png", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "task-result-checksum",
	}
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion(request.SourceURL)); err != nil {
		t.Fatalf("complete task result asset: %v", err)
	}
	asset, err = dbsqlc.New(db).GetMediaAssetByID(ctx, assetID)
	if err != nil || asset.Status != "ready" || asset.ChecksumSha256 != stored.ChecksumSHA256 {
		t.Fatalf("ready task result asset=%#v err=%v", asset, err)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, assetID)
	if err != nil {
		t.Fatal(err)
	}
	assertDerivativeTiers(t, derivatives, []string{"thumb", "preview", "processing-input"})
	for _, derivative := range derivatives {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivative_outbox WHERE derivative_id = $1`, derivative.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("derivative %d outbox count=%d err=%v", derivative.ID, count, err)
		}
	}
	retryHandle, err := registry.Begin(ctx, request)
	if err != nil || !retryHandle.ExistingReady || retryHandle.AssetID != assetID {
		t.Fatalf("retry task result handle=%#v err=%v, want asset %d ready", retryHandle, err, assetID)
	}
	if err := registry.Complete(ctx, retryHandle, stored.IngestionCompletion(request.SourceURL)); err != nil {
		t.Fatalf("retry complete task result asset: %v", err)
	}
	if derivatives, err = registry.ListDerivatives(ctx, owner.ID, assetID); err != nil {
		t.Fatal(err)
	} else {
		assertDerivativeTiers(t, derivatives, []string{"thumb", "preview", "processing-input"})
	}
}

func assertDerivativeTiers(t *testing.T, derivatives []dbsqlc.MediaDerivative, want []string) {
	t.Helper()
	got := make(map[string]struct{}, len(derivatives))
	for _, derivative := range derivatives {
		got[derivative.Tier] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("derivative tiers=%v, want %v", got, want)
	}
	for _, tier := range want {
		if _, ok := got[tier]; !ok {
			t.Fatalf("derivative tiers=%v, missing %q", got, tier)
		}
	}
}

func TestGenerationPreparationIngestionCompletesWithTypedMediaRef(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "generation-preparation-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	request := ingestion.Request{
		OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/prepare-action/input-0.png",
		URL: "/uploads/tasks/1/prepare-action/input-0.png", SourceURL: "https://source.example/input.png",
		Origin: ingestion.GenerationPreparationFacts{RefID: "action-1", Role: "input", InputIndex: 0},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil {
		t.Fatalf("begin generation preparation: %v", err)
	}
	if err := registry.Complete(ctx, handle, storage.StoredUploadRef{
		URL: request.URL, StorageKey: request.StorageKey, StorageProfile: request.StorageProfile,
		MIMEType: "image/png", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "prepared-checksum",
	}.IngestionCompletion(request.SourceURL)); err != nil {
		t.Fatalf("complete generation preparation: %v", err)
	}
	asset, err := registry.GetByID(ctx, owner.ID, handle.AssetID)
	if err != nil || asset.Status != "ready" || asset.ChecksumSha256 != "prepared-checksum" {
		t.Fatalf("prepared asset=%#v err=%v", asset, err)
	}
	var refs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_refs WHERE media_asset_id = $1 AND ref_type = 'prompt_reference'`, handle.AssetID).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("prepared media refs=%d err=%v, want 1", refs, err)
	}
}

func TestMediaAssetRegistryPersistsFailureForEveryOrigin(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "all-origins-failure-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var folderID, libraryAssetID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO library_folders (owner_user_id, name) VALUES ($1, 'Saved') RETURNING id`, owner.ID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO library_assets (owner_user_id, folder_id, provider, source_id, source_url, preview_url, original_url) VALUES ($1, $2, 'unsplash', 'failed-origin', 'https://unsplash.com/photos/failed-origin', 'https://images.unsplash.com/failed-preview', 'https://images.unsplash.com/failed-original') RETURNING id`, owner.ID, folderID).Scan(&libraryAssetID); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	requests := []ingestion.Request{
		{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/canvas/1/failure/image.png", URL: "/uploads/canvas/1/failure/image.png", Origin: ingestion.BoardUploadFacts{BoardID: "failure-board"}},
		{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/failure/input.png", URL: "/uploads/tasks/1/failure/input.png", Origin: ingestion.GenerationPreparationFacts{RefID: "failure-action", Role: "input", InputIndex: 0}},
		{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/failure/result.png", URL: "/uploads/tasks/1/failure/result.png", Origin: ingestion.TaskResultFacts{TaskID: "failure-task", ResultIndex: 0}},
		{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/library/1/failure/original.png", URL: "/uploads/library/1/failure/original.png", Origin: ingestion.LibraryMaterializationFacts{LibraryAssetID: libraryAssetID, Provider: "unsplash", SourceID: "failed-origin", CanonicalSourceURL: "https://unsplash.com/photos/failed-origin"}},
	}
	for _, request := range requests {
		t.Run(string(request.Origin.IngestionOrigin()), func(t *testing.T) {
			handle, err := registry.Begin(ctx, request)
			if err != nil {
				t.Fatalf("begin %s: %v", request.Origin.IngestionOrigin(), err)
			}
			if err := registry.Fail(ctx, handle, sql.ErrConnDone); err != nil {
				t.Fatalf("fail %s: %v", request.Origin.IngestionOrigin(), err)
			}
			asset, err := registry.GetByID(ctx, owner.ID, handle.AssetID)
			if err != nil || asset.Status != "failed" || !strings.Contains(string(asset.Metadata), sql.ErrConnDone.Error()) {
				t.Fatalf("failed %s asset=%#v err=%v", request.Origin.IngestionOrigin(), asset, err)
			}
		})
	}
}

func TestMediaAssetRegistryRegistersBoardUploadForInitialReclamation(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "board-upload-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatalf("create upload owner: %v", err)
	}
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	registry := NewMediaAssetRegistry(db)
	registry.now = func() time.Time { return now }

	upload := ingestion.Request{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/canvas/OWNER/shared-board/content/image.png", URL: "https://media.example.com/uploads/canvas/OWNER/shared-board/content/image.png", Origin: ingestion.BoardUploadFacts{BoardID: "shared-board"}}
	stored := storage.StoredUploadRef{
		URL:            "https://media.example.com/uploads/canvas/OWNER/shared-board/content/image.png",
		StorageKey:     "uploads/canvas/OWNER/shared-board/content/image.png",
		StorageProfile: "builtin-local",
		MIMEType:       "image/png",
		Width:          640,
		Height:         480,
		SizeBytes:      2048,
	}
	handle, err := registry.Begin(ctx, upload)
	if err != nil {
		t.Fatalf("begin board upload: %v", err)
	}
	mediaAssetID := handle.AssetID
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion("")); err != nil {
		t.Fatalf("complete board upload: %v", err)
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
	for name, source := range map[string]string{
		"provider URL":  asset.Url,
		"relative path": "/" + asset.StorageKey,
	} {
		t.Run("reusable "+name, func(t *testing.T) {
			gotID, found, err := registry.FindReusableGenerationMedia(ctx, owner.ID, source)
			if err != nil {
				t.Fatalf("FindReusableGenerationMedia returned error: %v", err)
			}
			if !found || gotID != mediaAssetID {
				t.Fatalf("reusable asset = %d, %v, want %d, true", gotID, found, mediaAssetID)
			}
		})
	}
	if gotID, found, err := registry.FindReusableGenerationMedia(ctx, owner.ID+1, asset.Url); err != nil || found || gotID != 0 {
		t.Fatalf("other owner reusable asset = %d, %v, %v, want 0, false, nil", gotID, found, err)
	}

	for name, lookup := range map[string]storage.StoredUploadLookup{
		"AssetID": {AssetID: mediaAssetID},
		"URL":     {URL: asset.Url, ObjectKey: "unused"},
		"storage key": {
			URL:       "https://retired-cdn.example.com/uploads/canvas/OWNER/shared-board/content/image.png",
			ObjectKey: asset.StorageKey,
		},
	} {
		t.Run(name, func(t *testing.T) {
			location, found, err := registry.LocateStoredUpload(ctx, lookup)
			if err != nil {
				t.Fatalf("LocateStoredUpload returned error: %v", err)
			}
			if !found {
				t.Fatal("LocateStoredUpload did not find registered asset")
			}
			if location.StorageProfileID != asset.StorageProfileID || location.StorageKey != asset.StorageKey {
				t.Fatalf("location = %#v, want profile/key from asset", location)
			}
		})
	}
}
