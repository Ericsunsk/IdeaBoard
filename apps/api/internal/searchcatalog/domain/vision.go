package domain

import (
	"errors"
	"fmt"
	"time"
)

const (
	VisionSearchTimeout = 30 * time.Second
	VisionMaxImages     = 4
	VisionMaxBytes      = 10 << 20
)

var (
	ErrVisionUnavailable      = errors.New("vision search provider is unavailable")
	ErrVisionNotConfigured    = errors.New("vision search provider is not configured")
	ErrVisionInput            = errors.New("vision search input is invalid")
	ErrVisionImageInvalid     = errors.New("vision search image is invalid")
	ErrVisionImageUnavailable = errors.New("vision search image is unavailable")
	ErrVisionSchema           = errors.New("vision search response schema is invalid")
)

// VisionExtraction is the normalized vision understanding result.
type VisionExtraction struct {
	Query string `json:"query"`
}

// VisionImage is one validated vision input image.
type VisionImage struct {
	Filename    string
	ContentType string
	Data        []byte
}

// VisionInput is the validated vision request: images plus free text.
type VisionInput struct {
	Images   []VisionImage
	UserText string
}

// VisionProviderFailure is the machine-readable vision provider failure
// kind, shared by failure classification (application) and HTTP error
// mapping (interfaces).
type VisionProviderFailure string

const (
	VisionProviderFailureNetwork     VisionProviderFailure = "network"
	VisionProviderFailureTimeout     VisionProviderFailure = "timeout"
	VisionProviderFailureCredentials VisionProviderFailure = "credentials"
	VisionProviderFailureQuota       VisionProviderFailure = "quota"
	VisionProviderFailureResponse    VisionProviderFailure = "response"
)

// VisionProviderError carries a provider failure kind with its cause.
type VisionProviderError struct {
	Kind VisionProviderFailure
	Err  error
}

func (e *VisionProviderError) Error() string {
	if e == nil || e.Err == nil {
		return "vision provider request failed"
	}
	return fmt.Sprintf("vision provider request failed: %v", e.Err)
}

// Unwrap exposes the cause for errors.Is/As classification.
func (e *VisionProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
