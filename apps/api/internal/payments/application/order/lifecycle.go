package order

import (
	"context"
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

// ConvergenceStore is the persistence seam required by PaymentOrderLifecycle.
// Implementations must preserve the repository's transaction and replay semantics.
// BillingAccountID scopes abandon to the persisted payer account (#919).
type ConvergenceStore interface {
	MarkPaymentOrderAsyncPending(context.Context, string) (paymentsdomain.PaymentOrder, error)
	MarkPaymentOrderFailed(context.Context, string) (paymentsdomain.PaymentOrder, error)
	MarkPaymentOrderExpired(context.Context, string) (paymentsdomain.PaymentOrder, bool, error)
	MarkPaymentOrderCancelled(context.Context, int64, string) (paymentsdomain.PaymentOrder, bool, error)
	ConvergePaidPaymentOrder(context.Context, paymentsdomain.PaidCheckoutFacts, time.Time) (paymentsdomain.PaymentConvergenceResult, error)
	CreditPaidPaymentOrder(context.Context, string, time.Time) (paymentsdomain.PaymentConvergenceResult, error)
}

type CheckoutSignal string

const (
	SignalObserved     CheckoutSignal = "observed"
	SignalCompleted    CheckoutSignal = "completed"
	SignalAsyncSuccess CheckoutSignal = "async_succeeded"
	SignalAsyncFailed  CheckoutSignal = "async_failed"
	SignalExpired      CheckoutSignal = "expired"
)

// SignalForWebhookEvent maps Stripe checkout lifecycle events to the
// corresponding order transition signal. The mapping is deliberately kept in
// the order application boundary so webhook adapters do not need to know the
// lifecycle implementation details.
func SignalForWebhookEvent(eventType string) (CheckoutSignal, bool) {
	switch eventType {
	case "checkout.session.completed":
		return SignalCompleted, true
	case "checkout.session.async_payment_succeeded":
		return SignalAsyncSuccess, true
	case "checkout.session.async_payment_failed":
		return SignalAsyncFailed, true
	case "checkout.session.expired":
		return SignalExpired, true
	default:
		return "", false
	}
}

type PaymentOrderLifecycle struct {
	repo  ConvergenceStore
	hints paymentports.PaymentHintPublisher
	now   func() time.Time
}

func NewLifecycle(repo ConvergenceStore, hints paymentports.PaymentHintPublisher, now func() time.Time) *PaymentOrderLifecycle {
	if now == nil {
		now = time.Now
	}
	return &PaymentOrderLifecycle{repo: repo, hints: hints, now: now}
}

func (l *PaymentOrderLifecycle) publishOrderEvent(ctx context.Context, order paymentsdomain.PaymentOrder, status paymentports.PaymentEventStatus) {
	if l == nil || l.hints == nil {
		return
	}
	l.hints.Publish(ctx, order.UserID, paymentports.PaymentEvent{PublicToken: order.PublicToken, Status: status, BillingAccountID: order.BillingAccountID})
}

func (l *PaymentOrderLifecycle) ExpireOrder(ctx context.Context, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	expired, changed, err := l.repo.MarkPaymentOrderExpired(ctx, order.ID)
	if err == nil && changed {
		l.publishOrderEvent(ctx, expired, paymentports.PaymentEventFailed)
	}
	return expired, err
}

func (l *PaymentOrderLifecycle) MarkFailed(ctx context.Context, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	return l.repo.MarkPaymentOrderFailed(ctx, order.ID)
}

func (l *PaymentOrderLifecycle) ConvergeCheckoutExpiration(ctx context.Context, order paymentsdomain.PaymentOrder, expiration paymentports.ExpireCheckoutSessionResult) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	if err := ValidateCheckoutSessionIdentity(order, expiration.Session); err != nil {
		return order, err
	}
	now := l.now().UTC()
	switch expiration.Outcome {
	case paymentports.CheckoutSessionExpirationOutcomeExpired:
		cancelled, changed, err := l.repo.MarkPaymentOrderCancelled(ctx, order.BillingAccountID, order.ID)
		if err == nil && changed {
			l.publishOrderEvent(ctx, cancelled, paymentports.PaymentEventFailed)
		}
		return cancelled, err
	case paymentports.CheckoutSessionExpirationOutcomePaid:
		result, err := l.repo.ConvergePaidPaymentOrder(ctx, PaidCheckoutFactsFromSession(order.StripeAccountID, expiration.Session, now), now)
		if err == nil && !result.Replayed {
			l.publishOrderEvent(ctx, result.Order, paymentports.PaymentEventCredited)
		}
		return result.Order, err
	case paymentports.CheckoutSessionExpirationOutcomeAsyncPaymentPending:
		previous := order.Status
		pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
		if err == nil && pending.Status != previous {
			l.publishOrderEvent(ctx, pending, paymentports.PaymentEventProcessing)
		}
		return pending, err
	case paymentports.CheckoutSessionExpirationOutcomeOpen:
		return order, paymentcontracts.ErrStripeProviderUnavailable
	default:
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
}

func (l *PaymentOrderLifecycle) ConvergePaidCheckoutSession(ctx context.Context, order paymentsdomain.PaymentOrder, session paymentports.CheckoutSession) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	now := l.now().UTC()
	result, err := l.repo.ConvergePaidPaymentOrder(ctx, PaidCheckoutFactsFromSession(order.StripeAccountID, session, now), now)
	if err != nil {
		return order, err
	}
	if !result.Replayed {
		l.publishOrderEvent(ctx, result.Order, paymentports.PaymentEventCredited)
	}
	return result.Order, nil
}

