package processing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/imageformat"
	storagedomain "ideaboard/api/internal/media/storage/domain"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
)

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

func (s *Service) Process(ctx context.Context, derivativeID int64) error {
	if s == nil || s.db == nil || s.storage == nil || derivativeID <= 0 {
		return fmt.Errorf("media derivative service is unavailable")
	}
	q := dbsqlc.New(s.db)
	derivative, err := q.GetMediaDerivativeByID(ctx, derivativeID)
	if err != nil {
		return fmt.Errorf("load media derivative: %w", err)
	}
	if isCompletedStatus(derivative.Status) {
		return s.clearCompletedOutbox(ctx, q, derivative.ID)
	}
	processing, err := q.MarkMediaDerivativeProcessing(ctx, dbsqlc.MarkMediaDerivativeProcessingParams{
		ID:              derivative.ID,
		MaxAttemptCount: domain.MaxAttempts,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("claim media derivative: %w", err)
	}
	asset, spec, failureCode, err := s.loadProcessingInputs(ctx, q, derivative, processing)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, failureCode, err)
	}
	rendered, failureCode, err := s.renderDerivative(ctx, asset, spec)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, failureCode, err)
	}
	persisted, failureCode, err := s.persistRenderedDerivative(ctx, asset, processing, rendered)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, failureCode, err)
	}
	return s.completeDerivative(ctx, q, processing, asset, rendered, persisted)
}

func isCompletedStatus(status string) bool {
	return status == "ready" || status == "obsolete"
}

func (s *Service) clearCompletedOutbox(ctx context.Context, q *dbsqlc.Queries, derivativeID int64) error {
	if err := q.DeleteMediaDerivativeOutbox(ctx, derivativeID); err != nil {
		return fmt.Errorf("clear completed media derivative outbox: %w", err)
	}
	return nil
}

func (s *Service) loadProcessingInputs(ctx context.Context, q *dbsqlc.Queries, derivative, processing dbsqlc.MediaDerivative) (dbsqlc.MediaAsset, domain.TierSpec, string, error) {
	asset, err := q.GetMediaAssetByID(ctx, derivative.MediaAssetID)
	if err != nil {
		return dbsqlc.MediaAsset{}, domain.TierSpec{}, "asset_not_found", err
	}
	spec, ok := domain.TierSpecs[processing.Tier]
	if !ok {
		return dbsqlc.MediaAsset{}, domain.TierSpec{}, "invalid_tier", fmt.Errorf("unknown tier %q", processing.Tier)
	}
	return asset, spec, "", nil
}

type renderedDerivative struct {
	encoded       []byte
	width, height int
	format        string
}

func (s *Service) renderDerivative(ctx context.Context, asset dbsqlc.MediaAsset, spec domain.TierSpec) (renderedDerivative, string, error) {
	renderCtx, cancel := context.WithTimeout(ctx, domain.RenderDeadline)
	defer cancel()
	original, err := s.storage.ReadObjectAtProfile(renderCtx, asset.StorageProfileID, asset.StorageKey, domain.MaxSourceBytes)
	if err != nil {
		return renderedDerivative{}, "source_read_failed", err
	}
	if original == nil || len(original.Data) == 0 {
		return renderedDerivative{}, "source_read_failed", fmt.Errorf("source object is empty")
	}
	if _, err := imageformat.InspectBytes(original.Data, filepath.Ext(asset.StorageKey)); err != nil {
		return renderedDerivative{}, "source_limits_exceeded", err
	}
	if s.renderer == nil {
		return renderedDerivative{}, "renderer_unavailable", fmt.Errorf("media renderer is unavailable")
	}
	encoded, width, height, format, err := s.renderer.Render(renderCtx, original.Data, asset.StorageKey, spec.MaxSide)
	if err != nil {
		return renderedDerivative{}, "render_failed", err
	}
	if len(encoded) == 0 || len(encoded) > domain.MaxOutputBytes {
		return renderedDerivative{}, "output_limits_exceeded", fmt.Errorf("rendered output size is invalid: %d", len(encoded))
	}
	return renderedDerivative{encoded: encoded, width: width, height: height, format: format}, "", nil
}

type persistedDerivative struct {
	key, mimeType, checksum string
}

