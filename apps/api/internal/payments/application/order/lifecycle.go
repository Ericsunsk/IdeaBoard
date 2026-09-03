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
	repo   ConvergenceStore
	events paymentports.PaymentEventPublisher
	now    func() time.Time
}

func NewLifecycle(repo ConvergenceStore, events paymentports.PaymentEventPublisher, now func() time.Time) *PaymentOrderLifecycle {
	if now == nil {
		now = time.Now
	}
	return &PaymentOrderLifecycle{repo: repo, events: events, now: now}
}

func (l *PaymentOrderLifecycle) Publish(userID int64, event paymentports.PaymentEvent) {
	if l != nil && l.events != nil {
		l.events.Publish(userID, event)
	}
}

func (l *PaymentOrderLifecycle) ExpireOrder(ctx context.Context, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	expired, changed, err := l.repo.MarkPaymentOrderExpired(ctx, order.ID)
	if err == nil && changed {
		l.Publish(expired.UserID, paymentports.PaymentEvent{PublicToken: expired.PublicToken, Status: "failed"})
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
		cancelled, changed, err := l.repo.MarkPaymentOrderCancelled(ctx, order.UserID, order.ID)
		if err == nil && changed {
			l.Publish(cancelled.UserID, paymentports.PaymentEvent{PublicToken: cancelled.PublicToken, Status: "failed"})
		}
		return cancelled, err
	case paymentports.CheckoutSessionExpirationOutcomePaid:
		result, err := l.repo.ConvergePaidPaymentOrder(ctx, PaidCheckoutFactsFromSession(order.StripeAccountID, expiration.Session, now), now)
		if err == nil && !result.Replayed {
			l.Publish(result.Order.UserID, paymentports.PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
		}
		return result.Order, err
	case paymentports.CheckoutSessionExpirationOutcomeAsyncPaymentPending:
		previous := order.Status
		pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
		if err == nil && pending.Status != previous {
			l.Publish(pending.UserID, paymentports.PaymentEvent{PublicToken: pending.PublicToken, Status: "processing"})
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
		l.Publish(result.Order.UserID, paymentports.PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
	}
	return result.Order, nil
}

func (l *PaymentOrderLifecycle) CreditPaidOrder(ctx context.Context, order paymentsdomain.PaymentOrder, creditedAt time.Time) (paymentsdomain.PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, paymentsdomain.ErrPurchasesUnavailable
	}
	result, err := l.repo.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
	if err == nil && !result.Replayed {
		l.Publish(result.Order.UserID, paymentports.PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
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
			l.Publish(failed.UserID, paymentports.PaymentEvent{PublicToken: failed.PublicToken, Status: "failed"})
		}
		return failed, nil
	}
	pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
	if err != nil {
		return order, err
	}
	if order.Status != pending.Status {
		l.Publish(pending.UserID, paymentports.PaymentEvent{PublicToken: pending.PublicToken, Status: "processing"})
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
