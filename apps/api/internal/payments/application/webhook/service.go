package webhook

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/database"
	"strings"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
)

type ConfigurationStore interface {
	ListStripeCredentials(context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error)
}

type InboxStore interface {
	InsertStripeWebhookEvent(context.Context, paymentports.StripeWebhookEvent, string, *time.Time) (bool, error)
	MarkStripeWebhookEventProcessed(context.Context, string, string, string, time.Time) error
}

type CheckoutStore interface {
	GetPaymentOrderByCheckoutSession(context.Context, string, string) (paymentsdomain.PaymentOrder, error)
}

type ConvergenceStore interface {
	GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error)
	CreateExternalPaymentRefund(context.Context, paymentports.StripeRefund, time.Time) (paymentsdomain.PaymentRefund, error)
	RecordPaymentRefundAnomaly(context.Context, paymentports.StripeRefund, string, time.Time) (int64, error)
	ConvergePaymentDispute(context.Context, paymentports.StripeDispute, time.Time) (paymentsdomain.PaymentDispute, bool, error)
}

type CredentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)

type Dependencies struct {
	Configuration    ConfigurationStore
	Inbox            InboxStore
	Convergence      ConvergenceStore
	Verifier         paymentports.StripeWebhookVerifier
	CheckoutStore    CheckoutStore
	Checkout         CheckoutLifecycle
	Refund           *paymentrefund.Service
	Dispute          *paymentdispute.Service
	CheckoutGateway  paymentports.CheckoutGateway
	DisputeGateway   paymentports.DisputeGateway
	CredentialAccess CredentialAccess
	Events           paymentports.PaymentEventPublisher
	Now              func() time.Time
}

type Service struct {
	configuration    ConfigurationStore
	inbox            InboxStore
	convergence      ConvergenceStore
	verifier         paymentports.StripeWebhookVerifier
	checkoutStore    CheckoutStore
	checkout         CheckoutLifecycle
	refund           *paymentrefund.Service
	dispute          *paymentdispute.Service
	checkoutGateway  paymentports.CheckoutGateway
	disputeGateway   paymentports.DisputeGateway
	credentialAccess CredentialAccess
	events           paymentports.PaymentEventPublisher
	now              func() time.Time
}

type CheckoutLifecycle interface {
	ConvergeCheckoutSession(context.Context, paymentsdomain.PaymentOrder, paymentports.CheckoutSession, paymentorder.CheckoutSignal) (paymentsdomain.PaymentOrder, error)
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.Configuration == nil || deps.Inbox == nil || deps.Convergence == nil || deps.Verifier == nil || deps.CheckoutStore == nil || deps.Checkout == nil || deps.Refund == nil || deps.Dispute == nil || deps.CheckoutGateway == nil || deps.DisputeGateway == nil || deps.CredentialAccess == nil {
		return nil, fmt.Errorf("webhook service requires configuration, inbox, convergence, verifier, checkout, refund, dispute, gateways, and credential access")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{configuration: deps.Configuration, inbox: deps.Inbox, convergence: deps.Convergence, verifier: deps.Verifier, checkoutStore: deps.CheckoutStore, checkout: deps.Checkout, refund: deps.Refund, dispute: deps.Dispute, checkoutGateway: deps.CheckoutGateway, disputeGateway: deps.DisputeGateway, credentialAccess: deps.CredentialAccess, events: deps.Events, now: deps.Now}, nil
}

func (s *Service) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (paymentports.StripeWebhookReceipt, error) {
	if s == nil || s.configuration == nil || s.inbox == nil || s.verifier == nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	if len(raw) == 0 || strings.TrimSpace(signature) == "" {
		return paymentports.StripeWebhookReceipt{}, paymentports.ErrMalformedWebhookPayload
	}
	credentials, err := s.configuration.ListStripeCredentials(ctx)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("load Stripe webhook credentials: %w", err)
	}
	if len(credentials) == 0 {
		return paymentports.StripeWebhookReceipt{}, paymentcontracts.ErrStripeConfigurationNotFound
	}
	now := s.now()
	stored, event, err := s.verifyWebhook(ctx, raw, signature, credentials, now)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	if err := paymentports.ValidateStripeWebhookEvent(event); err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	if event.Livemode != (stored.Mode == paymentports.StripeModeLive) {
		return paymentports.StripeWebhookReceipt{}, paymentports.ErrMalformedWebhookPayload
	}
	recognized := paymentports.IsRecognizedStripeWebhookEvent(event.Type)
	result := "received"
	var processedAt *time.Time
	if !recognized {
		result, processedAt = "ignored", &now
	}
	duplicate, err := s.inbox.InsertStripeWebhookEvent(ctx, event, result, processedAt)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	receipt := paymentports.StripeWebhookReceipt{EventID: event.ID, Duplicate: duplicate, Ignored: !recognized}
	if !recognized {
		return receipt, nil
	}
	if err := s.processEvent(ctx, event, stored, now); err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	return receipt, nil
}

