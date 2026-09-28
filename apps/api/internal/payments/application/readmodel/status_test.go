package readmodel

import "testing"

// TestRechargeHistoryStatusWireWords pins the history overlay
// vocabulary: typing must not change the strings on the wire.
func TestRechargeHistoryStatusWireWords(t *testing.T) {
	for status, want := range map[RechargeHistoryStatus]string{
		RechargeHistoryCredited: "credited", RechargeHistoryCancelled: "cancelled",
		RechargeHistoryRefunded: "refunded",
	} {
		if string(status) != want {
			t.Errorf("RechargeHistoryStatus = %q, want %q", string(status), want)
		}
	}
}
