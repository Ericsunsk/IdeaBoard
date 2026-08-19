package assets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/derivatives"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/media/storage"
)

type BoardUploadAsset struct {
	OwnerUserID int64
	BoardID     string
	Stored      storage.StoredUploadRef
}

type mediaAssetCompletion struct {
	ID             int64
	OwnerUserID    int64
	URL            string
	SourceURL      string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Metadata       []byte
}

func (r *MediaAssetRegistry) completeMediaAsset(ctx context.Context, completion mediaAssetCompletion) error {
	if r == nil || r.db == nil || completion.ID <= 0 || completion.OwnerUserID <= 0 {
		return fmt.Errorf("invalid media asset completion")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media asset completion: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	if _, err := q.MarkMediaAssetReady(ctx, dbsqlc.MarkMediaAssetReadyParams{
		ID: completion.ID, OwnerUserID: completion.OwnerUserID, Url: completion.URL, SourceUrl: completion.SourceURL,
		MimeType: completion.MIMEType, Width: int32(completion.Width), Height: int32(completion.Height), SizeBytes: completion.SizeBytes,
		ChecksumSha256: completion.ChecksumSHA256, Metadata: completion.Metadata,
	}); err != nil {
		return fmt.Errorf("complete media asset: %w", err)
	}
	derivativeIDs, err := derivatives.ScheduleDefault(ctx, q, completion.ID, completion.OwnerUserID)
	if err != nil {
		return fmt.Errorf("queue media asset derivatives: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media asset completion: %w", err)
	}
	committed = true
	r.enqueueDerivatives(ctx, derivativeIDs)
	return nil
}

func (r *MediaAssetRegistry) failMediaAsset(ctx context.Context, ownerUserID, assetID int64, cause error) error {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return fmt.Errorf("invalid media asset failure")
	}
	message := "media asset ingestion failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = cause.Error()
	}
	_, err := r.q.MarkMediaAssetFailed(ctx, dbsqlc.MarkMediaAssetFailedParams{ID: assetID, OwnerUserID: ownerUserID, ErrorMessage: message})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func (r *MediaAssetRegistry) BeginBoardUpload(ctx context.Context, upload BoardUploadAsset) (int64, error) {
	return r.insertBoardUpload(ctx, upload)
}

func (r *MediaAssetRegistry) CompleteBoardUpload(ctx context.Context, upload BoardUploadAsset, mediaAssetID int64) error {
	metadata, err := boardUploadMetadata(upload.BoardID, upload.Stored.Animated)
	if err != nil {
		return err
	}
	return r.completeMediaAsset(ctx, mediaAssetCompletion{
		ID: mediaAssetID, OwnerUserID: upload.OwnerUserID, URL: strings.TrimSpace(upload.Stored.URL),
		MIMEType: strings.TrimSpace(upload.Stored.MIMEType), Width: upload.Stored.Width, Height: upload.Stored.Height,
		SizeBytes: upload.Stored.SizeBytes, ChecksumSHA256: upload.Stored.ChecksumSHA256, Metadata: metadata,
	})
}

func (r *MediaAssetRegistry) FailBoardUpload(ctx context.Context, ownerUserID, mediaAssetID int64, cause error) error {
	return r.failMediaAsset(ctx, ownerUserID, mediaAssetID, cause)
}

type MediaAssetRegistry struct {
	db                 *sql.DB
	q                  *dbsqlc.Queries
	now                func() time.Time
	derivativeEnqueuer func(context.Context, int64) error
}

type DerivativeEnqueuer func(context.Context, int64) error

type PreparedGenerationMedia struct {
	AssetID        int64
	OwnerUserID    int64
	StorageProfile string
	StorageKey     string
	URL            string
	SourceURL      string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Animated       bool
	InputIndex     int
	Role           string
	RefID          string
}

func NewMediaAssetRegistry(db *sql.DB) *MediaAssetRegistry {
	return &MediaAssetRegistry{db: db, q: dbsqlc.New(db), now: time.Now}
}

