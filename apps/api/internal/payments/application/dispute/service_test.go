package dispute

import (
	"context"
	"errors"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"testing"
)

func TestExpiryNeverFallsBackFromOriginalPayer(t *testing.T) {
	readFailure := errors.New("order read unavailable")
	for _, status := range []paymentsdomain.PaymentDisputeStatus{paymentsdomain.PaymentDisputeOpen, paymentsdomain.PaymentDisputeLost} {
		for _, tc := range []struct {
			name    string
			account int64
			readErr error
		}{
			{name: "transient read failure", readErr: readFailure},
			{name: "missing payer"},
			{name: "original payer", account: 73},
		} {
			t.Run(string(status)+"/"+tc.name, func(t *testing.T) {
				store := &expiryStore{account: tc.account, readErr: tc.readErr}
				service := &Service{checkoutStore: store}
				err := service.ExpireDisputeCheckoutSessions(t.Context(), paymentsdomain.PaymentDispute{Status: status, PaymentOrderID: "original-order", UserID: 42})
				if tc.account == 0 {
					if err == nil || store.lockedAccount != 0 || store.listedAccount != 0 {
						t.Fatalf("unsafe expiry: err=%v store=%+v", err, store)
					}
					if tc.readErr != nil && !errors.Is(err, tc.readErr) {
						t.Fatalf("read error lost: %v", err)
					}
				} else if err != nil || store.lockedAccount != 73 || store.listedAccount != 73 {
					t.Fatalf("payer expiry: err=%v store=%+v", err, store)
				}
			})
		}
	}
}

type expiryStore struct {
	account       int64
	readErr       error
	lockedAccount int64
	listedAccount int64
}

func (s *expiryStore) GetPaymentOrderByID(context.Context, string) (paymentsdomain.PaymentOrder, error) {
	return paymentsdomain.PaymentOrder{BillingAccountID: s.account}, s.readErr
}
func (s *expiryStore) WithBillingAccountCheckoutLock(ctx context.Context, account int64, run func(context.Context) error) error {
	s.lockedAccount = account
	return run(ctx)
}
func (s *expiryStore) ListAccountActivePaymentOrders(_ context.Context, account int64) ([]paymentsdomain.PaymentOrder, error) {
	s.listedAccount = account
	return nil, nil
}
