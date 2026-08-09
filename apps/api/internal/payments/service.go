package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/secrets"
)

type Service struct {
	repo             paymentPersistence
	configuration    StripeConfigurationStore
	webhookInbox     StripeWebhookInboxStore
	gateway          StripeGateway
	refundGateway    RefundGateway
	disputeGateway   DisputeGateway
	checkoutGateway  CheckoutGateway
	orderLifecycle   *paymentOrderLifecycle
	verifier         StripeWebhookVerifier
	wallets          credits.WalletReader
	events           *EventHub
	now              func() time.Time
	shutdownEnqueuer PaymentCheckoutShutdownEnqueuer
}

type PaymentCheckoutShutdownEnqueuer interface {
	EnqueuePaymentCheckoutShutdown(context.Context, string) error
}

type ServiceDependencies struct {
	Persistence          paymentPersistence
	ConfigurationStore   StripeConfigurationStore
	WebhookInbox         StripeWebhookInboxStore
	ConfigurationGateway StripeGateway
	RefundGateway        RefundGateway
	DisputeGateway       DisputeGateway
	CheckoutGateway      CheckoutGateway
	WebhookVerifier      StripeWebhookVerifier
	Wallets              credits.WalletReader
	Events               *EventHub
	Now                  func() time.Time
}

func NewService(dependencies ServiceDependencies) *Service {
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	events := dependencies.Events
	if events == nil {
		events = NewEventHub()
	}
	service := &Service{
		repo: dependencies.Persistence, configuration: dependencies.ConfigurationStore,
		webhookInbox: dependencies.WebhookInbox, gateway: dependencies.ConfigurationGateway,
		refundGateway:   dependencies.RefundGateway,
		disputeGateway:  dependencies.DisputeGateway,
		checkoutGateway: dependencies.CheckoutGateway, verifier: dependencies.WebhookVerifier,
		wallets: dependencies.Wallets, events: events, now: now,
	}
	service.orderLifecycle = newPaymentOrderLifecycle(service.repo, service.events, service.now)
	return service
}

func (s *Service) EventHub() *EventHub {
	if s == nil {
		return nil
	}
	return s.events
}

func (s *Service) SetPaymentCheckoutShutdownEnqueuer(enqueuer PaymentCheckoutShutdownEnqueuer) {
	if s != nil {
		s.shutdownEnqueuer = enqueuer
	}
}

func (s *Service) GetStripeConfiguration(ctx context.Context) (StripeConfigurationSummary, error) {
	if s == nil || s.configuration == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments service unavailable")
	}
	stored, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StripeConfigurationSummary{}, nil
	}
	if err != nil {
		return StripeConfigurationSummary{}, fmt.Errorf("get Stripe configuration: %w", err)
	}
	return stripeConfigurationSummary(stored), nil
}

func (s *Service) ConfigureStripe(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.configuration == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	var result StripeConfigurationSummary
	err := s.configuration.WithStripeConfigurationExclusiveLock(ctx, func(lockedContext context.Context) error {
		var err error
		result, err = s.configureStripeLocked(lockedContext, command)
		return err
	})
	return result, err
}

