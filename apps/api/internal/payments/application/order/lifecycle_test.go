package order

import "testing"

func TestSignalForWebhookEvent(t *testing.T) {
	tests := []struct {
		eventType string
		want      CheckoutSignal
		ok        bool
	}{
		{eventType: "checkout.session.completed", want: SignalCompleted, ok: true},
		{eventType: "checkout.session.async_payment_succeeded", want: SignalAsyncSuccess, ok: true},
		{eventType: "checkout.session.async_payment_failed", want: SignalAsyncFailed, ok: true},
		{eventType: "checkout.session.expired", want: SignalExpired, ok: true},
		{eventType: "charge.succeeded", ok: false},
	}

	for _, tt := range tests {
		got, ok := SignalForWebhookEvent(tt.eventType)
		if got != tt.want || ok != tt.ok {
			t.Errorf("SignalForWebhookEvent(%q) = (%q, %t), want (%q, %t)", tt.eventType, got, ok, tt.want, tt.ok)
		}
	}
}
