package visionbilling

import (
	"testing"

	"ideaboard/api/internal/platform/riverpolicy"
)

// TestVisionBillingWorkerPinsTimeoutMatrix proves the worker does not fall
// back to the client-level JobTimeout (#783).
func TestVisionBillingWorkerPinsTimeoutMatrix(t *testing.T) {
	if got := (&VisionBillingReconciliationWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutVisionBilling {
		t.Fatalf("vision billing timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutVisionBilling)
	}
}