func (s *Service) configureStripeLocked(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.configuration == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	if command.ActorID <= 0 {
		return StripeConfigurationSummary{}, fmt.Errorf("invalid payment admin actor")
	}
	current, hasCurrent, err := s.loadCurrentConfiguration(ctx)
	if err != nil {
		return StripeConfigurationSummary{}, err
	}
	if hasCurrent && current.PurchasesEnabled && !command.PurchasesEnabled && strings.TrimSpace(command.APIKey) == "" {
		metadata := safeAuditMetadata(map[string]any{
			"account_id": current.AccountID, "mode": current.Mode, "product_tax_code": current.ProductTaxCode,
			"purchases_enabled": false,
		})
		stored, shutdown, err := s.configuration.ReplaceStripeConfiguration(ctx, StoredStripeConfiguration{
			AccountID: current.AccountID, Mode: current.Mode, ChargesEnabled: current.ChargesEnabled,
			ProductTaxCode: current.ProductTaxCode, PurchasesEnabled: false,
			StripeProductID: current.StripeProductID, StripeWebhookEndpointID: current.StripeWebhookEndpointID,
			APIKeyCiphertext: current.APIKeyCiphertext, WebhookSecretCiphertext: current.WebhookSecretCiphertext,
			SecretMaterialKind: current.SecretMaterialKind, UpdatedBy: command.ActorID,
		}, ConfigurationAudit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
		if err != nil {
			return StripeConfigurationSummary{}, fmt.Errorf("disable Stripe purchases: %w", err)
		}
		s.enqueuePaymentCheckoutShutdown(ctx, shutdown)
		return stripeConfigurationSummary(stored), nil
	}
	taxCode, err := normalizeProductTaxCode(command.ProductTaxCode)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "invalid_tax_code", err.Error(), nil, err)
	}
	webhookURL, err := normalizeWebhookURL(command.WebhookURL)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "invalid_webhook_url", err.Error(), nil, err)
	}
	if s.gateway == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	apiKey := strings.TrimSpace(command.APIKey)
	if hasCurrent && apiKey != "" {
		if err := s.settleOrdersBeforeStripeKeyChange(ctx, current); err != nil {
			return StripeConfigurationSummary{}, err
		}
	}
	if apiKey == "" {
		if !hasCurrent {
			return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "api_key_required", ErrStripeAPIKeyRequired.Error(), nil, ErrStripeAPIKeyRequired)
		}
		apiKey, err = secrets.DecryptMaterialString(current.APIKeyCiphertext, current.SecretMaterialKind)
		if err != nil {
			return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe API Key 无法读取", nil, ErrStripeConfigurationInvalid)
		}
	}
	mode, err := ClassifyStripeAPIKey(apiKey)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "invalid_key", err.Error(), map[string]any{"mode": "unknown"}, err)
	}

	prepare := PrepareStripeConfiguration{
		APIKey: apiKey, Mode: mode, PurchasesEnabled: command.PurchasesEnabled,
		ProductTaxCode: taxCode, WebhookURL: webhookURL,
	}
	if hasCurrent {
		prepare.ExistingAccountID = current.AccountID
		prepare.ExistingProductID = current.StripeProductID
		prepare.ExistingWebhookEndpointID = current.StripeWebhookEndpointID
		prepare.ExistingWebhookSecret, err = secrets.DecryptMaterialString(current.WebhookSecretCiphertext, current.SecretMaterialKind)
		if err != nil {
			return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe Webhook Secret 无法读取", map[string]any{"mode": mode}, ErrStripeConfigurationInvalid)
		}
	}

	prepared, err := s.gateway.PrepareConfiguration(ctx, prepare)
	if err != nil {
		code, summary, publicErr := gatewayFailure(err)
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, string(code), summary, map[string]any{"mode": mode}, wrapPublicError(publicErr, summary))
	}
	if err := validatePreparedConfiguration(prepared, mode); err != nil {
		code := "invalid_response"
		if errors.Is(err, ErrStripeCapabilityUnavailable) {
			code = "capability_missing"
		}
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, code, err.Error(), map[string]any{"mode": mode}, err)
	}

	encryptedAPIKey, err := secrets.EncryptString(apiKey)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe API Key 加密失败", map[string]any{"mode": mode}, ErrStripeConfigurationInvalid)
	}
	encryptedWebhookSecret, err := secrets.EncryptString(prepared.WebhookSecret)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe Webhook Secret 加密失败", map[string]any{"mode": mode}, ErrStripeConfigurationInvalid)
	}

	metadata := safeAuditMetadata(map[string]any{
		"account_id": prepared.AccountID, "mode": mode, "product_tax_code": taxCode,
		"purchases_enabled": command.PurchasesEnabled,
	})
	stored, shutdown, err := s.configuration.ReplaceStripeConfiguration(ctx, StoredStripeConfiguration{
		AccountID: prepared.AccountID, Mode: prepared.Mode, ChargesEnabled: prepared.ChargesEnabled,
		ProductTaxCode: taxCode, PurchasesEnabled: command.PurchasesEnabled,
		StripeProductID: prepared.ProductID, StripeWebhookEndpointID: prepared.WebhookEndpointID,
		APIKeyCiphertext: encryptedAPIKey, WebhookSecretCiphertext: encryptedWebhookSecret,
		SecretMaterialKind: secrets.MaterialKindEncrypted, UpdatedBy: command.ActorID,
	}, ConfigurationAudit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return StripeConfigurationSummary{}, fmt.Errorf("publish Stripe configuration: %w", err)
	}
	s.enqueuePaymentCheckoutShutdown(ctx, shutdown)
	return stripeConfigurationSummary(stored), nil
}

func (s *Service) enqueuePaymentCheckoutShutdown(ctx context.Context, shutdown *PaymentCheckoutShutdown) {
	if shutdown == nil || s == nil || s.shutdownEnqueuer == nil {
		return
	}
	if err := s.shutdownEnqueuer.EnqueuePaymentCheckoutShutdown(ctx, shutdown.ID); err != nil {
		slog.Error("enqueue Checkout shutdown failed; periodic River recovery will retry", "shutdown_id", shutdown.ID, "error", err)
	}
}

func (s *Service) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.configuration == nil || s.webhookInbox == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	var receipt StripeWebhookReceipt
	err := s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		var err error
		receipt, err = s.receiveStripeWebhookLocked(lockedContext, raw, signature)
		return err
	})
	return receipt, err
}

