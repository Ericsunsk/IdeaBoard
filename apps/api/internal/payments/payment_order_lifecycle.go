package payments

import (
	"context"
	"time"
)

type paymentOrderLifecycle struct {
	repo   PaymentOrderConvergenceStore
	events *EventHub
	now    func() time.Time
}

func newPaymentOrderLifecycle(repo PaymentOrderConvergenceStore, events *EventHub, now func() time.Time) *paymentOrderLifecycle {
	if now == nil {
		now = time.Now
	}
	return &paymentOrderLifecycle{repo: repo, events: events, now: now}
}

func (l *paymentOrderLifecycle) convergeCheckoutSession(
	ctx context.Context,
	order PaymentOrder,
	session CheckoutSession,
	signal checkoutSignal,
) (PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, ErrPurchasesUnavailable
	}
	if err := validateCheckoutSessionIdentity(order, session); err != nil {
		return order, err
	}

	if session.IsPaid() {
		return l.convergePaidCheckoutSession(ctx, order, session)
	}

	switch session.Status {
	case CheckoutSessionStatusExpired:
		return l.expireOrder(ctx, order)
	case CheckoutSessionStatusComplete:
		return l.convergeCompletedCheckoutSession(ctx, order, signal)
	case CheckoutSessionStatusOpen:
		return convergeOpenCheckoutSession(order, signal)
	default:
		return order, ErrPurchasesUnavailable
	}
}

func (l *paymentOrderLifecycle) convergePaidCheckoutSession(ctx context.Context, order PaymentOrder, session CheckoutSession) (PaymentOrder, error) {
	now := l.now().UTC()
	result, err := l.repo.ConvergePaidPaymentOrder(
		ctx,
		paidCheckoutFactsFromSession(order.StripeAccountID, session, now),
		now,
	)
	if err != nil {
		return order, err
	}
	if !result.Replayed {
		l.publish(result.Order.UserID, PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
	}
	return result.Order, nil
}

func (l *paymentOrderLifecycle) convergeCompletedCheckoutSession(ctx context.Context, order PaymentOrder, signal checkoutSignal) (PaymentOrder, error) {
	if signal == checkoutSignalAsyncSuccess {
		return order, ErrStripeProviderUnavailable
	}
	if signal == checkoutSignalAsyncFailed {
		failed, err := l.repo.MarkPaymentOrderFailed(ctx, order.ID)
		if err != nil {
			return order, err
		}
		if failed.Status == PaymentOrderFailed && failed.Status != order.Status {
			l.publish(failed.UserID, PaymentEvent{PublicToken: failed.PublicToken, Status: "failed"})
		}
		return failed, nil
	}
	pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
	if err != nil {
		return order, err
	}
	if order.Status != pending.Status {
		l.publish(pending.UserID, PaymentEvent{PublicToken: pending.PublicToken, Status: "processing"})
	}
	return pending, nil
}

func convergeOpenCheckoutSession(order PaymentOrder, signal checkoutSignal) (PaymentOrder, error) {
	switch signal {
	case checkoutSignalExpired, checkoutSignalCompleted, checkoutSignalAsyncFailed, checkoutSignalAsyncSuccess:
		return order, ErrStripeProviderUnavailable
	default:
		return order, nil
	}
}

func (l *paymentOrderLifecycle) creditPaidOrder(ctx context.Context, order PaymentOrder, creditedAt time.Time) (PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, ErrPurchasesUnavailable
	}
	result, err := l.repo.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
	if err == nil && !result.Replayed {
		l.publish(result.Order.UserID, PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
	}
	return result.Order, err
}

func (l *paymentOrderLifecycle) expireOrder(ctx context.Context, order PaymentOrder) (PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, ErrPurchasesUnavailable
	}
	expired, changed, err := l.repo.MarkPaymentOrderExpired(ctx, order.ID)
	if err == nil && changed {
		l.publish(expired.UserID, PaymentEvent{PublicToken: expired.PublicToken, Status: "failed"})
	}
	return expired, err
}

func (l *paymentOrderLifecycle) markFailed(ctx context.Context, order PaymentOrder) (PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, ErrPurchasesUnavailable
	}
	return l.repo.MarkPaymentOrderFailed(ctx, order.ID)
}

func (l *paymentOrderLifecycle) convergeCheckoutExpiration(ctx context.Context, order PaymentOrder, expiration ExpireCheckoutSessionResult) (PaymentOrder, error) {
	if l == nil || l.repo == nil {
		return order, ErrPurchasesUnavailable
	}
	if err := validateCheckoutSessionIdentity(order, expiration.Session); err != nil {
		return order, err
	}
	now := l.now().UTC()
	switch expiration.Outcome {
	case CheckoutSessionExpirationOutcomeExpired:
		cancelled, changed, err := l.repo.MarkPaymentOrderCancelled(ctx, order.UserID, order.ID)
		if err == nil && changed {
			l.publish(cancelled.UserID, PaymentEvent{PublicToken: cancelled.PublicToken, Status: "failed"})
		}
		return cancelled, err
	case CheckoutSessionExpirationOutcomePaid:
		result, err := l.repo.ConvergePaidPaymentOrder(ctx,
			paidCheckoutFactsFromSession(order.StripeAccountID, expiration.Session, now), now)
		if err == nil && !result.Replayed {
			l.publish(result.Order.UserID, PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
		}
		return result.Order, err
	case CheckoutSessionExpirationOutcomeAsyncPaymentPending:
		previous := order.Status
		pending, err := l.repo.MarkPaymentOrderAsyncPending(ctx, order.ID)
		if err == nil && pending.Status != previous {
			l.publish(pending.UserID, PaymentEvent{PublicToken: pending.PublicToken, Status: "processing"})
		}
		return pending, err
	case CheckoutSessionExpirationOutcomeOpen:
		return order, ErrStripeProviderUnavailable
	default:
		return order, ErrPurchasesUnavailable
	}
}

func (l *paymentOrderLifecycle) publish(userID int64, event PaymentEvent) {
	if l != nil && l.events != nil {
		l.events.Publish(userID, event)
	}
}