func (s *Service) verifyWebhook(ctx context.Context, raw []byte, signature string, credentials []paymentconfiguration.StoredStripeConfiguration, now time.Time) (paymentconfiguration.StoredStripeConfiguration, paymentports.StripeWebhookEvent, error) {
	var lastErr error
	for _, credential := range credentials {
		secret, decryptErr := secrets.DecryptMaterialString(credential.WebhookSecretCiphertext, credential.SecretMaterialKind)
		if decryptErr != nil {
			return paymentconfiguration.StoredStripeConfiguration{}, paymentports.StripeWebhookEvent{}, paymentcontracts.ErrStripeConfigurationInvalid
		}
		event, verifyErr := s.verifier.Verify(ctx, raw, signature, secret, paymentports.StripeWebhookTolerance, now)
		if verifyErr == nil {
			return credential, event, nil
		}
		lastErr = verifyErr
	}
	return paymentconfiguration.StoredStripeConfiguration{}, paymentports.StripeWebhookEvent{}, lastErr
}

func (s *Service) processEvent(ctx context.Context, event paymentports.StripeWebhookEvent, stored paymentconfiguration.StoredStripeConfiguration, now time.Time) error {
	signal, checkoutEvent := paymentorder.SignalForWebhookEvent(event.Type)
	if !checkoutEvent {
		if strings.HasPrefix(event.Type, "refund.") {
			if err := s.processRefund(ctx, event, stored.CredentialID); err != nil {
				_ = s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", paymentreconciliation.OperationalErrorSummary(err, "Stripe refund webhook processing failed"), now)
				return err
			}
		}
		if strings.HasPrefix(event.Type, "charge.dispute.") {
			if err := s.processDispute(ctx, event, stored.CredentialID); err != nil {
				_ = s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", paymentreconciliation.OperationalErrorSummary(err, "Stripe dispute webhook processing failed"), now)
				return err
			}
		}
		return s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now)
	}
	return s.processCheckout(ctx, event, stored, signal, now)
}

func (s *Service) processCheckout(ctx context.Context, event paymentports.StripeWebhookEvent, stored paymentconfiguration.StoredStripeConfiguration, signal paymentorder.CheckoutSignal, now time.Time) error {
	apiKey, err := apiKeyFromCredential(stored)
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
	return s.inbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now)
}

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

func apiKeyFromCredential(stored paymentconfiguration.StoredStripeConfiguration) (string, error) {
	return secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.SecretMaterialKind)
}

func operationError(err error) error {
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) && gatewayErr.Code() == paymentports.GatewayErrorProviderUnavailable {
		return errors.Join(paymentcontracts.ErrStripeProviderUnavailable, err)
	}
	return errors.Join(paymentsdomain.ErrPurchasesUnavailable, err)
}
