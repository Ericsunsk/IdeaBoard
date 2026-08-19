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
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/storage"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	TierThumb        = "thumb"
	TierPreview      = "preview"
	TierLargePreview = "large-preview"
	FormatJPEG       = "jpeg"
	FormatPNG        = "png"
)

type TierSpec struct {
	MaxSide int
}

var TierSpecs = map[string]TierSpec{
	TierThumb:        {MaxSide: 256},
	TierPreview:      {MaxSide: 640},
	TierLargePreview: {MaxSide: 1280},
}

type Service struct {
	db      *sql.DB
	storage *storage.StorageService
}

func NewService(db *sql.DB, storageService *storage.StorageService) *Service {
	return &Service{db: db, storage: storageService}
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
	if derivative.Status == "ready" || derivative.Status == "obsolete" {
		if err := q.DeleteMediaDerivativeOutbox(ctx, derivative.ID); err != nil {
			return fmt.Errorf("clear completed media derivative outbox: %w", err)
		}
		return nil
	}
	processing, err := q.MarkMediaDerivativeProcessing(ctx, derivative.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("claim media derivative: %w", err)
	}
	asset, err := q.GetMediaAssetByID(ctx, derivative.MediaAssetID)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, "asset_not_found", err)
	}
	spec, ok := TierSpecs[processing.Tier]
	if !ok {
		return s.markFailed(ctx, q, processing.ID, "invalid_tier", fmt.Errorf("unknown tier %q", processing.Tier))
	}
	original, err := s.storage.ReadObjectAtProfile(ctx, asset.StorageProfileID, asset.StorageKey, 100*1024*1024)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, "source_read_failed", err)
	}
	encoded, width, height, format, err := render(original.Data, spec.MaxSide)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, "render_failed", err)
	}
	key, err := storage.ImageDerivativeObjectKey(asset.StorageKey, asset.AssetID, processing.Tier, format)
	if err != nil {
		return s.markFailed(ctx, q, processing.ID, "key_failed", err)
	}
	mimeType := "image/jpeg"
	if format == FormatPNG {
		mimeType = "image/png"
	}
	if err := s.storage.PutObjectAtProfile(ctx, asset.StorageProfileID, key, encoded, mimeType); err != nil {
		return s.markFailed(ctx, q, processing.ID, "destination_write_failed", err)
	}
	digest := sha256.Sum256(encoded)
	if _, err := q.MarkMediaDerivativeReady(ctx, dbsqlc.MarkMediaDerivativeReadyParams{
		ID: processing.ID, Format: format, StorageKey: key, MimeType: mimeType, Width: int32(width), Height: int32(height),
		SizeBytes: int64(len(encoded)), ChecksumSha256: hex.EncodeToString(digest[:]),
	}); err != nil {
		return fmt.Errorf("mark media derivative ready: %w", err)
	}
	if err := q.DeleteMediaDerivativeOutbox(ctx, processing.ID); err != nil {
		return fmt.Errorf("delete media derivative outbox: %w", err)
	}
	return nil
}

func (s *Service) markFailed(ctx context.Context, q *dbsqlc.Queries, id int64, code string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if _, err := q.MarkMediaDerivativeFailed(ctx, dbsqlc.MarkMediaDerivativeFailedParams{ID: id, ErrorCode: code, ErrorMessage: message}); err != nil {
		return fmt.Errorf("%s: %v; mark failed: %w", code, cause, err)
	}
	return fmt.Errorf("%s: %w", code, cause)
}

func render(data []byte, maxSide int) ([]byte, int, int, string, error) {
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
