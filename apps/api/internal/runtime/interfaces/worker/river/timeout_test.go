package river

import (
	"testing"

	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

// TestRiverWorkersPinTimeoutMatrix proves neither drawing worker falls back
// to the client-level JobTimeout: the drawing worker pins the same 17min
// value explicitly, and the settlement-only final-failure worker pins its
// short budget (#783).
func TestRiverWorkersPinTimeoutMatrix(t *testing.T) {
	if got := (&TaskWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutDrawing {
		t.Fatalf("drawing timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutDrawing)
	}
	if got := defaultRiverJobTimeout; got != taskqueue.RiverJobTimeoutDrawing {
		t.Fatalf("client fallback timeout = %s, want matrix drawing %s", got, taskqueue.RiverJobTimeoutDrawing)
	}
	if got := (&FinalFailureWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutFinalFailure {
		t.Fatalf("final failure timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutFinalFailure)
	}
}
