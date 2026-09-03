package dispute

import (
	"context"
	"database/sql"
	"fmt"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type CheckoutStore interface {
	WithUserCheckoutLock(context.Context, int64, func(context.Context) error) error
	ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
}

type Service struct {
	checkoutStore    CheckoutStore
	checkoutGateway  paymentports.CheckoutGateway
	orderLifecycle   *paymentorder.PaymentOrderLifecycle
	credentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
}

type Dependencies struct {
	CheckoutStore    CheckoutStore
	CheckoutGateway  paymentports.CheckoutGateway
	OrderLifecycle   *paymentorder.PaymentOrderLifecycle
	CredentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.OrderLifecycle == nil || deps.CredentialAccess == nil {
		return nil, fmt.Errorf("dispute service requires checkout store, gateway, lifecycle, and credential access")
	}
	return &Service{checkoutStore: deps.CheckoutStore, checkoutGateway: deps.CheckoutGateway, orderLifecycle: deps.OrderLifecycle, credentialAccess: deps.CredentialAccess}, nil
}

// ExpireDisputeCheckoutSessions closes active Checkout sessions after a dispute
// restriction is durably committed. The operation is idempotent and safe for
// webhook and reconciliation retries.
func (s *Service) ExpireDisputeCheckoutSessions(ctx context.Context, dispute paymentsdomain.PaymentDispute) error {
	if s == nil || (dispute.Status != paymentsdomain.PaymentDisputeOpen && dispute.Status != paymentsdomain.PaymentDisputeLost) {
		return nil
	}
	return s.checkoutStore.WithUserCheckoutLock(ctx, dispute.UserID, func(locked context.Context) error {
		orders, err := s.checkoutStore.ListActivePaymentOrders(locked, dispute.UserID)
		if err != nil {
			return err
		}
		for _, order := range orders {
			switch order.Status {
			case paymentsdomain.PaymentOrderCreating:
				if _, err := s.orderLifecycle.ExpireOrder(locked, order); err != nil {
					return err
				}
			case paymentsdomain.PaymentOrderCheckoutOpen:
				_, apiKey, err := s.credentialAccess(locked, order.StripeCredentialID)
				if err != nil {
					return err
				}
				expiration, err := s.checkoutGateway.ExpireCheckoutSession(locked, paymentports.ExpireCheckoutSessionInput{APIKey: apiKey, SessionID: order.StripeCheckoutSessionID})
				if err != nil {
					return err
				}
				if _, err := s.orderLifecycle.ConvergeCheckoutExpiration(locked, order, expiration); err != nil {
					return err
				}
			case paymentsdomain.PaymentOrderAsyncPaymentPending:
				// Provider payment submission already happened; normal checkout
				// convergence owns this terminal outcome.
			default:
				return sql.ErrNoRows
			}
		}
		return nil
	})
}
