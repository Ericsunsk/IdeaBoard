package assets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/testdb"
)

type acceptingIntegrityVerifier struct{}

func (acceptingIntegrityVerifier) Verify(context.Context, string, string, string, string, int64) error {
	return nil
}

func newMediaAssetRegistryForTest(db *pgxpool.Pool) *MediaAssetRegistry {
	registry := NewMediaAssetRegistry(db)
	registry.SetIntegrityVerifier(acceptingIntegrityVerifier{})
	return registry
}

// setReadyProcessingInputMaterializerForTest wires a materializer fake that
// records a ready processing-input derivative row, mirroring what the
// production derivatives service persists after a successful render.
func setReadyProcessingInputMaterializerForTest(t *testing.T, db *pgxpool.Pool, registry *MediaAssetRegistry) {
	t.Helper()
	registry.SetProcessingInputMaterializer(func(ctx context.Context, assetID, ownerUserID int64) error {
		q := dbsqlc.New(db)
		asset, err := q.GetMediaAssetByID(ctx, assetID)
		if err != nil {
			return fmt.Errorf("load materialization asset: %w", err)
		}
		upserted, err := q.UpsertMediaDerivative(ctx, dbsqlc.UpsertMediaDerivativeParams{
			DerivativeID: fmt.Sprintf("test-processing-input-%d", assetID),
			MediaAssetID: assetID, OwnerUserID: ownerUserID,
			Tier: "processing-input", Format: "png",
		})
		if err != nil {
			return fmt.Errorf("upsert processing-input derivative: %w", err)
		}
		_, err = q.MarkMediaDerivativeReady(ctx, dbsqlc.MarkMediaDerivativeReadyParams{
			Format: "png", StorageKey: fmt.Sprintf("test-derivatives/%d/processing-input.png", assetID),
			MimeType: "image/png", Width: 32, Height: 32, SizeBytes: 128,
			ChecksumSha256:         "test-checksum",
			SourceObjectGeneration: asset.ObjectGeneration,
			ID:                     upserted.ID,
		})
		if err != nil {
			return fmt.Errorf("mark processing-input derivative ready: %w", err)
		}
		return nil
	})
}

func TestBoardUploadIngestionTransitionsToReadyWithProcessingInput(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "board-ingestion-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	setReadyProcessingInputMaterializerForTest(t, db, registry)
	upload := ingestion.Request{OwnerUserID: owner.ID, Origin: ingestion.BoardUploadFacts{BoardID: "board-ingestion"},
		StorageProfile: "builtin-local", URL: "/uploads/canvas/owner/board-ingestion/content/image.png", StorageKey: "uploads/canvas/owner/board-ingestion/content/image.png"}
	stored := ports.StoredUploadRef{
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
	// Display tiers resolve on demand through the image delivery layer, so
	// only the provider-neutral processing-input materializes synchronously.
	assertDerivativeTiers(t, derivatives, []string{"processing-input"})
}

func TestCompleteMarksAssetFailedWhenProcessingInputMaterializationFails(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "processing-input-failure-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	registry.SetProcessingInputMaterializer(func(context.Context, int64, int64) error {
		return fmt.Errorf("renderer unavailable")
	})
	request := ingestion.Request{
		OwnerUserID: owner.ID, StorageProfile: "builtin-local",
		StorageKey: "uploads/canvas/processing-input-failure/image.png",
		URL:        "/uploads/canvas/processing-input-failure/image.png",
		Origin:     ingestion.BoardUploadFacts{BoardID: "processing-input-failure"},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil {
		t.Fatalf("begin ingestion: %v", err)
	}
	err = registry.Complete(ctx, handle, ingestion.Completion{
		StorageKey: request.StorageKey, URL: request.URL, MIMEType: "image/png",
		Width: 32, Height: 24, SizeBytes: 128,
	})
	if err == nil || !strings.Contains(err.Error(), "materialize processing-input") {
		t.Fatalf("expected materialization failure, got %v", err)
	}
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, handle.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Status != "failed" {
		t.Fatalf("asset status = %q, want failed", asset.Status)
	}
}

