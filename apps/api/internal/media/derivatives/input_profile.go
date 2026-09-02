package derivatives

// InputProfile describes the bounded image contract used when an image is
// sent to an external processing endpoint.  It intentionally contains no
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
	// processing endpoints.  A derivative rendered to this profile is safe for
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
