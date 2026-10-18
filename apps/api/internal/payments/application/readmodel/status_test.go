package readmodel_test

import (
	"ideaboard/api/internal/payments/application/readmodel"
	"testing"
)

// TestRechargeHistoryStatusWireWords pins the history overlay
// vocabulary: typing must not change the strings on the wire.
func TestRechargeHistoryStatusWireWords(t *testing.T) {
	for status, want := range map[readmodel.RechargeHistoryStatus]string{
		readmodel.RechargeHistoryCredited: "credited", readmodel.RechargeHistoryCancelled: "cancelled",
		readmodel.RechargeHistoryRefunded: "refunded",
	} {
		if string(status) != want {
			t.Errorf("RechargeHistoryStatus = %q, want %q", string(status), want)
		}
	}
}
