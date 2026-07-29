package assets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
	q   *dbsqlc.Queries
	now func() time.Time
}

func NewMediaAssetRegistry(db *sql.DB) *MediaAssetRegistry {
	return &MediaAssetRegistry{q: dbsqlc.New(db), now: time.Now}
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

func newUploadMediaAssetID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate upload media asset id: %w", err)
	}
	return "asset_" + hex.EncodeToString(value[:]), nil
}
