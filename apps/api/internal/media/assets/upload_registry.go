package assets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/media/storage"
)

type BoardUploadAsset struct {
	OwnerUserID int64
	BoardID     string
	Stored      storage.StoredUploadRef
}

type MediaAssetRegistry struct {
	db  *sql.DB
	q   *dbsqlc.Queries
	now func() time.Time
}

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
	InputIndex     int
	Role           string
	RefID          string
}

func NewMediaAssetRegistry(db *sql.DB) *MediaAssetRegistry {
	return &MediaAssetRegistry{db: db, q: dbsqlc.New(db), now: time.Now}
}

func (r *MediaAssetRegistry) RegisterBoardUpload(ctx context.Context, upload BoardUploadAsset) (int64, error) {
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
	metadata, err := json.Marshal(map[string]string{
		"origin":   "board_upload",
		"board_id": boardID,
	})
	if err != nil {
		return 0, fmt.Errorf("encode board upload media metadata: %w", err)
	}
	registeredAt := r.now()
	id, err := r.q.CreateBoardUploadMediaAsset(ctx, dbsqlc.CreateBoardUploadMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      upload.OwnerUserID,
		StorageProfileID: storageProfileID,
		StorageKey:       storageKey,
		Url:              url,
		MimeType:         strings.TrimSpace(upload.Stored.MIMEType),
		Width:            int32(upload.Stored.Width),
		Height:           int32(upload.Stored.Height),
		SizeBytes:        upload.Stored.SizeBytes,
		Metadata:         metadata,
		ReclaimAfter:     sql.NullTime{Time: registeredAt.Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("register board upload media asset: %w", err)
	}
	return id, nil
}

func (r *MediaAssetRegistry) RegisterPreparedGenerationMedia(ctx context.Context, media PreparedGenerationMedia) (int64, error) {
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
	metadata, err := json.Marshal(map[string]any{"origin": "generation_prepare", "ref_id": media.RefID, "role": media.Role, "input_index": media.InputIndex, "unclaimed": true})
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
	return PreparedGenerationMedia{AssetID: row.ID, OwnerUserID: ownerUserID, StorageProfile: row.StorageProfileID, StorageKey: row.StorageKey, URL: row.Url, SourceURL: row.SourceUrl, MIMEType: row.MimeType, Width: int(row.Width), Height: int(row.Height), SizeBytes: row.SizeBytes, InputIndex: inputIndex, Role: role, RefID: refID}, true, nil
}

func newUploadMediaAssetID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate upload media asset id: %w", err)
	}
	return "asset_" + hex.EncodeToString(value[:]), nil
}
