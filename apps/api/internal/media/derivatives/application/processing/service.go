package processing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/imageformat"
	storagedomain "ideaboard/api/internal/media/storage/domain"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
)

// Service synchronously materializes the processing-input derivative right
// where assets are created. Display tiers resolve on demand through imgproxy,
// so there is no queue, no outbox, and no retry scheduling: a render failure
// fails the creating request instead of wedging clients on pending rows.
type Service struct {
	db       *sql.DB
	storage  ports.Storage
	verifier ports.Verifier
	renderer ports.Renderer
	events   ports.EventPublisher
}

func NewService(
	db *sql.DB,
	storage ports.Storage,
	verifier ports.Verifier,
	renderer ports.Renderer,
	events ports.EventPublisher,
) *Service {
	return &Service{
		db:       db,
		storage:  storage,
		verifier: verifier,
		renderer: renderer,
		events:   events,
	}
}

// MaterializeProcessingInput renders the provider-neutral processing-input
// derivative for a ready asset. It is idempotent: assets that already carry
// a ready row for the current object generation return immediately.
func (s *Service) MaterializeProcessingInput(ctx context.Context, assetID, ownerUserID int64) error {
	if s == nil || s.db == nil || s.storage == nil || s.renderer == nil || assetID <= 0 || ownerUserID <= 0 {
		return fmt.Errorf("media derivative service is unavailable")
	}
	q := dbsqlc.New(s.db)
	asset, err := q.GetMediaAssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("load media asset: %w", err)
	}
	if asset.OwnerUserID != ownerUserID || asset.Status != "ready" || asset.DeletedAt.Valid {
		return fmt.Errorf("media asset %d is not available for processing-input", assetID)
	}
	if existing, err := q.GetMediaDerivativeByAssetTier(ctx, dbsqlc.GetMediaDerivativeByAssetTierParams{
		MediaAssetID: assetID, OwnerUserID: ownerUserID, Tier: domain.TierProcessingInput,
	}); err == nil && existing.Status == "ready" && existing.SourceObjectGeneration == asset.ObjectGeneration {
		return nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load processing-input derivative: %w", err)
	}

	renderCtx, cancel := context.WithTimeout(ctx, domain.RenderDeadline)
	defer cancel()
	original, err := s.storage.ReadObjectAtProfile(renderCtx, asset.StorageProfileID, asset.StorageKey, domain.MaxSourceBytes)
	if err != nil {
		return fmt.Errorf("read processing-input source: %w", err)
	}
	if original == nil || len(original.Data) == 0 {
		return fmt.Errorf("processing-input source object is empty")
	}
	if _, err := imageformat.InspectDetectedBytes(original.Data); err != nil {
		return fmt.Errorf("inspect processing-input source: %w", err)
	}
	encoded, width, height, format, err := s.renderer.Render(renderCtx, original.Data, asset.StorageKey, domain.ProcessingInputMaxSide)
	if err != nil {
		return fmt.Errorf("render processing-input: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > domain.MaxOutputBytes {
		return fmt.Errorf("rendered processing-input size is invalid: %d", len(encoded))
	}
	key, err := storagedomain.ImageDerivativeObjectKey(asset.StorageKey, asset.AssetID, domain.TierProcessingInput, format)
	if err != nil {
		return fmt.Errorf("processing-input object key: %w", err)
	}
	mimeType := domain.DerivativeMimeType(format)
	if err := s.storage.PutObjectAtProfile(ctx, asset.StorageProfileID, key, encoded, mimeType); err != nil {
		return fmt.Errorf("store processing-input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	checksum := hex.EncodeToString(digest[:])
	if s.verifier != nil {
		if err := s.verifier.Verify(ctx, asset.StorageProfileID, key, checksum, mimeType, int64(len(encoded))); err != nil {
			code := "processing-input integrity check failed"
			if mediaerrors.IsRetryable(err) {
				code += " (transient)"
			} else {
				_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), asset.StorageProfileID, key)
			}
			return fmt.Errorf("%s: %w", code, err)
		}
	}
	derivativeID, err := domain.NewDerivativeID()
	if err != nil {
		return fmt.Errorf("new processing-input derivative id: %w", err)
	}
	derivative, err := q.UpsertMediaDerivative(ctx, dbsqlc.UpsertMediaDerivativeParams{
		DerivativeID: derivativeID, MediaAssetID: assetID, OwnerUserID: ownerUserID,
		Tier: domain.TierProcessingInput, Format: format,
	})
	if err != nil {
		return fmt.Errorf("upsert processing-input derivative: %w", err)
	}
	ready, err := q.MarkMediaDerivativeReady(ctx, dbsqlc.MarkMediaDerivativeReadyParams{
		ID: derivative.ID, Format: format, StorageKey: key, MimeType: mimeType,
		Width: int32(width), Height: int32(height), SizeBytes: int64(len(encoded)),
		ChecksumSha256: checksum, SourceObjectGeneration: asset.ObjectGeneration,
	})
	if err != nil {
		return fmt.Errorf("mark processing-input ready: %w", err)
	}
	s.publishEvent(ready.OwnerUserID, domain.Event{
		Type:          domain.EventTypeReady,
		MediaAssetID:  ready.MediaAssetID,
		DerivativeID:  ready.DerivativeID,
		Tier:          ready.Tier,
		Format:        ready.Format,
		Status:        ready.Status,
		SchemaVersion: domain.EventSchemaVersion,
	})
	return nil
}

func (s *Service) publishEvent(userID int64, event domain.Event) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(userID, event)
}
