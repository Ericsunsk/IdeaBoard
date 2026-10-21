package dispute

import (
	"context"
	"fmt"
	"ideaboard/api/internal/platform/database"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type CheckoutStore interface {
	WithBillingAccountCheckoutLock(context.Context, int64, func(context.Context) error) error
	ListAccountActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByID(context.Context, string) (paymentsdomain.PaymentOrder, error)
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
//
// #920: expiry targets the dispute order's fixed payer account, never the
// operator's personal scope, so member exits and management changes never
// move the obligation. Stripe I/O runs inside the account advisory lock with
// short transactions only, never holding a local transaction.
func (s *Service) ExpireDisputeCheckoutSessions(ctx context.Context, dispute paymentsdomain.PaymentDispute) error {
	if s == nil || (dispute.Status != paymentsdomain.PaymentDisputeOpen && dispute.Status != paymentsdomain.PaymentDisputeLost) {
		return nil
	}
	order, err := s.checkoutStore.GetPaymentOrderByID(ctx, dispute.PaymentOrderID)
	if err != nil {
		return fmt.Errorf("resolve dispute payer: %w", err)
	}
	if order.BillingAccountID <= 0 {
		return fmt.Errorf("resolve dispute payer: invalid Billing Account")
	}
	return s.expireAccountCheckoutSessions(ctx, order.BillingAccountID)
}

// expireAccountCheckoutSessions expires the payer account's active checkouts
// under the per-account advisory lock (#919).
func (s *Service) expireAccountCheckoutSessions(ctx context.Context, billingAccountID int64) error {
	return s.checkoutStore.WithBillingAccountCheckoutLock(ctx, billingAccountID, func(locked context.Context) error {
		orders, err := s.checkoutStore.ListAccountActivePaymentOrders(locked, billingAccountID)
		if err != nil {
			return err
		}
		return s.expireListedCheckoutSessions(locked, orders)
	})
}

func (s *Service) expireListedCheckoutSessions(locked context.Context, orders []paymentsdomain.PaymentOrder) error {
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
			return database.ErrNotFound
		}
	}
	return nil
}
