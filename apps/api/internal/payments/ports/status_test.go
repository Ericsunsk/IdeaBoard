package ports

import "testing"

// TestPaymentEventStatusWireWords pins the user-facing hint vocabulary:
// typing must not change the strings on the wire.
func TestPaymentEventStatusWireWords(t *testing.T) {
	for status, want := range map[PaymentEventStatus]string{
		PaymentEventProcessing: "processing", PaymentEventCredited: "credited",
		PaymentEventFailed: "failed", PaymentEventRefundUpdated: "refund_updated",
		PaymentEventDisputeUpdated: "dispute_updated",
	} {
		if string(status) != want {
			t.Errorf("PaymentEventStatus = %q, want %q", string(status), want)
		}
	}
}
