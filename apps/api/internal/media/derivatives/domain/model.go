package domain

import (
	"errors"
	"fmt"
)

// --- Tier Constants and Specs ---

const (
	TierThumb        = "thumb"
	TierPreview      = "preview"
	TierLargePreview = "large-preview"
	// TierProcessingInput is a provider-neutral derivative intended for
	// external image processing endpoints. It is separate from the editor
	// processing tier so UI sizing changes do not alter provider inputs.
	TierProcessingInput = "processing-input"

	FormatJPEG = "jpeg"
	FormatPNG  = "png"
	FormatWebP = "webp"
)

type TierSpec struct {
	MaxSide int
}

var TierSpecs = map[string]TierSpec{
	// Display tiers mirror the frontend width-to-tier map in
	// apps/web/src/core/media/displayTier.ts (MEDIA_DISPLAY_TIER_MAX_SIDE:
	// thumb 256, preview 640, large-preview 1280); keep both sides in sync.
	TierThumb:           {MaxSide: 256},
	TierPreview:         {MaxSide: 640},
	TierLargePreview:    {MaxSide: 1280},
	TierProcessingInput: {MaxSide: ProcessingInputMaxSide},
}

// --- Input Profile Specifications ---

// InputProfile describes the bounded image contract used when an image is
// sent to an external processing endpoint. It intentionally contains no
// provider or model names: endpoint implementations may be replaced without
// changing the media contract.
type InputProfile struct {
	Name      string
	MaxSide   int
	MaxPixels int64
	MaxBytes  int64
}

const (
	// ProfileProcessingInput is the conservative profile shared by image
	// processing endpoints. A derivative rendered to this profile is safe for
	// providers with tighter limits while remaining useful for higher-capacity
	// providers.
	ProfileProcessingInput   = "processing-input"
	ProcessingInputMaxSide   = 2048
	ProcessingInputMaxPixels = 4 * 1024 * 1024
	ProcessingInputMaxBytes  = 20 * 1024 * 1024
)

var InputProfiles = map[string]InputProfile{
	ProfileProcessingInput: {
		Name:      ProfileProcessingInput,
		MaxSide:   ProcessingInputMaxSide,
		MaxPixels: ProcessingInputMaxPixels,
		MaxBytes:  ProcessingInputMaxBytes,
	},
}

func ProcessingInputProfile() InputProfile {
	return InputProfiles[ProfileProcessingInput]
}

func (p InputProfile) ValidateInputFacts(width, height int, sizeBytes int64) error {
	if p.MaxSide <= 0 || p.MaxPixels <= 0 || p.MaxBytes <= 0 {
		return errors.New(msgValidateInputFacts)
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf(msgValidateInputFacts2, width, height)
	}
	if width > p.MaxSide || height > p.MaxSide {
		return fmt.Errorf(msgValidateInputFacts3, width, height, p.MaxSide)
	}
	if int64(width)*int64(height) > p.MaxPixels {
		return fmt.Errorf(msgValidateInputFacts4, width, height, p.MaxPixels)
	}
	if sizeBytes > p.MaxBytes {
		return fmt.Errorf(msgValidateInputFacts5, sizeBytes, p.MaxBytes)
	}
	return nil
}