func (r *MediaAssetRegistry) SetDerivativeEnqueuer(enqueuer DerivativeEnqueuer) {
	if r != nil {
		r.derivativeEnqueuer = enqueuer
	}
}

func (r *MediaAssetRegistry) GetByAssetID(ctx context.Context, ownerUserID int64, assetID string) (dbsqlc.MediaAsset, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || strings.TrimSpace(assetID) == "" {
		return dbsqlc.MediaAsset{}, fmt.Errorf("invalid media asset lookup")
	}
	return r.q.GetMediaAssetByAssetIDForOwner(ctx, dbsqlc.GetMediaAssetByAssetIDForOwnerParams{
		AssetID: assetID, OwnerUserID: ownerUserID,
	})
}

func (r *MediaAssetRegistry) GetByID(ctx context.Context, ownerUserID, assetID int64) (dbsqlc.MediaAsset, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return dbsqlc.MediaAsset{}, fmt.Errorf("invalid media asset lookup")
	}
	return r.q.GetMediaAssetByIDForOwner(ctx, dbsqlc.GetMediaAssetByIDForOwnerParams{ID: assetID, OwnerUserID: ownerUserID})
}

func (r *MediaAssetRegistry) ListDerivatives(ctx context.Context, ownerUserID, assetID int64) ([]dbsqlc.MediaDerivative, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return nil, fmt.Errorf("invalid media derivative lookup")
	}
	return r.q.ListMediaDerivativesByAsset(ctx, dbsqlc.ListMediaDerivativesByAssetParams{
		MediaAssetID: assetID, OwnerUserID: ownerUserID,
	})
}

func (r *MediaAssetRegistry) EnsureDerivative(ctx context.Context, ownerUserID, assetID int64, tier string) error {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return fmt.Errorf("invalid media derivative request")
	}
	_, err := derivatives.EnsureOnDemand(ctx, r.q, assetID, ownerUserID, tier)
	return err
}

