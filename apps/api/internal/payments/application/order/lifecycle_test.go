package order_test

import (
	"ideaboard/api/internal/payments/application/order"
	"testing"
)

func TestSignalForWebhookEvent(t *testing.T) {
	tests := []struct {
		eventType string
		want      order.CheckoutSignal
		ok        bool
	}{
		{eventType: "checkout.session.completed", want: order.SignalCompleted, ok: true},
		{eventType: "checkout.session.async_payment_succeeded", want: order.SignalAsyncSuccess, ok: true},
		{eventType: "checkout.session.async_payment_failed", want: order.SignalAsyncFailed, ok: true},
		{eventType: "checkout.session.expired", want: order.SignalExpired, ok: true},
		{eventType: "charge.succeeded", ok: false},
	}

	for _, tt := range tests {
		got, ok := order.SignalForWebhookEvent(tt.eventType)
		if got != tt.want || ok != tt.ok {
			t.Errorf("SignalForWebhookEvent(%q) = (%q, %t), want (%q, %t)", tt.eventType, got, ok, tt.want, tt.ok)
		}
	}
}
