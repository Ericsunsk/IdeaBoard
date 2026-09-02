package derivatives

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	TierThumb        = "thumb"
	TierPreview      = "preview"
	TierLargePreview = "large-preview"
	TierProcessing   = "processing"
	// TierProcessingInput is a provider-neutral derivative intended for
	// external image processing endpoints. It is separate from the editor
	// processing tier so UI sizing changes do not alter provider inputs.
	TierProcessingInput = "processing-input"
	FormatJPEG          = "jpeg"
	FormatPNG           = "png"
	FormatWebP          = "webp"
)

type TierSpec struct {
	MaxSide int
}

var TierSpecs = map[string]TierSpec{
	TierThumb:           {MaxSide: 256},
	TierPreview:         {MaxSide: 640},
	TierLargePreview:    {MaxSide: 1280},
	TierProcessing:      {MaxSide: 2560},
	TierProcessingInput: {MaxSide: ProcessingInputMaxSide},
}

type Service struct {
	db       *sql.DB
	storage  *storage.StorageService
	verifier *storage.IntegrityVerifier
	renderer renderer
	events   EventPublisher
}

type EventPublisher interface {
	Publish(int64, Event)
}

const MaxAttempts = 5

const (
	maxSourceBytes = 100 * 1024 * 1024
	maxOutputBytes = 20 * 1024 * 1024
	renderDeadline = 2 * time.Minute
)

const (
	retryDelayInitial = 5 * time.Second
	retryDelayMax     = 2 * time.Minute
)

type renderer interface {
	Render(context.Context, []byte, string, int) ([]byte, int, int, string, error)
}

type failureRecordedError struct{ err error }

func (e *failureRecordedError) Error() string { return e.err.Error() }

func (e *failureRecordedError) Unwrap() error { return e.err }