func (r *MediaAssetRegistry) insertBoardUpload(ctx context.Context, upload BoardUploadAsset) (int64, error) {
	if r == nil || r.q == nil || upload.OwnerUserID <= 0 {
		return 0, fmt.Errorf("invalid board upload media asset")
	}
	boardID := strings.TrimSpace(upload.BoardID)
	storageProfileID := strings.TrimSpace(upload.Stored.StorageProfile)
	storageKey := strings.TrimSpace(upload.Stored.StorageKey)
	url := strings.TrimSpace(upload.Stored.URL)
	if boardID == "" || storageProfileID == "" || storageKey == "" || url == "" {
		return 0, fmt.Errorf("incomplete board upload media asset")
	}
	assetID, err := newUploadMediaAssetID()
	if err != nil {
		return 0, err
	}
	metadata, err := boardUploadMetadata(boardID, upload.Stored.Animated)
	if err != nil {
		return 0, fmt.Errorf("encode board upload media metadata: %w", err)
	}
	registeredAt := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin board upload media registration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	id, err := q.CreateBoardUploadMediaAsset(ctx, dbsqlc.CreateBoardUploadMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      upload.OwnerUserID,
		StorageProfileID: storageProfileID,
		StorageKey:       storageKey,
		Url:              url,
		MimeType:         strings.TrimSpace(upload.Stored.MIMEType),
		Width:            int32(upload.Stored.Width),
		Height:           int32(upload.Stored.Height),
		SizeBytes:        upload.Stored.SizeBytes,
		ChecksumSha256:   upload.Stored.ChecksumSHA256,
		Status:           "uploading",
		Metadata:         metadata,
		ReclaimAfter:     sql.NullTime{Time: registeredAt.Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("register board upload media asset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit board upload media registration: %w", err)
	}
	committed = true
	return id, nil
}

func boardUploadMetadata(boardID string, animated bool) ([]byte, error) {
	return json.Marshal(map[string]any{"origin": "board_upload", "board_id": strings.TrimSpace(boardID), "animated": animated})
}

func (r *MediaAssetRegistry) BeginPreparedGenerationMedia(ctx context.Context, media PreparedGenerationMedia) (int64, error) {
	return r.insertPreparedGenerationMedia(ctx, media)
}

func (r *MediaAssetRegistry) CompletePreparedGenerationMedia(ctx context.Context, media PreparedGenerationMedia, assetID int64) error {
	metadata, err := preparedMediaMetadata(media)
	if err != nil {
		return err
	}
	return r.completeMediaAsset(ctx, mediaAssetCompletion{
		ID: assetID, OwnerUserID: media.OwnerUserID, URL: media.URL, SourceURL: media.SourceURL,
		MIMEType: media.MIMEType, Width: media.Width, Height: media.Height, SizeBytes: media.SizeBytes,
		ChecksumSHA256: media.ChecksumSHA256, Metadata: metadata,
	})
}

func (r *MediaAssetRegistry) FailPreparedGenerationMedia(ctx context.Context, ownerUserID, assetID int64, cause error) error {
	return r.failMediaAsset(ctx, ownerUserID, assetID, cause)
}

func (r *MediaAssetRegistry) BeginTaskResultIngestion(ctx context.Context, ingestion storage.TaskResultIngestion) (int64, bool, error) {
	if r == nil || r.q == nil || ingestion.OwnerUserID <= 0 || ingestion.ResultIndex < 0 {
		return 0, false, fmt.Errorf("invalid task result ingestion")
	}
	taskID := strings.TrimSpace(ingestion.TaskID)
	profileID := strings.TrimSpace(ingestion.StorageProfile)
	storageKey := strings.TrimSpace(ingestion.StorageKey)
	if taskID == "" || profileID == "" || storageKey == "" {
		return 0, false, fmt.Errorf("incomplete task result ingestion")
	}
	assetID, err := newUploadMediaAssetID()
	if err != nil {
		return 0, false, err
	}
	metadata, err := json.Marshal(map[string]any{
		"origin": "task_result", "task_id": taskID, "result_index": ingestion.ResultIndex,
	})
	if err != nil {
		return 0, false, fmt.Errorf("encode task result ingestion metadata: %w", err)
	}
	existingReady := false
	if existing, lookupErr := r.q.GetMediaAssetByStorageLocator(ctx, dbsqlc.GetMediaAssetByStorageLocatorParams{StorageProfileID: profileID, StorageKey: storageKey}); lookupErr == nil {
		existingReady = existing.Status == "ready"
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("check task result ingestion state: %w", lookupErr)
	}
	id, err := r.q.BeginTaskResultMediaAsset(ctx, dbsqlc.BeginTaskResultMediaAssetParams{
		AssetID: assetID, OwnerUserID: ingestion.OwnerUserID, StorageProfileID: profileID,
		StorageKey: storageKey, Url: strings.TrimSpace(ingestion.URL), SourceUrl: strings.TrimSpace(ingestion.SourceURL),
		Metadata: metadata, ReclaimAfter: sql.NullTime{Time: r.now().Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return 0, false, fmt.Errorf("begin task result ingestion: %w", err)
	}
	return id, existingReady, nil
}

func (r *MediaAssetRegistry) CompleteTaskResultIngestion(ctx context.Context, ingestion storage.TaskResultIngestion, stored storage.StoredUploadRef) error {
	metadata, err := json.Marshal(map[string]any{
		"origin": "task_result", "task_id": strings.TrimSpace(ingestion.TaskID), "result_index": ingestion.ResultIndex,
		"animated": stored.Animated,
	})
	if err != nil {
		return fmt.Errorf("encode task result completion metadata: %w", err)
	}
	return r.completeMediaAsset(ctx, mediaAssetCompletion{
		ID: ingestion.AssetID, OwnerUserID: ingestion.OwnerUserID, URL: stored.URL, SourceURL: ingestion.SourceURL,
		MIMEType: stored.MIMEType, Width: stored.Width, Height: stored.Height, SizeBytes: stored.SizeBytes,
		ChecksumSHA256: stored.ChecksumSHA256, Metadata: metadata,
	})
}

func (r *MediaAssetRegistry) FailTaskResultIngestion(ctx context.Context, ownerUserID, assetID int64, cause error) error {
	return r.failMediaAsset(ctx, ownerUserID, assetID, cause)
}

func (r *MediaAssetRegistry) insertPreparedGenerationMedia(ctx context.Context, media PreparedGenerationMedia) (int64, error) {
	if r == nil || r.db == nil || r.q == nil || media.OwnerUserID <= 0 || media.InputIndex < 0 {
		return 0, fmt.Errorf("invalid prepared generation media")
	}
	if strings.TrimSpace(media.StorageProfile) == "" || strings.TrimSpace(media.StorageKey) == "" || strings.TrimSpace(media.URL) == "" || strings.TrimSpace(media.RefID) == "" || strings.TrimSpace(media.Role) == "" {
		return 0, fmt.Errorf("incomplete prepared generation media")
	}
	assetID, err := newUploadMediaAssetID()
	if err != nil {
		return 0, err
	}
	metadata, err := preparedMediaMetadata(media)
	if err != nil {
		return 0, fmt.Errorf("encode prepared media metadata: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prepared media registration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	asset, err := q.CreatePreparedMediaAsset(ctx, dbsqlc.CreatePreparedMediaAssetParams{
		AssetID: assetID, OwnerUserID: media.OwnerUserID, StorageProfileID: media.StorageProfile,
		StorageKey: media.StorageKey, Url: media.URL, SourceUrl: media.SourceURL, MimeType: media.MIMEType,
		Width: int32(media.Width), Height: int32(media.Height), SizeBytes: media.SizeBytes, Metadata: metadata,
		ChecksumSha256: media.ChecksumSHA256, Status: "uploading",
		ReclaimAfter: sql.NullTime{Time: r.now().Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("register prepared media asset: %w", err)
	}
	if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{MediaAssetID: asset, OwnerUserID: media.OwnerUserID, RefType: "prompt_reference", RefID: media.RefID, TaskID: "", Role: media.Role, InputIndex: int32(media.InputIndex), Metadata: metadata}); err != nil {
		return 0, fmt.Errorf("register prepared media ref: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prepared media registration: %w", err)
	}
	committed = true
	return asset, nil
}

func preparedMediaMetadata(media PreparedGenerationMedia) ([]byte, error) {
	return json.Marshal(map[string]any{"origin": "generation_prepare", "ref_id": media.RefID, "role": media.Role, "input_index": media.InputIndex, "unclaimed": true, "animated": media.Animated})
}

func (r *MediaAssetRegistry) enqueueDerivatives(ctx context.Context, ids []int64) {
	if r == nil || r.derivativeEnqueuer == nil {
		return
	}
	for _, id := range ids {
		if err := r.derivativeEnqueuer(ctx, id); err != nil {
			continue
		}
	}
}

func (r *MediaAssetRegistry) FindPreparedGenerationMedia(ctx context.Context, ownerUserID int64, refID, role string, inputIndex int) (PreparedGenerationMedia, bool, error) {
	if r == nil || r.q == nil {
		return PreparedGenerationMedia{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	row, err := r.q.FindPreparedGenerationMedia(ctx, dbsqlc.FindPreparedGenerationMediaParams{OwnerUserID: ownerUserID, RefID: refID, Role: role, InputIndex: int32(inputIndex)})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PreparedGenerationMedia{}, false, nil
		}
		return PreparedGenerationMedia{}, false, err
	}
	return PreparedGenerationMedia{AssetID: row.ID, OwnerUserID: ownerUserID, StorageProfile: row.StorageProfileID, StorageKey: row.StorageKey, URL: row.Url, SourceURL: row.SourceUrl, MIMEType: row.MimeType, Width: int(row.Width), Height: int(row.Height), SizeBytes: row.SizeBytes, ChecksumSHA256: row.ChecksumSha256, InputIndex: inputIndex, Role: role, RefID: refID}, true, nil
}

func (r *MediaAssetRegistry) FindIngestingPreparedGenerationMedia(ctx context.Context, ownerUserID int64, refID, role string, inputIndex int) (PreparedGenerationMedia, bool, error) {
	if r == nil || r.q == nil {
		return PreparedGenerationMedia{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	row, err := r.q.FindIngestingPreparedGenerationMedia(ctx, dbsqlc.FindIngestingPreparedGenerationMediaParams{OwnerUserID: ownerUserID, RefID: refID, Role: role, InputIndex: int32(inputIndex)})
	if errors.Is(err, sql.ErrNoRows) {
		return PreparedGenerationMedia{}, false, nil
	}
	if err != nil {
		return PreparedGenerationMedia{}, false, err
	}
	return PreparedGenerationMedia{AssetID: row.ID, OwnerUserID: ownerUserID, StorageProfile: row.StorageProfileID, StorageKey: row.StorageKey, URL: row.Url, SourceURL: row.SourceUrl, MIMEType: row.MimeType, Width: int(row.Width), Height: int(row.Height), SizeBytes: row.SizeBytes, ChecksumSHA256: row.ChecksumSha256, InputIndex: inputIndex, Role: role, RefID: refID}, true, nil
}

func (r *MediaAssetRegistry) FindReusableGenerationMedia(ctx context.Context, ownerUserID int64, source string) (int64, bool, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 {
		return 0, false, fmt.Errorf("media asset registry is unavailable")
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return 0, false, nil
	}
	if strings.HasPrefix(trimmed, "/uploads/") {
		resource, err := storage.ParseUploadResource(strings.TrimPrefix(trimmed, "/"))
		if err != nil {
			return 0, false, nil
		}
		ids, err := r.q.ListReadyOwnedMediaAssetIDsByStorageKey(ctx, dbsqlc.ListReadyOwnedMediaAssetIDsByStorageKeyParams{
			OwnerUserID: ownerUserID,
			StorageKey:  resource.ObjectKey,
		})
		if err != nil {
			return 0, false, fmt.Errorf("find reusable media asset by storage key: %w", err)
		}
		switch len(ids) {
		case 0:
			return 0, false, nil
		case 1:
			return ids[0], true, nil
		default:
			return 0, false, fmt.Errorf("reusable media asset storage key is ambiguous")
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/uploads/") {
		return 0, false, nil
	}
	assetID, err := r.q.FindReadyOwnedMediaAssetIDByURL(ctx, dbsqlc.FindReadyOwnedMediaAssetIDByURLParams{
		OwnerUserID: ownerUserID,
		Url:         trimmed,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find reusable media asset by URL: %w", err)
	}
	return assetID, true, nil
}

func (r *MediaAssetRegistry) LocateStoredUpload(ctx context.Context, lookup storage.StoredUploadLookup) (storage.StoredUploadLocation, bool, error) {
	if r == nil || r.q == nil {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	if lookup.AssetID > 0 {
		row, err := r.q.GetReadyMediaAssetStorageLocationByID(ctx, lookup.AssetID)
		if err == nil {
			return storage.StoredUploadLocation{StorageProfileID: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by ID: %w", err)
		}
		return storage.StoredUploadLocation{}, false, nil
	}
	if rawURL := strings.TrimSpace(lookup.URL); rawURL != "" {
		row, err := r.q.FindMediaAssetStorageLocationByURL(ctx, rawURL)
		if err == nil {
			return storage.StoredUploadLocation{
				StorageProfileID: row.StorageProfileID,
				StorageKey:       row.StorageKey,
			}, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by URL: %w", err)
		}
	}

	storageKey := strings.TrimSpace(lookup.ObjectKey)
	if storageKey == "" {
		return storage.StoredUploadLocation{}, false, nil
	}
	rows, err := r.q.ListMediaAssetStorageLocationsByKey(ctx, storageKey)
	if err != nil {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by storage key: %w", err)
	}
	if len(rows) == 0 {
		return storage.StoredUploadLocation{}, false, nil
	}
	if len(rows) > 1 {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("media asset storage key is ambiguous")
	}
	return storage.StoredUploadLocation{
		StorageProfileID: rows[0].StorageProfileID,
		StorageKey:       rows[0].StorageKey,
	}, true, nil
}

func newUploadMediaAssetID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate upload media asset id: %w", err)
	}
	return "asset_" + hex.EncodeToString(value[:]), nil
}
