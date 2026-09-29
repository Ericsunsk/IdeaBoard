// Order recovery owns the payment-order reconciliation state machine: one
// claimed order converges from creating / paid-pending-credit / bound
// checkout states toward a durable outcome. Stripe I/O always happens outside
// the claim transaction; this file only evaluates the claim and records the
// outcome the batch runner persists.
package reconciliation

import (
	"context"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentsdomain "ideaboard/api/internal/payments/domain"
)

// reconciliationCredentialError fences stale workers: a generation mismatch
// between the claimed order and the resolved credential never converges.
func reconciliationCredentialError(order paymentsdomain.PaymentOrder, credential paymentconfiguration.StoredStripeConfiguration, credentialErr error) error {
	if credentialErr != nil {
		return credentialErr
	}
	if credential.AccountID != order.StripeAccountID {
		return paymentsdomain.ErrPurchasesUnavailable
	}
	return nil
}

// terminal closes a converged order: clear any anomaly and schedule no
// further attempt before the next interval scan.
func terminal(now time.Time) Outcome {
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(Interval), ClearAnomaly: true}
}

// checkpoint keeps an unconverged order under observation without flagging
// it anomalous yet.
func checkpoint(now time.Time, order paymentsdomain.PaymentOrder) Outcome {
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(Interval), Anomalous: !order.CreatedAt.After(now.Add(-AnomalyAge))}
}

// failureOutcome records a redacted failure with an operable retry delay.
// Transient faults retry immediately; deterministic faults back off to avoid
// hot loops.
func failureOutcome(now time.Time, order paymentsdomain.PaymentOrder, err error, transient bool) Outcome {
	code, summary := FailureDetail(err)
	delay := OperableDelay
	if transient {
		delay = 0
	}
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(delay), ErrorCode: code, ErrorSummary: summary, Anomalous: !order.CreatedAt.After(now.Add(-AnomalyAge))}
}

// reconcileCreating recovers an order that never bound a Checkout Session:
// expire it when purchases are disabled, otherwise replay session creation
// under the original idempotency key.
func (s *Service) reconcileCreating(ctx context.Context, credential paymentconfiguration.StoredStripeConfiguration, apiKey string, enabled bool, order paymentsdomain.PaymentOrder, now time.Time) (Outcome, error) {
	if !enabled {
		if _, err := s.orderLifecycle.ExpireOrder(ctx, order); err != nil {
			return failureOutcome(now, order, err, true), err
		}
		return terminal(now), nil
	}
	if _, err := s.checkoutRecovery.CreateSessionForOrder(ctx, credential, apiKey, order); err != nil {
		return failureOutcome(now, order, err, FailureIsTransient(err)), err
	}
	return checkpoint(now, order), nil
}

// reconcileBound re-reads the canonical Stripe session for a bound order and
// converges it through the order lifecycle, expiring locally when the
// session already lapsed.
func (s *Service) reconcileBound(ctx context.Context, apiKey string, order paymentsdomain.PaymentOrder, now time.Time) (Outcome, error) {
	if order.StripeCheckoutSessionID == "" {
		err := paymentsdomain.ErrPurchasesUnavailable
		return failureOutcome(now, order, err, false), err
	}
	resolved, err := s.checkoutRecovery.RetrieveAndConverge(ctx, apiKey, order)
	if err != nil {
		return failureOutcome(now, order, err, FailureIsTransient(err)), err
	}
	if resolved.Status == paymentsdomain.PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
		resolved, err = s.orderLifecycle.ExpireOrder(ctx, resolved)
		if err != nil {
			return failureOutcome(now, order, err, true), err
		}
	}
	switch resolved.Status {
	case paymentsdomain.PaymentOrderCredited, paymentsdomain.PaymentOrderCancelled, paymentsdomain.PaymentOrderExpired, paymentsdomain.PaymentOrderFailed:
		return terminal(now), nil
	}
	return checkpoint(now, resolved), nil
}
