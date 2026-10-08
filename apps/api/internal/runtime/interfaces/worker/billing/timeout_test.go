package billing

import (
	"testing"

	"ideaboard/api/internal/platform/riverpolicy"
)

// TestBillingWorkersPinTimeoutMatrix proves no billing worker falls back to
// the client-level JobTimeout: every Timeout() references the River timeout
// matrix (#783).
func TestBillingWorkersPinTimeoutMatrix(t *testing.T) {
	if got := (&CreditReconciliationWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutCreditReconciliation {
		t.Fatalf("credit reconciliation timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutCreditReconciliation)
	}
	if got := (&CreditReconciliationScheduleWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutCreditSchedule {
		t.Fatalf("credit schedule timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutCreditSchedule)
	}
	if got := (&PaymentReconciliationWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutPaymentReconciliation {
		t.Fatalf("payment reconciliation timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutPaymentReconciliation)
	}
	if got := (&PaymentCheckoutShutdownWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutPaymentCheckoutShutdown {
		t.Fatalf("checkout shutdown timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutPaymentCheckoutShutdown)
	}
	if got := (&PaymentCheckoutShutdownScheduleWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutPaymentCheckoutShutdown {
		t.Fatalf("checkout shutdown schedule timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutPaymentCheckoutShutdown)
	}
}
