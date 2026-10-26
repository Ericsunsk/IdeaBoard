package domain_test

import (
	"ideaboard/api/internal/payments/domain"
	"testing"
)

// TestCanOrderTransitionMirrorsSQLGuards pins the durable order
// transition table to guard_payment_order_lifecycle in
// internal/db/schema/fragments/payments.sql. Every true entry was
// already possible before typing.
func TestCanOrderTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]domain.PaymentOrderStatus{
		{domain.PaymentOrderCreating, domain.PaymentOrderCheckoutOpen},
		{domain.PaymentOrderCreating, domain.PaymentOrderPaidPendingCredit},
		{domain.PaymentOrderCreating, domain.PaymentOrderExpired},
		{domain.PaymentOrderCreating, domain.PaymentOrderFailed},
		{domain.PaymentOrderCheckoutOpen, domain.PaymentOrderAsyncPaymentPending},
		{domain.PaymentOrderCheckoutOpen, domain.PaymentOrderPaidPendingCredit},
		{domain.PaymentOrderCheckoutOpen, domain.PaymentOrderCancelled},
		{domain.PaymentOrderCheckoutOpen, domain.PaymentOrderExpired},
		{domain.PaymentOrderCheckoutOpen, domain.PaymentOrderFailed},
		{domain.PaymentOrderAsyncPaymentPending, domain.PaymentOrderPaidPendingCredit},
		{domain.PaymentOrderAsyncPaymentPending, domain.PaymentOrderExpired},
		{domain.PaymentOrderAsyncPaymentPending, domain.PaymentOrderFailed},
		{domain.PaymentOrderPaidPendingCredit, domain.PaymentOrderCredited},
		{domain.PaymentOrderCancelled, domain.PaymentOrderPaidPendingCredit},
		{domain.PaymentOrderExpired, domain.PaymentOrderPaidPendingCredit},
		{domain.PaymentOrderFailed, domain.PaymentOrderPaidPendingCredit},
	}
	for _, transition := range allowed {
		if !domain.CanOrderTransition(transition[0], transition[1]) {
			t.Errorf("CanOrderTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if domain.CanOrderTransition(domain.PaymentOrderCredited, domain.PaymentOrderPaidPendingCredit) {
		t.Error("CanOrderTransition(credited, paid_pending_credit) = true, want false (credited is terminal)")
	}
	for _, to := range []domain.PaymentOrderStatus{
		domain.PaymentOrderCreating, domain.PaymentOrderCheckoutOpen, domain.PaymentOrderAsyncPaymentPending,
		domain.PaymentOrderCredited, domain.PaymentOrderCancelled, domain.PaymentOrderExpired, domain.PaymentOrderFailed,
	} {
		if domain.CanOrderTransition(domain.PaymentOrderCredited, to) {
			t.Errorf("CanOrderTransition(credited, %q) = true, want false (credited is terminal)", to)
		}
	}
	if domain.CanOrderTransition(domain.PaymentOrderCreating, domain.PaymentOrderCreating) {
		t.Error("CanOrderTransition(creating, creating) = true, want false (no self-transition guard exists)")
	}
	if domain.CanOrderTransition(domain.PaymentOrderCreating, domain.PaymentOrderCancelled) {
		t.Error("CanOrderTransition(creating, cancelled) = true, want false (cancel only leaves checkout_open)")
	}
	if domain.CanOrderTransition(domain.PaymentOrderCheckoutOpen, domain.PaymentOrderCredited) {
		t.Error("CanOrderTransition(checkout_open, credited) = true, want false (credit passes through paid_pending_credit)")
	}
}

// TestCanRefundTransitionMirrorsSQLGuards pins the durable refund
// transition table to enforce_payment_refund_transition in
// internal/db/schema/fragments/payments.sql. anomaly is insert-only
// and never a transition target, so it has no incoming edge.
func TestCanRefundTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]domain.PaymentRefundStatus{
		{domain.PaymentRefundPending, domain.PaymentRefundPending},
		{domain.PaymentRefundPending, domain.PaymentRefundSucceeded},
		{domain.PaymentRefundPending, domain.PaymentRefundFailed},
	}
	for _, transition := range allowed {
		if !domain.CanRefundTransition(transition[0], transition[1]) {
			t.Errorf("CanRefundTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if domain.CanRefundTransition(domain.PaymentRefundPending, domain.PaymentRefundAnomaly) {
		t.Error("CanRefundTransition(pending, anomaly) = true, want false (anomaly is insert-only, never an UPDATE target)")
	}
	terminals := []domain.PaymentRefundStatus{domain.PaymentRefundSucceeded, domain.PaymentRefundFailed, domain.PaymentRefundAnomaly}
	live := []domain.PaymentRefundStatus{domain.PaymentRefundPending, domain.PaymentRefundSucceeded, domain.PaymentRefundFailed, domain.PaymentRefundAnomaly}
	for _, from := range terminals {
		for _, to := range live {
			if domain.CanRefundTransition(from, to) {
				t.Errorf("CanRefundTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
}

// TestCanDisputeTransitionMirrorsSQLGuards pins the durable dispute
// transition table to guard_payment_dispute_lifecycle in
// internal/db/schema/fragments/payments.sql.
func TestCanDisputeTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]domain.PaymentDisputeStatus{
		{domain.PaymentDisputeOpen, domain.PaymentDisputeWon},
		{domain.PaymentDisputeOpen, domain.PaymentDisputeLost},
		{domain.PaymentDisputeLost, domain.PaymentDisputeWon},
	}
	for _, transition := range allowed {
		if !domain.CanDisputeTransition(transition[0], transition[1]) {
			t.Errorf("CanDisputeTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	if domain.CanDisputeTransition(domain.PaymentDisputeWon, domain.PaymentDisputeLost) {
		t.Error("CanDisputeTransition(won, lost) = true, want false (won is terminal)")
	}
	if domain.CanDisputeTransition(domain.PaymentDisputeLost, domain.PaymentDisputeLost) {
		t.Error("CanDisputeTransition(lost, lost) = true, want false (no self-transition guard exists)")
	}
	if domain.CanDisputeTransition(domain.PaymentDisputeOpen, domain.PaymentDisputeOpen) {
		t.Error("CanDisputeTransition(open, open) = true, want false (no self-transition guard exists)")
	}
	for _, to := range []domain.PaymentDisputeStatus{domain.PaymentDisputeOpen, domain.PaymentDisputeWon, domain.PaymentDisputeLost} {
		if domain.CanDisputeTransition(domain.PaymentDisputeWon, to) {
			t.Errorf("CanDisputeTransition(won, %q) = true, want false (won is terminal)", to)
		}
	}
}

// TestStatusWireWords pins the wire/DB words: typing must not change
// the strings on the wire or in the database.
func TestStatusWireWords(t *testing.T) {
	for status, want := range map[domain.PaymentOrderStatus]string{
		domain.PaymentOrderCreating: "creating", domain.PaymentOrderCheckoutOpen: "checkout_open",
		domain.PaymentOrderAsyncPaymentPending: "async_payment_pending",
		domain.PaymentOrderPaidPendingCredit:   "paid_pending_credit",
		domain.PaymentOrderCredited:            "credited", domain.PaymentOrderCancelled: "cancelled",
		domain.PaymentOrderExpired: "expired", domain.PaymentOrderFailed: "payment_failed",
	} {
		if string(status) != want {
			t.Errorf("PaymentOrderStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[domain.PaymentRefundStatus]string{
		domain.PaymentRefundPending: "pending", domain.PaymentRefundSucceeded: "succeeded",
		domain.PaymentRefundFailed: "failed", domain.PaymentRefundAnomaly: "anomaly",
	} {
		if string(status) != want {
			t.Errorf("PaymentRefundStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[domain.PaymentDisputeStatus]string{
		domain.PaymentDisputeOpen: "open", domain.PaymentDisputeWon: "won", domain.PaymentDisputeLost: "lost",
	} {
		if string(status) != want {
			t.Errorf("PaymentDisputeStatus = %q, want %q", string(status), want)
		}
	}
}
