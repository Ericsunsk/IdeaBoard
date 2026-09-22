package domain

import (
	"context"
	"errors"
	"testing"
)

func TestVisionContractConstantsStayBounded(t *testing.T) {
	if VisionSearchTimeout <= 0 || VisionMaxImages < 1 || VisionMaxBytes < 1 {
		t.Fatalf("invalid vision limits: timeout=%s images=%d bytes=%d", VisionSearchTimeout, VisionMaxImages, VisionMaxBytes)
	}
}

func TestVisionProviderErrorPreservesKindAndCause(t *testing.T) {
	cause := context.DeadlineExceeded
	err := &VisionProviderError{Kind: VisionProviderFailureTimeout, Err: cause}
	if err.Kind != VisionProviderFailureTimeout || !errors.Is(err, cause) {
		t.Fatalf("provider error = %+v, want timeout kind and wrapped cause", err)
	}
	if (&VisionProviderError{}).Error() == "" {
		t.Fatal("nil-cause provider error must retain a safe message")
	}
}