func (s *Service) persistRenderedDerivative(ctx context.Context, asset dbsqlc.MediaAsset, processing dbsqlc.MediaDerivative, rendered renderedDerivative) (persistedDerivative, string, error) {
	key, err := storagedomain.ImageDerivativeObjectKey(asset.StorageKey, asset.AssetID, processing.Tier, rendered.format)
	if err != nil {
		return persistedDerivative{}, "key_failed", err
	}
	mimeType := domain.DerivativeMimeType(rendered.format)
	if err := s.storage.PutObjectAtProfile(ctx, asset.StorageProfileID, key, rendered.encoded, mimeType); err != nil {
		return persistedDerivative{}, "destination_write_failed", err
	}
	digest := sha256.Sum256(rendered.encoded)
	checksum := hex.EncodeToString(digest[:])
	if err := s.verifier.Verify(ctx, asset.StorageProfileID, key, checksum, mimeType, int64(len(rendered.encoded))); err != nil {
		code := "integrity_invalid"
		if mediaerrors.IsRetryable(err) {
			code = "integrity_transient"
		} else {
			_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), asset.StorageProfileID, key)
		}
		return persistedDerivative{}, code, err
	}
	return persistedDerivative{key: key, mimeType: mimeType, checksum: checksum}, "", nil
}

func (s *Service) completeDerivative(ctx context.Context, q *dbsqlc.Queries, processing dbsqlc.MediaDerivative, asset dbsqlc.MediaAsset, rendered renderedDerivative, persisted persistedDerivative) error {
	ready, err := q.MarkMediaDerivativeReady(ctx, dbsqlc.MarkMediaDerivativeReadyParams{
		ID:                     processing.ID,
		Format:                 rendered.format,
		StorageKey:             persisted.key,
		MimeType:               persisted.mimeType,
		Width:                  int32(rendered.width),
		Height:                 int32(rendered.height),
		SizeBytes:              int64(len(rendered.encoded)),
		ChecksumSha256:         persisted.checksum,
		SourceObjectGeneration: asset.ObjectGeneration,
	})
	if err != nil {
		return fmt.Errorf("mark media derivative ready: %w", err)
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
	if err := q.DeleteMediaDerivativeOutbox(ctx, processing.ID); err != nil {
		return fmt.Errorf("delete media derivative outbox: %w", err)
	}
	return nil
}

func (s *Service) markFailed(ctx context.Context, q *dbsqlc.Queries, id int64, code string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	retryable := domain.IsRetryableDerivativeError(code)
	minAttemptCount := int32(0)
	if !retryable {
		minAttemptCount = domain.MaxAttempts
	}
	failed, err := q.MarkMediaDerivativeFailed(ctx, dbsqlc.MarkMediaDerivativeFailedParams{
		ID:              id,
		ErrorCode:       code,
		ErrorMessage:    message,
		MinAttemptCount: minAttemptCount,
	})
	if err != nil {
		return fmt.Errorf("%s: %v; mark failed: %w", code, cause, err)
	}
	s.publishEvent(failed.OwnerUserID, domain.Event{
		Type:          domain.EventTypeUpdated,
		MediaAssetID:  failed.MediaAssetID,
		DerivativeID:  failed.DerivativeID,
		Tier:          failed.Tier,
		Format:        failed.Format,
		Status:        failed.Status,
		SchemaVersion: domain.EventSchemaVersion,
	})
	if retryable && failed.AttemptCount < domain.MaxAttempts {
		if err := q.RescheduleMediaDerivativeOutbox(ctx, dbsqlc.RescheduleMediaDerivativeOutboxParams{
			DerivativeID: id,
			AvailableAt:  time.Now().Add(domain.DerivativeRetryDelay(failed.AttemptCount)),
			LastError:    message,
		}); err != nil {
			return fmt.Errorf("%s: %v; schedule retry: %w", code, cause, err)
		}
	} else if err := q.DeleteMediaDerivativeOutbox(ctx, id); err != nil {
		return fmt.Errorf("%s: %v; clear terminal outbox: %w", code, cause, err)
	}
	return domain.NewFailureRecordedError(fmt.Errorf("%s: %w", code, cause))
}

func (s *Service) publishEvent(userID int64, event domain.Event) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(userID, event)
}