func (s *Service) receiveStripeWebhookLocked(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.configuration == nil || s.webhookInbox == nil || s.verifier == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	if len(raw) == 0 || strings.TrimSpace(signature) == "" {
		return StripeWebhookReceipt{}, ErrMalformedWebhookPayload
	}
	stored, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StripeWebhookReceipt{}, ErrStripeConfigurationNotFound
	}
	if err != nil {
		return StripeWebhookReceipt{}, fmt.Errorf("load Stripe webhook configuration: %w", err)
	}
	secret, err := secrets.DecryptMaterialString(stored.WebhookSecretCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return StripeWebhookReceipt{}, ErrStripeConfigurationInvalid
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	event, err := s.verifier.Verify(ctx, raw, signature, secret, StripeWebhookTolerance, now)
	if err != nil {
		return StripeWebhookReceipt{}, err
	}
	if err := validateStripeWebhookEvent(event); err != nil {
		return StripeWebhookReceipt{}, err
	}
	if event.Livemode != (stored.Mode == StripeModeLive) {
		return StripeWebhookReceipt{}, ErrMalformedWebhookPayload
	}
	recognized := isRecognizedStripeWebhookEvent(event.Type)
	result := "received"
	var processedAt *time.Time
	if !recognized {
		result = "ignored"
		processedAt = &now
	}
	duplicate, err := s.webhookInbox.InsertStripeWebhookEvent(ctx, event, result, processedAt)
	if err != nil {
		return StripeWebhookReceipt{}, err
	}
	receipt := StripeWebhookReceipt{EventID: event.ID, Duplicate: duplicate, Ignored: !recognized}
	if !recognized {
		return receipt, nil
	}
	signal, checkoutLifecycleEvent := checkoutSignalForWebhookEvent(event.Type)
	if !checkoutLifecycleEvent {
		if strings.HasPrefix(event.Type, "refund.") {
			if err := s.processRefundWebhook(ctx, event); err != nil {
				_ = s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", operationalErrorSummary(err, "Stripe refund webhook processing failed"), now)
				return StripeWebhookReceipt{}, err
			}
		}
		if strings.HasPrefix(event.Type, "charge.dispute.") {
			if err := s.processDisputeWebhook(ctx, event); err != nil {
				_ = s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", operationalErrorSummary(err, "Stripe dispute webhook processing failed"), now)
				return StripeWebhookReceipt{}, err
			}
		}
		if err := s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now); err != nil {
			return StripeWebhookReceipt{}, err
		}
		return receipt, nil
	}
	if s.checkoutGateway == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments Checkout gateway unavailable")
	}
	apiKey, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return StripeWebhookReceipt{}, ErrStripeConfigurationInvalid
	}
	session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: event.ObjectID})
	if err != nil {
		return StripeWebhookReceipt{}, checkoutOperationError(err)
	}
	if session.ID != event.ObjectID {
		return StripeWebhookReceipt{}, ErrMalformedWebhookPayload
	}
	order, err := s.repo.GetPaymentOrderByCheckoutSession(ctx, stored.AccountID, session.ID)
	if err != nil {
		return StripeWebhookReceipt{}, fmt.Errorf("resolve payment order for Stripe Session: %w", err)
	}
	if _, err := s.orderLifecycle.convergeCheckoutSession(ctx, order, session, signal); err != nil {
		return StripeWebhookReceipt{}, err
	}
	if err := s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now); err != nil {
		return StripeWebhookReceipt{}, err
	}
	return receipt, nil
}

func (s *Service) processDisputeWebhook(ctx context.Context, event StripeWebhookEvent) error {
	if s == nil || s.repo == nil || s.disputeGateway == nil || strings.TrimSpace(event.ObjectID) == "" {
		return ErrMalformedWebhookPayload
	}
	_, apiKey, err := s.stripeAccessConfiguration(ctx)
	if err != nil {
		return err
	}
	provider, err := s.disputeGateway.RetrieveDispute(ctx, apiKey, event.ObjectID)
	if err != nil {
		return err
	}
	if provider.ID != event.ObjectID || (event.ChargeID != "" && provider.ChargeID != event.ChargeID) ||
		(event.PaymentIntentID != "" && provider.PaymentIntentID != "" && provider.PaymentIntentID != event.PaymentIntentID) {
		return ErrDisputeProviderConflict
	}
	dispute, changed, err := s.repo.ConvergePaymentDispute(ctx, provider, s.now().UTC())
	if errors.Is(err, ErrDisputeOrderNotEligible) {
		return nil
	}
	if err != nil {
		return err
	}
	if changed {
		s.publishDisputeUpdate(dispute.UserID)
	}
	return s.expireDisputeCheckoutSessions(ctx, apiKey, dispute)
}

