package domain

const (
	TierThumb        = "thumb"
	TierPreview      = "preview"
	TierLargePreview = "large-preview"
	TierProcessing   = "processing"
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
	TierThumb:           {MaxSide: 256},
	TierPreview:         {MaxSide: 640},
	TierLargePreview:    {MaxSide: 1280},
	TierProcessing:      {MaxSide: 2560},
	TierProcessingInput: {MaxSide: ProcessingInputMaxSide},
}
