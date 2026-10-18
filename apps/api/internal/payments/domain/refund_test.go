package domain_test

import (
	"ideaboard/api/internal/payments/domain"
	"testing"
)

func TestProportionalRefundIdeas(t *testing.T) {
	tests := []struct {
		name                         string
		amountMinor, subtotal, ideas int64
		want                         int64
	}{
		{name: "full refund", amountMinor: 100, subtotal: 100, ideas: 10, want: 10},
		{name: "fractional refund floors", amountMinor: 33, subtotal: 100, ideas: 10, want: 3},
		{name: "invalid input", amountMinor: 0, subtotal: 100, ideas: 10, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := domain.ProportionalRefundIdeas(test.amountMinor, test.subtotal, test.ideas); got != test.want {
				t.Fatalf("ProportionalRefundIdeas() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestProportionalRefundAmount(t *testing.T) {
	if got := domain.ProportionalRefundAmount(10, 3, 0, 1); got != 3 {
		t.Fatalf("first refund amount = %d, want 3", got)
	}
	if got := domain.ProportionalRefundAmount(10, 3, 1, 1); got != 3 {
		t.Fatalf("second refund amount = %d, want 3", got)
	}
	if got := domain.ProportionalRefundAmount(10, 3, 2, 1); got != 4 {
		t.Fatalf("third refund amount = %d, want 4", got)
	}
}
