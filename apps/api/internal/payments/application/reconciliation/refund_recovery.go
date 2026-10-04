// Refund recovery owns the payment-refund reconciliation state machine:
// pending local refunds converge toward provider-confirmed succeeded/failed
// facts, creating the Stripe refund once when the local row has no provider
// ID yet. Deterministic provider rejections mark the refund failed with a
// redacted summary; transient faults requeue through the batch runner.
package reconciliation

import (
	"context"
	"errors"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

func (s *Service) reconcileDuePaymentRefunds(ctx context.Context, now time.Time) error {
	refunds, err := s.convergence.ListPendingPaymentRefunds(ctx, now.Add(-2*time.Minute), 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, refund := range refunds {
		_, apiKey, accessErr := s.orderAccess(ctx, refund.PaymentOrderID)
		if accessErr != nil {
			retryErr = errors.Join(retryErr, accessErr)
			continue
		}
		var provider paymentports.StripeRefund
		if refund.StripeRefundID == "" {
			provider, err = s.refundGateway.CreateRefund(ctx, paymentports.CreateStripeRefundInput{APIKey: apiKey, ChargeID: refund.StripeChargeID, AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID})
			if err != nil {
				if FailureIsTransient(err) {
					retryErr = errors.Join(retryErr, err)
					continue
				}
				if _, markErr := s.convergence.MarkPaymentRefundFailed(ctx, refund.ID, OperationalErrorSummary(err, "Stripe refund request failed"), now); markErr != nil {
					retryErr = errors.Join(retryErr, markErr)
				}
				continue
			}
		} else {
			provider, err = s.refundGateway.RetrieveRefund(ctx, apiKey, refund.StripeRefundID)
			if err != nil {
				retryErr = errors.Join(retryErr, err)
				continue
			}
		}
		if err := s.convergeRefund(ctx, refund, provider, now); err != nil {
			retryErr = errors.Join(retryErr, err)
			continue
		}
		// The durable refund fact is committed before this post-commit hint.
		// Re-emitting during a later recovery pass lets connected clients
		// refresh after a process crash dropped the original in-memory event.
		if s.publish != nil {
			s.publish(refund.UserID, paymentports.PaymentEventRefundUpdated)
		}
	}
	return retryErr
}

// convergeRefund records the provider result then settles the durable refund
// fact exactly once toward succeeded or failed.
func (s *Service) convergeRefund(ctx context.Context, refund paymentsdomain.PaymentRefund, provider paymentports.StripeRefund, now time.Time) error {
	if err := paymentrefundValidate(refund, provider); err != nil {
		return err
	}
	updated, err := s.convergence.MarkPaymentRefundProviderResult(ctx, refund.ID, provider.ID, provider.Status, now)
	if err != nil {
		return err
	}
	switch paymentports.StripeRefundStatus(provider.Status) {
	case paymentports.StripeRefundSucceeded:
		_, err = s.convergence.MarkPaymentRefundSucceeded(ctx, updated.ID, now)
	case paymentports.StripeRefundFailed, paymentports.StripeRefundCanceled:
		_, err = s.convergence.MarkPaymentRefundFailed(ctx, updated.ID, "Stripe refund failed", now)
	}
	return err
}

// paymentrefundValidate enforces provider identity and amount invariants
// before any durable transition.
func paymentrefundValidate(refund paymentsdomain.PaymentRefund, provider paymentports.StripeRefund) error {
	if refund.StripeRefundID != "" && refund.StripeRefundID != provider.ID {
		return paymentcontracts.ErrRefundProviderConflict
	}
	if refund.StripeChargeID != provider.ChargeID || refund.AmountMinor != provider.AmountMinor || refund.Currency != provider.Currency {
		return paymentcontracts.ErrRefundProviderConflict
	}
	return nil
}
