package ports_test

import (
	"ideaboard/api/internal/payments/ports"
	"testing"
)

// TestPaymentEventStatusWireWords pins the user-facing hint vocabulary:
// typing must not change the strings on the wire.
func TestPaymentEventStatusWireWords(t *testing.T) {
	for status, want := range map[ports.PaymentEventStatus]string{
		ports.PaymentEventProcessing: "processing", ports.PaymentEventCredited: "credited",
		ports.PaymentEventFailed: "failed", ports.PaymentEventRefundUpdated: "refund_updated",
		ports.PaymentEventDisputeUpdated: "dispute_updated",
	} {
		if string(status) != want {
			t.Errorf("PaymentEventStatus = %q, want %q", string(status), want)
		}
	}
}