func (s *Service) publishDisputeUpdate(userID int64) {
	if s != nil && s.events != nil {
		s.events.Publish(userID, PaymentEvent{Status: "dispute_updated"})
	}
}

func (s *Service) processRefundWebhook(ctx context.Context, event StripeWebhookEvent) error {
	if s == nil || s.repo == nil || s.refundGateway == nil || strings.TrimSpace(event.ObjectID) == "" {
		return ErrMalformedWebhookPayload
	}
	refund, err := s.repo.GetPaymentRefundByStripeID(ctx, event.ObjectID)
	if errors.Is(err, sql.ErrNoRows) {
		configuration, apiKey, configErr := s.stripeAccessConfiguration(ctx)
		_ = configuration
		if configErr != nil {
			return configErr
		}
		providerRefund, retrieveErr := s.refundGateway.RetrieveRefund(ctx, apiKey, event.ObjectID)
		if retrieveErr != nil {
			return retrieveErr
		}
		if providerRefund.ChargeID == "" {
			// Keep an unmatched provider event in the inbox for the Admin read
			// model; without a charge identity it cannot be linked safely.
			return nil
		}
		if _, createErr := s.repo.CreateExternalPaymentRefund(ctx, providerRefund, s.now().UTC()); createErr != nil {
			if errors.Is(createErr, credits.ErrInsufficientCredits) || errors.Is(createErr, ErrRefundAmountExceeds) {
				userID, anomalyErr := s.repo.RecordPaymentRefundAnomaly(ctx, providerRefund, "Stripe Dashboard refund requires manual balance recovery", s.now().UTC())
				if anomalyErr == nil {
					s.publishRefundUpdate(userID)
				}
				return anomalyErr
			}
			if errors.Is(createErr, sql.ErrNoRows) || errors.Is(createErr, ErrRefundOrderNotEligible) {
				return nil
			}
			return createErr
		}
		refund, err = s.repo.GetPaymentRefundByStripeID(ctx, event.ObjectID)
		if err != nil {
			return err
		}
		s.publishRefundUpdate(refund.UserID)
		// The provider has already reached a terminal state. Re-read the
		// durable row and apply the same Credits transition as Admin refunds.
		providerRefund.Status = strings.ToLower(strings.TrimSpace(providerRefund.Status))
		_, err = s.convergePaymentRefund(ctx, refund, providerRefund)
		return err
	}
	if err != nil {
		return err
	}
	_, apiKey, err := s.stripeAccessConfiguration(ctx)
	if err != nil {
		return err
	}
	providerRefund, err := s.refundGateway.RetrieveRefund(ctx, apiKey, event.ObjectID)
	if err != nil {
		return err
	}
	_, err = s.convergePaymentRefund(ctx, refund, providerRefund)
	return err
}

func checkoutSignalForWebhookEvent(eventType string) (checkoutSignal, bool) {
	switch eventType {
	case "checkout.session.completed":
		return checkoutSignalCompleted, true
	case "checkout.session.async_payment_succeeded":
		return checkoutSignalAsyncSuccess, true
	case "checkout.session.async_payment_failed":
		return checkoutSignalAsyncFailed, true
	case "checkout.session.expired":
		return checkoutSignalExpired, true
	default:
		return "", false
	}
}

func (s *Service) loadCurrentConfiguration(ctx context.Context) (StoredStripeConfiguration, bool, error) {
	stored, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StoredStripeConfiguration{}, false, nil
	}
	if err != nil {
		return StoredStripeConfiguration{}, false, fmt.Errorf("load current Stripe configuration: %w", err)
	}
	return stored, true, nil
}

func (s *Service) recordFailure(ctx context.Context, actorID int64, code, summary string, metadata map[string]any, publicErr error) error {
	auditErr := s.configuration.RecordConfigurationAudit(ctx, ConfigurationAudit{
		ActorID: actorID, Outcome: "failed", ErrorCode: boundedText(code, 64),
		ErrorSummary: boundedText(summary, 500), Metadata: safeAuditMetadata(metadata),
	})
	if auditErr != nil {
		return errors.Join(publicErr, auditErr)
	}
	return publicErr
}

func stripeConfigurationSummary(stored StoredStripeConfiguration) StripeConfigurationSummary {
	return StripeConfigurationSummary{
		Configured: true, APIKeyConfigured: stored.APIKeyCiphertext != "",
		WebhookSecretConfigured: stored.WebhookSecretCiphertext != "", AccountID: stored.AccountID,
		Mode: stored.Mode, ChargesEnabled: stored.ChargesEnabled, ProductTaxCode: stored.ProductTaxCode,
		ProductReady: stored.StripeProductID != "", WebhookReady: stored.StripeWebhookEndpointID != "",
		PurchasesEnabled: stored.PurchasesEnabled, UpdatedAt: stored.UpdatedAt,
	}
}

func safeAuditMetadata(value map[string]any) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func boundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
