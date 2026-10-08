package river

import (
	"testing"

	"ideaboard/api/internal/platform/riverpolicy"
)

// TestRiverClientFallbackPinsTimeoutMatrix proves the client-level
// JobTimeout stays on the matrix drawing value now that the legacy drawing
// and settlement-only final-failure workers are deleted (#783 pins live on
// the timeout matrix itself).
func TestRiverClientFallbackPinsTimeoutMatrix(t *testing.T) {
	if got := defaultRiverJobTimeout; got != riverpolicy.RiverJobTimeoutDrawing {
		t.Fatalf("client fallback timeout = %s, want matrix drawing %s", got, riverpolicy.RiverJobTimeoutDrawing)
	}
}
