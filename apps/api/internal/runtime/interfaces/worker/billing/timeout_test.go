package billing

import (
	"testing"

	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

// TestBillingWorkersPinTimeoutMatrix proves no billing worker falls back to
// the client-level JobTimeout: every Timeout() references the River timeout
// matrix (#783).
func TestBillingWorkersPinTimeoutMatrix(t *testing.T) {
	if got := (&CreditReconciliationWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutCreditReconciliation {
		t.Fatalf("credit reconciliation timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutCreditReconciliation)
	}
	if got := (&CreditReconciliationScheduleWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutCreditSchedule {
		t.Fatalf("credit schedule timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutCreditSchedule)
	}
	if got := (&PaymentReconciliationWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutPaymentReconciliation {
		t.Fatalf("payment reconciliation timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutPaymentReconciliation)
	}
	if got := (&PaymentCheckoutShutdownWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutPaymentCheckoutShutdown {
		t.Fatalf("checkout shutdown timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutPaymentCheckoutShutdown)
	}
	if got := (&PaymentCheckoutShutdownScheduleWorker{}).Timeout(nil); got != taskqueue.RiverJobTimeoutPaymentCheckoutShutdown {
		t.Fatalf("checkout shutdown schedule timeout = %s, want matrix %s", got, taskqueue.RiverJobTimeoutPaymentCheckoutShutdown)
	}
}
