// Webhook dispatch routes durable, recognized Stripe events to the owning
// recovery service: checkout sessions converge through the order lifecycle,
// refund events converge through the refund service, and dispute events
// converge through the dispute service plus the convergence store.
//
// Dispatch never verifies signatures and never writes the inbox receipt:
// intake.go owns those. Each branch records inbox failure metadata through
// the shared operational summary so retries stay observable.
package webhook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits/domain"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/database"
)

// processEvent fans one recognized event out to its owning recovery path.
// Refund and dispute branches share the refund./charge.dispute. prefixes;
// checkout lifecycle events converge through the order state machine.
func (s *Service) processEvent(ctx context.Context, event paymentports.StripeWebhookEvent, stored paymentconfiguration.StoredStripeConfiguration, now time.Time) error {
	signal, checkoutEvent := paymentorder.SignalForWebhookEvent(event.Type)
	if !checkoutEvent {
		if strings.HasPrefix(event.Type, "refund.") {
			if err := s.processRefund(ctx, event, stored.CredentialID); err != nil {
				_ = s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, string(WebhookProcessingFailed), paymentreconciliation.OperationalErrorSummary(err, "Stripe refund webhook processing failed"), now)
				return err
			}
		}
		if strings.HasPrefix(event.Type, "charge.dispute.") {
			if err := s.processDispute(ctx, event, stored.CredentialID); err != nil {
				_ = s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, string(WebhookProcessingFailed), paymentreconciliation.OperationalErrorSummary(err, "Stripe dispute webhook processing failed"), now)
				return err
			}
		}
		return s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, string(WebhookProcessingProcessed), "", now)
	}
	return s.processCheckout(ctx, event, stored, signal, now)
}

// processCheckout re-reads the canonical Checkout Session from Stripe and
// converges it through the order lifecycle. Browser-supplied facts never
// credit; only this server-side read may promote the order.
func (s *Service) processCheckout(ctx context.Context, event paymentports.StripeWebhookEvent, stored paymentconfiguration.StoredStripeConfiguration, signal paymentorder.CheckoutSignal, now time.Time) error {
	_, apiKey, err := s.credentialAccess(ctx, stored.CredentialID)
	if err != nil {
		return paymentcontracts.ErrStripeConfigurationInvalid
	}
	session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, paymentports.RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: event.ObjectID})
	if err != nil {
		return operationError(err)
	}
	if session.ID != event.ObjectID {
		return paymentports.ErrMalformedWebhookPayload
	}
	order, err := s.checkoutStore.GetPaymentOrderByCheckoutSession(ctx, stored.AccountID, session.ID)
	if err != nil {
		return fmt.Errorf("resolve payment order for Stripe Session: %w", err)
	}
	if _, err := s.checkout.ConvergeCheckoutSession(ctx, order, session, signal); err != nil {
		return err
	}
	return s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, string(WebhookProcessingProcessed), "", now)
}

// processDispute retrieves the canonical dispute and converges it. Orders
// that cannot carry a dispute are acknowledged without state change so
// unrelated disputes never block the inbox.
func (s *Service) processDispute(ctx context.Context, event paymentports.StripeWebhookEvent, credentialID string) error {
	if strings.TrimSpace(event.ObjectID) == "" {
		return paymentports.ErrMalformedWebhookPayload
	}
	_, apiKey, err := s.credentialAccess(ctx, credentialID)
	if err != nil {
		return err
	}
	provider, err := s.disputeGateway.RetrieveDispute(ctx, apiKey, event.ObjectID)
	if err != nil {
		return err
	}
	if provider.ID != event.ObjectID || (event.ChargeID != "" && provider.ChargeID != event.ChargeID) || (event.PaymentIntentID != "" && provider.PaymentIntentID != "" && provider.PaymentIntentID != event.PaymentIntentID) {
		return paymentcontracts.ErrDisputeProviderConflict
	}
	dispute, changed, err := s.convergence.ConvergePaymentDispute(ctx, provider, s.now().UTC())
	if errors.Is(err, paymentcontracts.ErrDisputeOrderNotEligible) {
		return nil
	}
	if err != nil {
		return err
	}
	if changed && s.events != nil {
		s.events.Publish(dispute.UserID, paymentports.PaymentEvent{Status: paymentports.PaymentEventDisputeUpdated})
	}
	return s.dispute.ExpireDisputeCheckoutSessions(ctx, dispute)
}

// processRefund converges a refund event for a known local refund, or falls
// back to the external (Stripe Dashboard initiated) path when the Stripe ID
// is unknown locally.
func (s *Service) processRefund(ctx context.Context, event paymentports.StripeWebhookEvent, credentialID string) error {
	if strings.TrimSpace(event.ObjectID) == "" {
		return paymentports.ErrMalformedWebhookPayload
	}
	refund, err := s.convergence.GetPaymentRefundByStripeID(ctx, event.ObjectID)
	if database.IsNotFound(err) {
		return s.processExternalRefund(ctx, event.ObjectID, credentialID)
	}
	if err != nil {
		return err
	}
	apiKey, err := s.refund.RefundAPIKeyForOrder(ctx, refund.PaymentOrderID)
	if err != nil {
		return err
	}
	providerRefund, err := s.refund.RetrieveProviderRefund(ctx, apiKey, event.ObjectID)
	if err != nil {
		return err
	}
	_, err = s.refund.Converge(ctx, refund, providerRefund)
	return err
}

// processExternalRefund converges a Dashboard-initiated refund with no local
// reservation. Insufficient unconsumed Ideas become a durable anomaly for
// manual recovery instead of a silent drop.
func (s *Service) processExternalRefund(ctx context.Context, refundID, credentialID string) error {
	_, apiKey, err := s.credentialAccess(ctx, credentialID)
	if err != nil {
		return err
	}
	providerRefund, err := s.refund.RetrieveProviderRefund(ctx, apiKey, refundID)
	if err != nil {
		return err
	}
	if providerRefund.ChargeID == "" {
		return nil
	}
	if _, createErr := s.convergence.CreateExternalPaymentRefund(ctx, providerRefund, s.now().UTC()); createErr != nil {
		if errors.Is(createErr, domain.ErrInsufficientCredits) || errors.Is(createErr, paymentcontracts.ErrRefundAmountExceeds) {
			userID, anomalyErr := s.convergence.RecordPaymentRefundAnomaly(ctx, providerRefund, "Stripe Dashboard refund requires manual balance recovery", s.now().UTC())
			if anomalyErr == nil && s.events != nil {
				s.events.Publish(userID, paymentports.PaymentEvent{Status: paymentports.PaymentEventRefundUpdated})
			}
			return anomalyErr
		}
		if database.IsNotFound(createErr) || errors.Is(createErr, paymentcontracts.ErrRefundOrderNotEligible) {
			return nil
		}
		return createErr
	}
	refund, err := s.convergence.GetPaymentRefundByStripeID(ctx, refundID)
	if err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(refund.UserID, paymentports.PaymentEvent{Status: paymentports.PaymentEventRefundUpdated})
	}
	providerRefund.Status = strings.ToLower(strings.TrimSpace(providerRefund.Status))
	_, err = s.refund.Converge(ctx, refund, providerRefund)
	return err
}

// operationError maps provider transport failures to the stable checkout
// boundary taxonomy.
func operationError(err error) error {
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) && gatewayErr.Code() == paymentports.GatewayErrorProviderUnavailable {
		return errors.Join(paymentcontracts.ErrStripeProviderUnavailable, err)
	}
	return errors.Join(paymentsdomain.ErrPurchasesUnavailable, err)
}