func TestRegistryContractCompletesBoardIngestion(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "registry-contract-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	concreteRegistry := newMediaAssetRegistryForTest(db)
	setReadyProcessingInputMaterializerForTest(t, db, concreteRegistry)
	var registry interface {
		IngestionRegistry
		GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
		ListDerivatives(context.Context, int64, int64) ([]dbsqlc.MediaDerivative, error)
	} = concreteRegistry
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
	mismatched := completion
	mismatched.StorageKey = "uploads/canvas/registry-contract/other.png"
	if err := registry.Complete(ctx, handle, mismatched); err == nil || !strings.Contains(err.Error(), "storage key does not match") {
		t.Fatalf("expected completion storage key mismatch, got %v", err)
	}
	if err := registry.Complete(ctx, handle, completion); err != nil {
		t.Fatalf("complete ingestion: %v", err)
	}

	asset, err := registry.GetByID(ctx, owner.ID, handle.AssetID)
	if err != nil || asset.Status != "ready" {
		t.Fatalf("ready asset = %#v err=%v", asset, err)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, handle.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	assertDerivativeTiers(t, derivatives, []string{"processing-input"})
}

func TestLibraryMaterializationReusesManagedAssetAndMaterializesProcessingInput(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "library-materialization-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var folderID, libraryAssetID int64
	if err := db.QueryRow(ctx, `INSERT INTO library_folders (owner_user_id, name) VALUES ($1, 'Saved') RETURNING id`, owner.ID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO library_assets (owner_user_id, folder_id, provider, source_id, source_url, preview_url, original_url) VALUES ($1, $2, 'unsplash', 'materialized', 'https://unsplash.com/photos/materialized', 'https://images.unsplash.com/materialized-preview', 'https://images.unsplash.com/materialized-original') RETURNING id`, owner.ID, folderID).Scan(&libraryAssetID); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	setReadyProcessingInputMaterializerForTest(t, db, registry)
	media := ingestion.Request{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/library/1/materialized/original", URL: "/uploads/library/1/materialized/original", Origin: ingestion.LibraryMaterializationFacts{LibraryAssetID: libraryAssetID, CanonicalSourceURL: "https://unsplash.com/photos/materialized", Provider: "unsplash", SourceID: "materialized"}}
	handle, err := registry.Begin(ctx, media)
	if err != nil {
		t.Fatalf("begin library materialization: %v", err)
	}
	mediaAssetID := handle.AssetID
	stored := ports.StoredUploadRef{URL: "/uploads/library/1/materialized/original", StorageKey: "uploads/library/1/materialized/original", StorageProfile: media.StorageProfile, MIMEType: "image/jpeg", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "library-checksum"}
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
	assertDerivativeTiers(t, derivatives, []string{"processing-input"})
}

func TestTaskResultIngestionTransitionsToReadyIdempotently(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "task-result-ingestion-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	setReadyProcessingInputMaterializerForTest(t, db, registry)
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
	stored := ports.StoredUploadRef{
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
	assertDerivativeTiers(t, derivatives, []string{"processing-input"})
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
		assertDerivativeTiers(t, derivatives, []string{"processing-input"})
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
	owner := &identitydomain.User{DisplayName: "generation-preparation-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	request := ingestion.Request{
		OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/prepare-action/input-0.png",
		URL: "/uploads/tasks/1/prepare-action/input-0.png", SourceURL: "https://source.example/input.png",
		Origin: ingestion.GenerationPreparationFacts{RefID: "action-1", Role: "input", InputIndex: 0},
	}
	handle, err := registry.Begin(ctx, request)
	if err != nil {
		t.Fatalf("begin generation preparation: %v", err)
	}
	if err := registry.Complete(ctx, handle, ports.StoredUploadRef{
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
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM media_refs WHERE media_asset_id = $1 AND ref_type = 'prompt_reference'`, handle.AssetID).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("prepared media refs=%d err=%v, want 1", refs, err)
	}
}