func NewService(db *sql.DB, storageService *storage.StorageService, publishers ...EventPublisher) *Service {
	var publisher EventPublisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		db: db, storage: storageService,
		verifier: storage.NewIntegrityVerifier(storageService, storage.IntegrityConfig{MaxBytes: maxOutputBytes + 1}),
		renderer: newRenderer(), events: publisher,
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
	processing, err := q.MarkMediaDerivativeProcessing(ctx, dbsqlc.MarkMediaDerivativeProcessingParams{ID: derivative.ID, MaxAttemptCount: MaxAttempts})
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

func (s *Service) loadProcessingInputs(ctx context.Context, q *dbsqlc.Queries, derivative, processing dbsqlc.MediaDerivative) (dbsqlc.MediaAsset, TierSpec, string, error) {
	asset, err := q.GetMediaAssetByID(ctx, derivative.MediaAssetID)
	if err != nil {
		return dbsqlc.MediaAsset{}, TierSpec{}, "asset_not_found", err
	}
	spec, ok := TierSpecs[processing.Tier]
	if !ok {
		return dbsqlc.MediaAsset{}, TierSpec{}, "invalid_tier", fmt.Errorf("unknown tier %q", processing.Tier)
	}
	return asset, spec, "", nil
}

type renderedDerivative struct {
	encoded       []byte
	width, height int
	format        string
}

func (s *Service) renderDerivative(ctx context.Context, asset dbsqlc.MediaAsset, spec TierSpec) (renderedDerivative, string, error) {
	renderCtx, cancel := context.WithTimeout(ctx, renderDeadline)
	defer cancel()
	original, err := s.storage.ReadObjectAtProfile(renderCtx, asset.StorageProfileID, asset.StorageKey, maxSourceBytes)
	if err != nil {
		return renderedDerivative{}, "source_read_failed", err
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
	if len(encoded) == 0 || len(encoded) > maxOutputBytes {
		return renderedDerivative{}, "output_limits_exceeded", fmt.Errorf("rendered output size is invalid: %d", len(encoded))
	}
	return renderedDerivative{encoded: encoded, width: width, height: height, format: format}, "", nil
}

type persistedDerivative struct {
	key, mimeType, checksum string
}

func (s *Service) persistRenderedDerivative(ctx context.Context, asset dbsqlc.MediaAsset, processing dbsqlc.MediaDerivative, rendered renderedDerivative) (persistedDerivative, string, error) {
	key, err := storage.ImageDerivativeObjectKey(asset.StorageKey, asset.AssetID, processing.Tier, rendered.format)
	if err != nil {
		return persistedDerivative{}, "key_failed", err
	}
	mimeType := derivativeMimeType(rendered.format)
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

func derivativeMimeType(format string) string {
	switch format {
	case FormatPNG:
		return "image/png"
	case FormatWebP:
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func (s *Service) completeDerivative(ctx context.Context, q *dbsqlc.Queries, processing dbsqlc.MediaDerivative, asset dbsqlc.MediaAsset, rendered renderedDerivative, persisted persistedDerivative) error {
	ready, err := q.MarkMediaDerivativeReady(ctx, dbsqlc.MarkMediaDerivativeReadyParams{
		ID: processing.ID, Format: rendered.format, StorageKey: persisted.key, MimeType: persisted.mimeType, Width: int32(rendered.width), Height: int32(rendered.height),
		SizeBytes: int64(len(rendered.encoded)), ChecksumSha256: persisted.checksum, SourceObjectGeneration: asset.ObjectGeneration,
	})
	if err != nil {
		return fmt.Errorf("mark media derivative ready: %w", err)
	}
	s.publishEvent(ready.OwnerUserID, Event{Type: EventTypeReady, MediaAssetID: ready.MediaAssetID, DerivativeID: ready.DerivativeID, Tier: ready.Tier, Format: ready.Format, Status: ready.Status, SchemaVersion: EventSchemaVersion})
	if err := q.DeleteMediaDerivativeOutbox(ctx, processing.ID); err != nil {
		return fmt.Errorf("delete media derivative outbox: %w", err)
	}
	return nil
}

func (s *Service) markFailed(ctx context.Context, q *dbsqlc.Queries, id int64, code string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	retryable := isRetryableDerivativeError(code)
	minAttemptCount := int32(0)
	if !retryable {
		minAttemptCount = MaxAttempts
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
	s.publishEvent(failed.OwnerUserID, Event{Type: EventTypeUpdated, MediaAssetID: failed.MediaAssetID, DerivativeID: failed.DerivativeID, Tier: failed.Tier, Format: failed.Format, Status: failed.Status, SchemaVersion: EventSchemaVersion})
	if retryable && failed.AttemptCount < MaxAttempts {
		if err := q.RescheduleMediaDerivativeOutbox(ctx, dbsqlc.RescheduleMediaDerivativeOutboxParams{
			DerivativeID: id,
			AvailableAt:  time.Now().Add(derivativeRetryDelay(failed.AttemptCount)),
			LastError:    message,
		}); err != nil {
			return fmt.Errorf("%s: %v; schedule retry: %w", code, cause, err)
		}
	} else if err := q.DeleteMediaDerivativeOutbox(ctx, id); err != nil {
		return fmt.Errorf("%s: %v; clear terminal outbox: %w", code, cause, err)
	}
	return &failureRecordedError{err: fmt.Errorf("%s: %w", code, cause)}
}

func (s *Service) publishEvent(userID int64, event Event) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(userID, event)
}

func isRetryableDerivativeError(code string) bool {
	switch code {
	case "source_read_failed", "render_failed", "destination_write_failed", "integrity_transient":
		return true
	default:
		return false
	}
}

func derivativeRetryDelay(attemptCount int32) time.Duration {
	if attemptCount <= 1 {
		return retryDelayInitial
	}
	delay := retryDelayInitial
	for attempt := int32(1); attempt < attemptCount; attempt++ {
		delay *= 3
		if delay >= retryDelayMax {
			return retryDelayMax
		}
	}
	return delay
}

type goRenderer struct{}

func (goRenderer) Render(_ context.Context, data []byte, _ string, maxSide int) ([]byte, int, int, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("decode source image: %w", err)
	}
	bounds := img.Bounds()
	longest := max(bounds.Dx(), bounds.Dy())
	width, height := bounds.Dx(), bounds.Dy()
	if longest > maxSide {
		scale := float64(maxSide) / float64(longest)
		width = max(1, int(float64(width)*scale+0.5))
		height = max(1, int(float64(height)*scale+0.5))
	}
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
	transparent := hasTransparency(resized)
	var output bytes.Buffer
	if transparent {
		if err := png.Encode(&output, resized); err != nil {
			return nil, 0, 0, "", err
		}
		return output.Bytes(), width, height, FormatPNG, nil
	}
	if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, "", err
	}
	return output.Bytes(), width, height, FormatJPEG, nil
}

type vipsRenderer struct{ binary string }

func newRenderer() renderer {
	binary := strings.TrimSpace(os.Getenv("IDEABOARD_VIPS_BINARY"))
	if binary == "" {
		return goRenderer{}
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return goRenderer{}
	}
	return vipsRenderer{binary: resolved}
}

func (r vipsRenderer) Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error) {
	if strings.TrimSpace(r.binary) == "" {
		return nil, 0, 0, "", fmt.Errorf("libvips binary is not configured")
	}
	ext := filepath.Ext(sourceName)
	if ext == "" {
		ext = ".source"
	}
	input, err := os.CreateTemp("", "ideaboard-vips-*"+ext)
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("create libvips input: %w", err)
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	defer os.Remove(inputPath + ".webp")
	if err := input.Chmod(0600); err != nil {
		_ = input.Close()
		return nil, 0, 0, "", err
	}
	if _, err := input.Write(data); err != nil {
		_ = input.Close()
		return nil, 0, 0, "", fmt.Errorf("write libvips input: %w", err)
	}
	if err := input.Close(); err != nil {
		return nil, 0, 0, "", err
	}
	outputPath := inputPath + ".webp"
	cmd := exec.CommandContext(ctx, r.binary, "thumbnail", inputPath, outputPath, strconv.Itoa(maxSide), "--size", "both")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, 0, "", fmt.Errorf("libvips thumbnail failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("read libvips output: %w", err)
	}
	facts, err := imageformat.InspectBytes(encoded, ".webp")
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("validate libvips output: %w", err)
	}
	return encoded, facts.Width, facts.Height, FormatWebP, nil
}

func hasTransparency(img image.Image) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A < 255 {
				return true
			}
		}
	}
	return false
}