func (l *PaymentOrderLifecycle) CreditPaidOrder(ctx context.Context, order paymentsdomain.PaymentOrder, creditedAt time.Time) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	result, err := l.repo.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
	if err == nil && !result.Replayed {
		l.publishOrderEvent(ctx, result.Order, paymentports.PaymentEventCredited)
	}
	return result.Order, err
}

func (l *PaymentOrderLifecycle) ConvergeCheckoutSession(ctx context.Context, order paymentsdomain.PaymentOrder, session paymentports.CheckoutSession, signal CheckoutSignal) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	if err := ValidateCheckoutSessionIdentity(order, session); err != nil {
		return order, err
	}
	if session.IsPaid() {
		return l.ConvergePaidCheckoutSession(ctx, order, session)
	}
	switch session.Status {
	case paymentports.CheckoutSessionStatusExpired:
		return l.ExpireOrder(ctx, order)
	case paymentports.CheckoutSessionStatusComplete:
		return l.convergeCompletedCheckoutSession(ctx, order, signal)
	case paymentports.CheckoutSessionStatusOpen:
		return convergeOpenCheckoutSession(order, signal)
	default:
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
}

func (l *PaymentOrderLifecycle) convergeCompletedCheckoutSession(ctx context.Context, order paymentsdomain.PaymentOrder, signal CheckoutSignal) (paymentsdomain.PaymentOrder, error) {
	if signal == SignalAsyncSuccess {
		return order, paymentcontracts.ErrStripeProviderUnavailable
	}
	if signal == SignalAsyncFailed {
		failed, err := l.repo.MarkPaymentOrderFailed(ctx, order.ID)
		if err != nil {
			return order, err
		}
		if failed.Status == paymentsdomain.PaymentOrderFailed && failed.Status != order.Status {
			l.publishOrderEvent(ctx, failed, paymentports.PaymentEventFailed)
		}
		return failed, nil
	}
	pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
	if err != nil {
		return order, err
	}
	if order.Status != pending.Status {
		l.publishOrderEvent(ctx, pending, paymentports.PaymentEventProcessing)
	}
	return pending, nil
}

func convergeOpenCheckoutSession(order paymentsdomain.PaymentOrder, signal CheckoutSignal) (paymentsdomain.PaymentOrder, error) {
	switch signal {
	case SignalExpired, SignalCompleted, SignalAsyncFailed, SignalAsyncSuccess:
		return order, paymentcontracts.ErrStripeProviderUnavailable
	default:
		return order, nil
	}
}

func ValidateCheckoutSessionIdentity(order paymentsdomain.PaymentOrder, session paymentports.CheckoutSession) error {
	if strings.TrimSpace(session.ID) == "" || session.ID != order.StripeCheckoutSessionID && order.StripeCheckoutSessionID != "" ||
		session.PublicOrderNumber != order.PublicOrderNumber || session.PaymentOrderID != order.ID ||
		session.RequestHash != order.RequestHash || session.IdeaQuantity != order.IdeaQuantity {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	return nil
}

func PaidCheckoutFactsFromSession(accountID string, session paymentports.CheckoutSession, paidAt time.Time) paymentsdomain.PaidCheckoutFacts {
	return paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: accountID, StripeSessionID: session.ID, PublicOrderNumber: session.PublicOrderNumber,
		ExpiresAt: session.ExpiresAt, PaymentOrderID: session.PaymentOrderID, RequestHash: session.RequestHash,
		StripeCustomerID: session.CustomerID, StripePaymentIntentID: session.PaymentIntentID, StripeChargeID: session.ChargeID,
		IdeaQuantity: session.IdeaQuantity, PaidSubtotalMinor: session.AmountSubtotalMinor, PaidTaxMinor: session.AmountTaxMinor,
		PaidTotalMinor: session.AmountTotalMinor, PaidCurrency: session.Currency, ReceiptURL: session.ReceiptURL, PaidAt: paidAt,
	}
}
