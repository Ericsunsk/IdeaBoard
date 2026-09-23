package visionbilling

import (
	"testing"

	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

// TestVisionBillingWorkerPinsTimeoutMatrix proves the worker does not fall
// back to the client-level JobTimeout (#783).
func TestVisionBillingWorkerPinsTimeoutMatrix(t *testing.T) {
	if got := (&VisionBillingReconciliationWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutVisionBilling {
		t.Fatalf("vision billing timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutVisionBilling)
	}
}
