package workspacemedia

import (
	"context"
	"fmt"
	"strings"

	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	derivativeports "ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/imageformat"
	storageports "ideaboard/api/internal/media/storage/ports"
)

// DerivativeObjects is the narrow byte seam derivative generation
// needs: read the original, write rendered variants. *storage.Service
// satisfies it in production; tests inject fakes.
type DerivativeObjects interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error)
	PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error
}

// displayVariants are the v1-facing tiers generation materializes, in
// render order. processing-input is provider-internal (never a v1
// display tier) and is intentionally excluded.
var displayVariants = []string{
	derivativedomain.TierThumb,
	derivativedomain.TierPreview,
	derivativedomain.TierLargePreview,
}

// EnsureDerivatives generates display derivatives for an image asset
// and records each variant row. Generation is SEQUENTIAL by design:
// renders are memory-heavy, variants are few (three), and sequential
// execution keeps failure attribution trivial (the returned error
// names the failed variant; already-recorded tiers stay and later
// runs converge via upsert). Callers treat failures as best-effort:
// the asset is already ready and tiers without rows simply stay
// pending (409 at the derivative-bytes endpoint).
//
// Non-image assets are a no-op (nil, nil): only image has a render
// seam. A missing original reads back as
// ErrDerivativeSourceMissing (409-class, never 500).
func EnsureDerivatives(ctx context.Context, store AssetStore, objects DerivativeObjects, renderer derivativeports.Renderer, workspaceID string, asset *Asset) ([]*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if asset == nil || store == nil || objects == nil || renderer == nil {
		return nil, fmt.Errorf("workspacemedia: derivative generation dependencies are not configured")
	}
	if strings.TrimSpace(asset.MediaKind) != MediaKindImage {
		return nil, nil
	}
	if strings.TrimSpace(asset.StorageKey) == "" {
		return nil, fmt.Errorf("workspacemedia: asset %s has no storage key: %w", asset.ID, ErrDerivativeSourceMissing)
	}
	original, err := objects.ReadObjectAtProfile(ctx, DefaultStorageProfile, asset.StorageKey, derivativedomain.MaxSourceBytes)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: read derivative source: %w", ErrDerivativeSourceMissing)
	}
	if original == nil || len(original.Data) == 0 {
		return nil, fmt.Errorf("workspacemedia: empty derivative source: %w", ErrDerivativeSourceMissing)
	}
	var out []*Derivative
	for _, variant := range displayVariants {
		spec, ok := derivativedomain.TierSpecs[variant]
		if !ok {
			continue
		}
		record, err := renderVariant(ctx, store, objects, renderer, workspaceID, asset, original.Data, variant, spec.MaxSide)
		if err != nil {
			return out, fmt.Errorf("workspacemedia: render derivative %s: %w", variant, err)
		}
		out = append(out, record)
	}
	return out, nil
}

func renderVariant(ctx context.Context, store AssetStore, objects DerivativeObjects, renderer derivativeports.Renderer, workspaceID string, asset *Asset, source []byte, variant string, maxSide int) (*Derivative, error) {
	rendered, width, height, mimeType, err := renderer.Render(ctx, source, asset.ID, maxSide)
	if err != nil {
		return nil, err
	}
	if len(rendered) == 0 || len(rendered) > derivativedomain.MaxOutputBytes {
		return nil, fmt.Errorf("workspacemedia: rendered derivative %s has invalid size %d", variant, len(rendered))
	}
	mime := strings.TrimSpace(mimeType)
	if mime == "" {
		mime = derivativedomain.DerivativeMimeType(derivativedomain.FormatWebP)
	}
	ext := imageformat.ImageExtensionFromMIME(mime)
	if ext == "" {
		ext = ".webp"
	}
	slot := DerivativeSlotKey(workspaceID, asset.ID, variant, ext)
	if err := objects.PutObjectAtProfile(ctx, DefaultStorageProfile, slot, rendered, mime); err != nil {
		return nil, fmt.Errorf("workspacemedia: store derivative %s: %w", variant, err)
	}
	record, err := store.RecordDerivative(ctx, workspaceID, asset.ID, Derivative{
		VariantName: variant,
		MIMEType:    mime,
		Width:       int32(width),
		Height:      int32(height),
		SizeBytes:   int64(len(rendered)),
		StorageKey:  slot,
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}