func TestMediaAssetRegistryPersistsFailureForEveryOrigin(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "all-origins-failure-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var folderID, libraryAssetID int64
	if err := db.QueryRow(ctx, `INSERT INTO library_folders (owner_user_id, name) VALUES ($1, 'Saved') RETURNING id`, owner.ID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO library_assets (owner_user_id, folder_id, provider, source_id, source_url, preview_url, original_url) VALUES ($1, $2, 'unsplash', 'failed-origin', 'https://unsplash.com/photos/failed-origin', 'https://images.unsplash.com/failed-preview', 'https://images.unsplash.com/failed-original') RETURNING id`, owner.ID, folderID).Scan(&libraryAssetID); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
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
			if err := registry.Fail(ctx, handle, errors.New("conn done")); err != nil {
				t.Fatalf("fail %s: %v", request.Origin.IngestionOrigin(), err)
			}
			asset, err := registry.GetByID(ctx, owner.ID, handle.AssetID)
			if err != nil || asset.Status != "failed" || !strings.Contains(string(asset.Metadata), errors.New("conn done").Error()) {
				t.Fatalf("failed %s asset=%#v err=%v", request.Origin.IngestionOrigin(), asset, err)
			}
		})
	}
}

func TestMediaAssetRegistryRegistersBoardUploadForInitialReclamation(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "board-upload-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatalf("create upload owner: %v", err)
	}
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	registry := newMediaAssetRegistryForTest(db)
	registry.now = func() time.Time { return now }

	upload := ingestion.Request{OwnerUserID: owner.ID, StorageProfile: "builtin-local", StorageKey: "uploads/canvas/OWNER/shared-board/content/image.png", URL: "https://media.example.com/uploads/canvas/OWNER/shared-board/content/image.png", Origin: ingestion.BoardUploadFacts{BoardID: "shared-board"}}
	stored := ports.StoredUploadRef{
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

	for name, lookup := range map[string]ports.StoredUploadLookup{
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

func TestRetireMissingOwnedMediaAssetReleasesChecksumSlot(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &identitydomain.User{DisplayName: "retire-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := newMediaAssetRegistryForTest(db)
	setReadyProcessingInputMaterializerForTest(t, db, registry)
	upload := ingestion.Request{OwnerUserID: owner.ID, Origin: ingestion.BoardUploadFacts{BoardID: "board-retire"},
		StorageProfile: "builtin-local", URL: "/uploads/canvas/owner/board-retire/content/image.png", StorageKey: "uploads/canvas/owner/board-retire/content/image.png"}
	handle, err := registry.Begin(ctx, upload)
	if err != nil {
		t.Fatalf("begin board upload: %v", err)
	}
	stored := ports.StoredUploadRef{
		URL: "/uploads/canvas/owner/board-retire/content/image.png", StorageKey: "uploads/canvas/owner/board-retire/content/image.png",
		StorageProfile: "builtin-local", MIMEType: "image/png", Width: 64, Height: 64, SizeBytes: 128, ChecksumSHA256: "retire-checksum",
	}
	if err := registry.Complete(ctx, handle, stored.IngestionCompletion("")); err != nil {
		t.Fatalf("complete board upload: %v", err)
	}
	if _, found, err := registry.FindReadyOwnedMediaAssetByChecksum(ctx, owner.ID, "builtin-local", "retire-checksum"); err != nil || !found {
		t.Fatalf("dedupe lookup before retire found=%v err=%v", found, err)
	}
	if err := registry.RetireMissingOwnedMediaAsset(ctx, owner.ID, handle.AssetID); err != nil {
		t.Fatalf("retire missing asset: %v", err)
	}
	if _, found, err := registry.FindReadyOwnedMediaAssetByChecksum(ctx, owner.ID, "builtin-local", "retire-checksum"); err != nil || found {
		t.Fatalf("dedupe lookup after retire found=%v err=%v, want miss", found, err)
	}
	// Retiring twice stays a no-op success so concurrent healers converge.
	if err := registry.RetireMissingOwnedMediaAsset(ctx, owner.ID, handle.AssetID); err != nil {
		t.Fatalf("second retire: %v", err)
	}
	if err := registry.RetireMissingOwnedMediaAsset(ctx, 0, handle.AssetID); err == nil {
		t.Fatal("retire with invalid owner should fail")
	}
}
