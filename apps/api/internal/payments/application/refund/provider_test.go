package refund

import (
	"errors"
	"testing"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

func TestValidateProviderRefund(t *testing.T) {
	local := paymentsdomain.PaymentRefund{StripeChargeID: "ch_123", StripeRefundID: "", AmountMinor: 250, Currency: "usd"}
	tests := []struct {
		name     string
		provider paymentports.StripeRefund
		wantErr  bool
	}{
		{name: "valid", provider: paymentports.StripeRefund{ID: "re_123", ChargeID: "ch_123", AmountMinor: 250, Currency: "USD"}},
		{name: "missing identity", provider: paymentports.StripeRefund{ChargeID: "ch_123", AmountMinor: 250, Currency: "usd"}, wantErr: true},
		{name: "charge conflict", provider: paymentports.StripeRefund{ID: "re_123", ChargeID: "ch_other", AmountMinor: 250, Currency: "usd"}, wantErr: true},
		{name: "amount conflict", provider: paymentports.StripeRefund{ID: "re_123", ChargeID: "ch_123", AmountMinor: 251, Currency: "usd"}, wantErr: true},
		{name: "currency conflict", provider: paymentports.StripeRefund{ID: "re_123", ChargeID: "ch_123", AmountMinor: 250, Currency: "eur"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateProviderRefund(local, test.provider)
			if test.wantErr {
				if !errors.Is(err, paymentcontracts.ErrRefundProviderConflict) {
					t.Fatalf("error = %v, want ErrRefundProviderConflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateProviderRefundRejectsMismatchedRefundID(t *testing.T) {
	err := ValidateProviderRefund(
		paymentsdomain.PaymentRefund{StripeChargeID: "ch_123", StripeRefundID: "re_existing", AmountMinor: 1, Currency: "usd"},
		paymentports.StripeRefund{ID: "re_other", ChargeID: "ch_123", AmountMinor: 1, Currency: "usd"},
	)
	if !errors.Is(err, paymentcontracts.ErrRefundProviderConflict) {
		t.Fatalf("error = %v, want ErrRefundProviderConflict", err)
	}
}
