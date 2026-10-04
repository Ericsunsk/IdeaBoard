// Dispute recovery owns the payment-dispute reconciliation state machine:
// due disputes re-read the canonical Stripe fact, converge it durably, and
// expire active checkout sessions once an open/lost restriction commits.
// The pass is idempotent and safe to retry from webhook or River recovery.
package reconciliation

import (
	"context"
	"errors"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"
)

func (s *Service) reconcileDuePaymentDisputes(ctx context.Context, now time.Time) error {
	disputes, err := s.convergence.ListDuePaymentDisputes(ctx, now, 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, dispute := range disputes {
		_, apiKey, accessErr := s.orderAccess(ctx, dispute.PaymentOrderID)
		if accessErr != nil {
			retryErr = errors.Join(retryErr, accessErr)
			continue
		}
		provider, err := s.disputeGateway.RetrieveDispute(ctx, apiKey, dispute.StripeDisputeID)
		if err != nil {
			retryErr = errors.Join(retryErr, err)
			continue
		}
		updated, changed, err := s.convergence.ConvergePaymentDispute(ctx, provider, now)
		if err != nil {
			retryErr = errors.Join(retryErr, err)
			continue
		}
		if s.disputeExpiry != nil {
			if err := s.disputeExpiry.ExpireDisputeCheckoutSessions(ctx, updated); err != nil {
				retryErr = errors.Join(retryErr, err)
				continue
			}
		}
		// The dispute fact and dependent checkout expiry are durable before
		// this hint. Re-emitting on a later due pass repairs a dropped
		// in-memory notification without changing the ledger fact.
		if (changed || updated.UserID > 0) && s.publish != nil {
			s.publish(updated.UserID, paymentports.PaymentEventDisputeUpdated)
		}
	}
	return retryErr
}
