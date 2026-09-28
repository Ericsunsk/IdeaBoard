package domain

import "testing"

// TestCanOrderTransitionMirrorsSQLGuards pins the durable order
// transition table to guard_payment_order_lifecycle in
// internal/db/schema/fragments/payments.sql. Every true entry was
// already possible before typing.
func TestCanOrderTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]PaymentOrderStatus{
		{PaymentOrderCreating, PaymentOrderCheckoutOpen},
		{PaymentOrderCreating, PaymentOrderPaidPendingCredit},
		{PaymentOrderCreating, PaymentOrderExpired},
		{PaymentOrderCreating, PaymentOrderFailed},
		{PaymentOrderCheckoutOpen, PaymentOrderAsyncPaymentPending},
		{PaymentOrderCheckoutOpen, PaymentOrderPaidPendingCredit},
		{PaymentOrderCheckoutOpen, PaymentOrderCancelled},
		{PaymentOrderCheckoutOpen, PaymentOrderExpired},
		{PaymentOrderCheckoutOpen, PaymentOrderFailed},
		{PaymentOrderAsyncPaymentPending, PaymentOrderPaidPendingCredit},
		{PaymentOrderAsyncPaymentPending, PaymentOrderExpired},
		{PaymentOrderAsyncPaymentPending, PaymentOrderFailed},
		{PaymentOrderPaidPendingCredit, PaymentOrderCredited},
		{PaymentOrderCancelled, PaymentOrderPaidPendingCredit},
		{PaymentOrderExpired, PaymentOrderPaidPendingCredit},
		{PaymentOrderFailed, PaymentOrderPaidPendingCredit},
	}
	for _, transition := range allowed {
		if !CanOrderTransition(transition[0], transition[1]) {
			t.Errorf("CanOrderTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if CanOrderTransition(PaymentOrderCredited, PaymentOrderPaidPendingCredit) {
		t.Error("CanOrderTransition(credited, paid_pending_credit) = true, want false (credited is terminal)")
	}
	for _, to := range []PaymentOrderStatus{
		PaymentOrderCreating, PaymentOrderCheckoutOpen, PaymentOrderAsyncPaymentPending,
		PaymentOrderCredited, PaymentOrderCancelled, PaymentOrderExpired, PaymentOrderFailed,
	} {
		if CanOrderTransition(PaymentOrderCredited, to) {
			t.Errorf("CanOrderTransition(credited, %q) = true, want false (credited is terminal)", to)
		}
	}
	if CanOrderTransition(PaymentOrderCreating, PaymentOrderCreating) {
		t.Error("CanOrderTransition(creating, creating) = true, want false (no self-transition guard exists)")
	}
	if CanOrderTransition(PaymentOrderCreating, PaymentOrderCancelled) {
		t.Error("CanOrderTransition(creating, cancelled) = true, want false (cancel only leaves checkout_open)")
	}
	if CanOrderTransition(PaymentOrderCheckoutOpen, PaymentOrderCredited) {
		t.Error("CanOrderTransition(checkout_open, credited) = true, want false (credit passes through paid_pending_credit)")
	}
}

// TestCanRefundTransitionMirrorsSQLGuards pins the durable refund
// transition table to enforce_payment_refund_transition in
// internal/db/schema/fragments/payments.sql. anomaly is insert-only
// and never a transition target, so it has no incoming edge.
func TestCanRefundTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]PaymentRefundStatus{
		{PaymentRefundPending, PaymentRefundPending},
		{PaymentRefundPending, PaymentRefundSucceeded},
		{PaymentRefundPending, PaymentRefundFailed},
	}
	for _, transition := range allowed {
		if !CanRefundTransition(transition[0], transition[1]) {
			t.Errorf("CanRefundTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if CanRefundTransition(PaymentRefundPending, PaymentRefundAnomaly) {
		t.Error("CanRefundTransition(pending, anomaly) = true, want false (anomaly is insert-only, never an UPDATE target)")
	}
	terminals := []PaymentRefundStatus{PaymentRefundSucceeded, PaymentRefundFailed, PaymentRefundAnomaly}
	live := []PaymentRefundStatus{PaymentRefundPending, PaymentRefundSucceeded, PaymentRefundFailed, PaymentRefundAnomaly}
	for _, from := range terminals {
		for _, to := range live {
			if CanRefundTransition(from, to) {
				t.Errorf("CanRefundTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
}

// TestCanDisputeTransitionMirrorsSQLGuards pins the durable dispute
// transition table to guard_payment_dispute_lifecycle in
// internal/db/schema/fragments/integrity.sql.
func TestCanDisputeTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]PaymentDisputeStatus{
		{PaymentDisputeOpen, PaymentDisputeWon},
		{PaymentDisputeOpen, PaymentDisputeLost},
		{PaymentDisputeLost, PaymentDisputeWon},
	}
	for _, transition := range allowed {
		if !CanDisputeTransition(transition[0], transition[1]) {
			t.Errorf("CanDisputeTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if CanDisputeTransition(PaymentDisputeWon, PaymentDisputeLost) {
		t.Error("CanDisputeTransition(won, lost) = true, want false (won is terminal)")
	}
	if CanDisputeTransition(PaymentDisputeLost, PaymentDisputeLost) {
		t.Error("CanDisputeTransition(lost, lost) = true, want false (no self-transition guard exists)")
	}
	if CanDisputeTransition(PaymentDisputeOpen, PaymentDisputeOpen) {
		t.Error("CanDisputeTransition(open, open) = true, want false (no self-transition guard exists)")
	}
	for _, to := range []PaymentDisputeStatus{PaymentDisputeOpen, PaymentDisputeWon, PaymentDisputeLost} {
		if CanDisputeTransition(PaymentDisputeWon, to) {
			t.Errorf("CanDisputeTransition(won, %q) = true, want false (won is terminal)", to)
		}
	}
}

// TestStatusWireWords pins the wire/DB words: typing must not change
// the strings on the wire or in the database.
func TestStatusWireWords(t *testing.T) {
	for status, want := range map[PaymentOrderStatus]string{
		PaymentOrderCreating: "creating", PaymentOrderCheckoutOpen: "checkout_open",
		PaymentOrderAsyncPaymentPending: "async_payment_pending",
		PaymentOrderPaidPendingCredit:   "paid_pending_credit",
		PaymentOrderCredited:            "credited", PaymentOrderCancelled: "cancelled",
		PaymentOrderExpired: "expired", PaymentOrderFailed: "payment_failed",
	} {
		if string(status) != want {
			t.Errorf("PaymentOrderStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[PaymentRefundStatus]string{
		PaymentRefundPending: "pending", PaymentRefundSucceeded: "succeeded",
		PaymentRefundFailed: "failed", PaymentRefundAnomaly: "anomaly",
	} {
		if string(status) != want {
			t.Errorf("PaymentRefundStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[PaymentDisputeStatus]string{
		PaymentDisputeOpen: "open", PaymentDisputeWon: "won", PaymentDisputeLost: "lost",
	} {
		if string(status) != want {
			t.Errorf("PaymentDisputeStatus = %q, want %q", string(status), want)
		}
	}
}
