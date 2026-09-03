package domain

import "testing"

func TestCanonicalPaymentDisputeStatus(t *testing.T) {
	tests := map[string]PaymentDisputeStatus{
		"won":            PaymentDisputeWon,
		"warning_closed": PaymentDisputeWon,
		"lost":           PaymentDisputeLost,
		"under_review":   PaymentDisputeOpen,
		"":               PaymentDisputeOpen,
	}
	for providerStatus, want := range tests {
		if got := CanonicalPaymentDisputeStatus(providerStatus); got != want {
			t.Errorf("CanonicalPaymentDisputeStatus(%q) = %q, want %q", providerStatus, got, want)
		}
	}
}

func TestProportionalDisputeIdeas(t *testing.T) {
	tests := []struct {
		name                          string
		amountMinor, paidTotal, ideas int64
		want                          int64
	}{
		{name: "full dispute", amountMinor: 100, paidTotal: 100, ideas: 10, want: 10},
		{name: "fractional dispute rounds up", amountMinor: 1, paidTotal: 3, ideas: 10, want: 4},
		{name: "invalid input", amountMinor: 0, paidTotal: 100, ideas: 10, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ProportionalDisputeIdeas(test.amountMinor, test.paidTotal, test.ideas); got != test.want {
				t.Fatalf("ProportionalDisputeIdeas() = %d, want %d", got, test.want)
			}
		})
	}
}
