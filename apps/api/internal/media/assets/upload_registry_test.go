package assets

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
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
	upload := BoardUploadAsset{OwnerUserID: owner.ID, BoardID: "board-ingestion", Stored: storage.StoredUploadRef{
		URL: "/uploads/canvas/owner/board-ingestion/content/image.png", StorageKey: "uploads/canvas/owner/board-ingestion/content/image.png",
		StorageProfile: "builtin-local", MIMEType: "image/png",
	}}
	mediaAssetID, err := registry.BeginBoardUpload(ctx, upload)
	if err != nil {
		t.Fatalf("begin board upload: %v", err)
	}
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil || asset.Status != "uploading" {
		t.Fatalf("uploading asset = %#v err=%v", asset, err)
	}
	if derivatives, err := registry.ListDerivatives(ctx, owner.ID, mediaAssetID); err != nil || len(derivatives) != 0 {
		t.Fatalf("uploading derivatives=%#v err=%v, want none", derivatives, err)
	}
	upload.Stored.Width = 640
	upload.Stored.Height = 480
	upload.Stored.SizeBytes = 2048
	upload.Stored.ChecksumSHA256 = "checksum"
	if err := registry.CompleteBoardUpload(ctx, upload, mediaAssetID); err != nil {
		t.Fatalf("complete board upload: %v", err)
	}
	asset, err = dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil || asset.Status != "ready" || asset.ChecksumSha256 != "checksum" {
		t.Fatalf("ready asset = %#v err=%v", asset, err)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, mediaAssetID)
	if err != nil || len(derivatives) != 2 {
		t.Fatalf("ready derivatives=%#v err=%v, want 2", derivatives, err)
	}
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
	if err != nil || len(derivatives) != 3 {
		t.Fatalf("ensured derivatives=%#v err=%v, want 3", derivatives, err)
	}
}

func TestBoardUploadIngestionPersistsFailure(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "board-ingestion-failure-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	upload := BoardUploadAsset{OwnerUserID: owner.ID, BoardID: "board-ingestion-failure", Stored: storage.StoredUploadRef{
		URL: "/uploads/canvas/owner/board-ingestion-failure/content/image.png", StorageKey: "uploads/canvas/owner/board-ingestion-failure/content/image.png", StorageProfile: "builtin-local",
	}}
	mediaAssetID, err := registry.BeginBoardUpload(ctx, upload)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.FailBoardUpload(ctx, owner.ID, mediaAssetID, sql.ErrConnDone); err != nil {
		t.Fatal(err)
	}
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, mediaAssetID)
	if err != nil || asset.Status != "failed" || !strings.Contains(string(asset.Metadata), sql.ErrConnDone.Error()) {
		t.Fatalf("failed asset=%#v err=%v", asset, err)
	}
}

func TestTaskResultIngestionTransitionsToReadyIdempotently(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	owner := &users.User{DisplayName: "task-result-ingestion-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	registry := NewMediaAssetRegistry(db)
	ingestion := storage.TaskResultIngestion{
		OwnerUserID: owner.ID, TaskID: "task-result-ingestion", ResultIndex: 0,
		StorageProfile: "builtin-local", StorageKey: "uploads/tasks/1/task-result-ingestion/result-0.png",
		URL: "/uploads/tasks/1/task-result-ingestion/result-0.png", SourceURL: "https://provider.example/result.png",
	}
	assetID, existingReady, err := registry.BeginTaskResultIngestion(ctx, ingestion)
	if err != nil || existingReady {
		t.Fatalf("begin task result asset=%d existingReady=%v err=%v", assetID, existingReady, err)
	}
	asset, err := dbsqlc.New(db).GetMediaAssetByID(ctx, assetID)
	if err != nil || asset.Status != "uploading" {
		t.Fatalf("uploading task result asset=%#v err=%v", asset, err)
	}
	stored := storage.StoredUploadRef{
		URL: ingestion.URL, StorageKey: ingestion.StorageKey, StorageProfile: ingestion.StorageProfile,
		MIMEType: "image/png", Width: 640, Height: 480, SizeBytes: 2048, ChecksumSHA256: "task-result-checksum",
	}
	if err := registry.CompleteTaskResultIngestion(ctx, ingestion, stored); err != nil {
		t.Fatalf("complete task result asset: %v", err)
	}
	asset, err = dbsqlc.New(db).GetMediaAssetByID(ctx, assetID)
	if err != nil || asset.Status != "ready" || asset.ChecksumSha256 != stored.ChecksumSHA256 {
		t.Fatalf("ready task result asset=%#v err=%v", asset, err)
	}
	derivatives, err := registry.ListDerivatives(ctx, owner.ID, assetID)
	if err != nil || len(derivatives) != 2 {
		t.Fatalf("task result derivatives=%#v err=%v, want 2", derivatives, err)
	}
	for _, derivative := range derivatives {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivative_outbox WHERE derivative_id = $1`, derivative.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("derivative %d outbox count=%d err=%v", derivative.ID, count, err)
		}
	}
	assetIDRetry, existingReady, err := registry.BeginTaskResultIngestion(ctx, ingestion)
	if err != nil || !existingReady || assetIDRetry != assetID {
		t.Fatalf("retry task result asset=%d existingReady=%v err=%v, want %d,true,nil", assetIDRetry, existingReady, err, assetID)
	}
	if err := registry.CompleteTaskResultIngestion(ctx, ingestion, stored); err != nil {
		t.Fatalf("retry complete task result asset: %v", err)
	}
	if derivatives, err = registry.ListDerivatives(ctx, owner.ID, assetID); err != nil || len(derivatives) != 2 {
		t.Fatalf("retry task result derivatives=%#v err=%v, want 2", derivatives, err)
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

	upload := BoardUploadAsset{
		OwnerUserID: owner.ID,
		BoardID:     "shared-board",
		Stored: storage.StoredUploadRef{
			URL:            "https://media.example.com/uploads/canvas/OWNER/shared-board/content/image.png",
			StorageKey:     "uploads/canvas/OWNER/shared-board/content/image.png",
			StorageProfile: "builtin-local",
			MIMEType:       "image/png",
			Width:          640,
			Height:         480,
			SizeBytes:      2048,
		},
	}
	mediaAssetID, err := registry.BeginBoardUpload(ctx, upload)
	if err != nil {
		t.Fatalf("begin board upload: %v", err)
	}
	if err := registry.CompleteBoardUpload(ctx, upload, mediaAssetID); err != nil {
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
