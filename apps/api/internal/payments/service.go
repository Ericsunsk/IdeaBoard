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
	*CheckoutWorkflow
	*StripeFactsWorkflow
	runtime *PaymentRuntime
}

type PaymentCheckoutShutdownEnqueuer interface {
	EnqueuePaymentCheckoutShutdown(context.Context, string) error
}

func (s *Service) EventHub() *EventHub {
	if s == nil {
		return nil
	}
	return s.runtime.events
}

func (s *CheckoutWorkflow) GetStripeConfiguration(ctx context.Context) (StripeConfigurationSummary, error) {
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

func (s *CheckoutWorkflow) ConfigureStripe(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.configuration == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	return s.configureStripe(ctx, command)
}

func (s *CheckoutWorkflow) configureStripe(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
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
	if summary, handled, err := s.updatePurchaseAvailability(ctx, command, current, hasCurrent); handled || err != nil {
		return summary, err
	}
	input, err := s.normalizeStripeConfigurationInput(ctx, command)
	if err != nil {
		return StripeConfigurationSummary{}, err
	}
	if s.configurationGateway == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	input, err = s.resolveStripeConfigurationSecrets(ctx, command, current, hasCurrent, input)
	if err != nil {
		return StripeConfigurationSummary{}, err
	}
	prepared, err := s.configurationGateway.PrepareConfiguration(ctx, input.prepare)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordGatewayFailure(ctx, command.ActorID, input.mode, err)
	}
	if err := s.validatePreparedStripeConfiguration(ctx, command.ActorID, prepared, input); err != nil {
		return StripeConfigurationSummary{}, err
	}
	return s.persistStripeConfiguration(ctx, command, current, hasCurrent, input, prepared)
}

type stripeConfigurationInput struct {
	taxCode, paymentMethodConfigurationID, webhookURL string
	apiKey, currentAPIKey, currentWebhookSecret       string
	mode                                              StripeMode
	prepare                                           PrepareStripeConfiguration
}

func (s *CheckoutWorkflow) updatePurchaseAvailability(ctx context.Context, command ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool) (StripeConfigurationSummary, bool, error) {
	if !hasCurrent || current.PaymentMethodConfigurationID == "" ||
		current.ProductTaxCode != strings.TrimSpace(command.ProductTaxCode) ||
		current.PaymentMethodConfigurationID != strings.TrimSpace(command.PaymentMethodConfigurationID) ||
		current.PurchasesEnabled == command.PurchasesEnabled || strings.TrimSpace(command.APIKey) != "" {
		return StripeConfigurationSummary{}, false, nil
	}
	metadata := safeAuditMetadata(map[string]any{
		"account_id": current.AccountID, "mode": current.Mode, "product_tax_code": current.ProductTaxCode,
		"payment_method_configuration_id": current.PaymentMethodConfigurationID,
		"purchases_enabled":               command.PurchasesEnabled,
	})
	stored, shutdown, err := s.configuration.ReplaceStripeConfiguration(ctx, current.Revision, StoredStripeConfiguration{
		CredentialID: current.CredentialID, AccountID: current.AccountID, Mode: current.Mode, ChargesEnabled: current.ChargesEnabled,
		PaymentMethodConfigurationID: current.PaymentMethodConfigurationID, ProductTaxCode: current.ProductTaxCode,
		PurchasesEnabled: command.PurchasesEnabled, StripeProductID: current.StripeProductID,
		StripeWebhookEndpointID: current.StripeWebhookEndpointID, APIKeyCiphertext: current.APIKeyCiphertext,
		WebhookSecretCiphertext: current.WebhookSecretCiphertext, SecretMaterialKind: current.SecretMaterialKind,
		UpdatedBy: command.ActorID,
	}, ConfigurationAudit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return StripeConfigurationSummary{}, true, fmt.Errorf("update Stripe purchase availability: %w", err)
	}
	s.enqueuePaymentCheckoutShutdown(ctx, shutdown)
	return stripeConfigurationSummary(stored), true, nil
}

func (s *CheckoutWorkflow) normalizeStripeConfigurationInput(ctx context.Context, command ConfigureStripeCommand) (stripeConfigurationInput, error) {
	taxCode, err := normalizeProductTaxCode(command.ProductTaxCode)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_tax_code", err.Error(), nil, err)
	}
	paymentMethodConfigurationID, err := normalizePaymentMethodConfigurationID(command.PaymentMethodConfigurationID)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_payment_method_configuration", err.Error(), nil, err)
	}
	webhookURL, err := normalizeWebhookURL(command.WebhookURL)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_webhook_url", err.Error(), nil, err)
	}
	return stripeConfigurationInput{taxCode: taxCode, paymentMethodConfigurationID: paymentMethodConfigurationID, webhookURL: webhookURL}, nil
}

func (s *CheckoutWorkflow) resolveStripeConfigurationSecrets(ctx context.Context, command ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool, input stripeConfigurationInput) (stripeConfigurationInput, error) {
	if hasCurrent {
		var err error
		input.currentAPIKey, err = secrets.DecryptMaterialString(current.APIKeyCiphertext, current.SecretMaterialKind)
		if err != nil {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe API Key 无法读取", nil, ErrStripeConfigurationInvalid)
		}
		input.currentWebhookSecret, err = secrets.DecryptMaterialString(current.WebhookSecretCiphertext, current.SecretMaterialKind)
		if err != nil {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe Webhook Secret 无法读取", nil, ErrStripeConfigurationInvalid)
		}
	}
	input.apiKey = strings.TrimSpace(command.APIKey)
	if input.apiKey == "" {
		if !hasCurrent {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "api_key_required", ErrStripeAPIKeyRequired.Error(), nil, ErrStripeAPIKeyRequired)
		}
		input.apiKey = input.currentAPIKey
	}
	mode, err := ClassifyStripeAPIKey(input.apiKey)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_key", err.Error(), map[string]any{"mode": "unknown"}, err)
	}
	input.mode = mode
	input.prepare = PrepareStripeConfiguration{APIKey: input.apiKey, Mode: input.mode, PurchasesEnabled: command.PurchasesEnabled,
		ProductTaxCode: input.taxCode, PaymentMethodConfigurationID: input.paymentMethodConfigurationID, WebhookURL: input.webhookURL}
	if hasCurrent {
		input.prepare.ExistingAccountID = current.AccountID
		input.prepare.ExistingProductID = current.StripeProductID
		input.prepare.ExistingWebhookEndpointID = current.StripeWebhookEndpointID
		input.prepare.ExistingWebhookSecret = input.currentWebhookSecret
	}
	return input, nil
}

func (s *CheckoutWorkflow) recordGatewayFailure(ctx context.Context, actorID int64, mode StripeMode, err error) error {
	code, summary, publicErr := gatewayFailure(err)
	return s.recordFailure(ctx, actorID, string(code), summary, map[string]any{"mode": mode}, wrapPublicError(publicErr, summary))
}

func (s *CheckoutWorkflow) validatePreparedStripeConfiguration(ctx context.Context, actorID int64, prepared PreparedStripeConfiguration, input stripeConfigurationInput) error {
	if err := validatePreparedConfiguration(prepared, input.mode, input.paymentMethodConfigurationID); err != nil {
		code := "invalid_response"
		if errors.Is(err, ErrStripeCapabilityUnavailable) {
			code = "capability_missing"
		}
		return s.recordFailure(ctx, actorID, code, err.Error(), map[string]any{"mode": input.mode}, err)
	}
	return nil
}

func (s *CheckoutWorkflow) persistStripeConfiguration(ctx context.Context, command ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool, input stripeConfigurationInput, prepared PreparedStripeConfiguration) (StripeConfigurationSummary, error) {
	credentialChanged := !hasCurrent || input.apiKey != input.currentAPIKey || prepared.AccountID != current.AccountID ||
		prepared.Mode != current.Mode || prepared.ChargesEnabled != current.ChargesEnabled || prepared.PaymentMethodConfigurationID != current.PaymentMethodConfigurationID ||
		prepared.ProductID != current.StripeProductID || prepared.WebhookEndpointID != current.StripeWebhookEndpointID || prepared.WebhookSecret != input.currentWebhookSecret
	credentialID, encryptedAPIKey, encryptedWebhookSecret, err := s.persistCredentialMaterial(ctx, command, current, input, prepared, credentialChanged)
	if err != nil {
		return StripeConfigurationSummary{}, err
	}
	metadata := safeAuditMetadata(map[string]any{"account_id": prepared.AccountID, "mode": input.mode, "product_tax_code": input.taxCode,
		"payment_method_configuration_id": prepared.PaymentMethodConfigurationID, "purchases_enabled": command.PurchasesEnabled})
	expectedRevision := int64(0)
	if hasCurrent {
		expectedRevision = current.Revision
	}
	stored, shutdown, err := s.configuration.ReplaceStripeConfiguration(ctx, expectedRevision, StoredStripeConfiguration{
		CredentialID: credentialID, AccountID: prepared.AccountID, Mode: prepared.Mode, ChargesEnabled: prepared.ChargesEnabled,
		PaymentMethodConfigurationID: prepared.PaymentMethodConfigurationID, ProductTaxCode: input.taxCode, PurchasesEnabled: command.PurchasesEnabled,
		StripeProductID: prepared.ProductID, StripeWebhookEndpointID: prepared.WebhookEndpointID, APIKeyCiphertext: encryptedAPIKey,
		WebhookSecretCiphertext: encryptedWebhookSecret, SecretMaterialKind: secrets.MaterialKindEncrypted, UpdatedBy: command.ActorID,
	}, ConfigurationAudit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return StripeConfigurationSummary{}, fmt.Errorf("publish Stripe configuration: %w", err)
	}
	s.enqueuePaymentCheckoutShutdown(ctx, shutdown)
	return stripeConfigurationSummary(stored), nil
}

func (s *CheckoutWorkflow) persistCredentialMaterial(ctx context.Context, command ConfigureStripeCommand, current StoredStripeConfiguration, input stripeConfigurationInput, prepared PreparedStripeConfiguration, credentialChanged bool) (string, string, string, error) {
	if !credentialChanged {
		return current.CredentialID, current.APIKeyCiphertext, current.WebhookSecretCiphertext, nil
	}
	encryptedAPIKey, err := secrets.EncryptString(input.apiKey)
	if err != nil {
		return "", "", "", s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe API Key 加密失败", map[string]any{"mode": input.mode}, ErrStripeConfigurationInvalid)
	}
	encryptedWebhookSecret, err := secrets.EncryptString(prepared.WebhookSecret)
	if err != nil {
		return "", "", "", s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe Webhook Secret 加密失败", map[string]any{"mode": input.mode}, ErrStripeConfigurationInvalid)
	}
	return "", encryptedAPIKey, encryptedWebhookSecret, nil
}

func (s *CheckoutWorkflow) enqueuePaymentCheckoutShutdown(ctx context.Context, shutdown *PaymentCheckoutShutdown) {
	if shutdown == nil || s == nil || s.shutdownEnqueuer == nil {
		return
	}
	if err := s.shutdownEnqueuer.EnqueuePaymentCheckoutShutdown(ctx, shutdown.ID); err != nil {
		slog.Error("enqueue Checkout shutdown failed; periodic River recovery will retry", "shutdown_id", shutdown.ID, "error", err)
	}
}

func (s *StripeFactsWorkflow) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.configuration == nil || s.webhookInbox == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	return s.receiveStripeWebhook(ctx, raw, signature)
}

func (s *StripeFactsWorkflow) receiveStripeWebhook(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.configuration == nil || s.webhookInbox == nil || s.verifier == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	stored, event, now, err := s.verifyStripeWebhook(ctx, raw, signature)
	if err != nil {
		return StripeWebhookReceipt{}, err
	}
	receipt, recognized, err := s.recordStripeWebhookReceipt(ctx, event, now)
	if err != nil {
		return StripeWebhookReceipt{}, err
	}
	if !recognized {
		return receipt, nil
	}
	if err := s.processStripeWebhookEvent(ctx, event, stored, now); err != nil {
		return StripeWebhookReceipt{}, err
	}
	return receipt, nil
}

func (s *StripeFactsWorkflow) verifyStripeWebhook(ctx context.Context, raw []byte, signature string) (StoredStripeConfiguration, StripeWebhookEvent, time.Time, error) {
	if len(raw) == 0 || strings.TrimSpace(signature) == "" {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, ErrMalformedWebhookPayload
	}
	credentials, err := s.configuration.ListStripeCredentials(ctx)
	if err != nil {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, fmt.Errorf("load Stripe webhook credentials: %w", err)
	}
	if len(credentials) == 0 {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, ErrStripeConfigurationNotFound
	}
	now := s.runtime.now()
	var stored StoredStripeConfiguration
	var event StripeWebhookEvent
	var verifyErr error
	for _, credential := range credentials {
		secret, decryptErr := secrets.DecryptMaterialString(credential.WebhookSecretCiphertext, credential.SecretMaterialKind)
		if decryptErr != nil {
			return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, ErrStripeConfigurationInvalid
		}
		candidate, candidateErr := s.verifier.Verify(ctx, raw, signature, secret, StripeWebhookTolerance, now)
		if candidateErr == nil {
			stored, event, verifyErr = credential, candidate, nil
			break
		}
		verifyErr = candidateErr
	}
	if stored.CredentialID == "" {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, verifyErr
	}
	if err := validateStripeWebhookEvent(event); err != nil {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, err
	}
	if event.Livemode != (stored.Mode == StripeModeLive) {
		return StoredStripeConfiguration{}, StripeWebhookEvent{}, time.Time{}, ErrMalformedWebhookPayload
	}
	return stored, event, now, nil
}

func (s *StripeFactsWorkflow) recordStripeWebhookReceipt(ctx context.Context, event StripeWebhookEvent, now time.Time) (StripeWebhookReceipt, bool, error) {
	recognized := isRecognizedStripeWebhookEvent(event.Type)
	result := "received"
	var processedAt *time.Time
	if !recognized {
		result = "ignored"
		processedAt = &now
	}
	duplicate, err := s.webhookInbox.InsertStripeWebhookEvent(ctx, event, result, processedAt)
	if err != nil {
		return StripeWebhookReceipt{}, false, err
	}
	return StripeWebhookReceipt{EventID: event.ID, Duplicate: duplicate, Ignored: !recognized}, recognized, nil
}

func (s *StripeFactsWorkflow) processStripeWebhookEvent(ctx context.Context, event StripeWebhookEvent, stored StoredStripeConfiguration, now time.Time) error {
	signal, checkoutLifecycleEvent := checkoutSignalForWebhookEvent(event.Type)
	if !checkoutLifecycleEvent {
		if strings.HasPrefix(event.Type, "refund.") {
			if err := s.processRefundWebhook(ctx, event, stored.CredentialID); err != nil {
				_ = s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", operationalErrorSummary(err, "Stripe refund webhook processing failed"), now)
				return err
			}
		}
		if strings.HasPrefix(event.Type, "charge.dispute.") {
			if err := s.processDisputeWebhook(ctx, event, stored.CredentialID); err != nil {
				_ = s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "failed", operationalErrorSummary(err, "Stripe dispute webhook processing failed"), now)
				return err
			}
		}
		return s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now)
	}
	return s.processCheckoutWebhook(ctx, event, stored, signal, now)
}

func (s *StripeFactsWorkflow) processCheckoutWebhook(ctx context.Context, event StripeWebhookEvent, stored StoredStripeConfiguration, signal checkoutSignal, now time.Time) error {
	if s.checkoutGateway == nil {
		return fmt.Errorf("payments Checkout gateway unavailable")
	}
	if s.checkoutStore == nil || s.orderLifecycle == nil {
		return fmt.Errorf("payments Checkout persistence unavailable")
	}
	apiKey, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return ErrStripeConfigurationInvalid
	}
	session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: event.ObjectID})
	if err != nil {
		return checkoutOperationError(err)
	}
	if session.ID != event.ObjectID {
		return ErrMalformedWebhookPayload
	}
	order, err := s.checkoutStore.GetPaymentOrderByCheckoutSession(ctx, stored.AccountID, session.ID)
	if err != nil {
		return fmt.Errorf("resolve payment order for Stripe Session: %w", err)
	}
	if _, err := s.orderLifecycle.convergeCheckoutSession(ctx, order, session, signal); err != nil {
		return err
	}
	return s.webhookInbox.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now)
}

func (s *StripeFactsWorkflow) processDisputeWebhook(ctx context.Context, event StripeWebhookEvent, credentialID string) error {
	if s == nil || s.convergenceStore == nil || s.checkoutStore == nil || s.disputeGateway == nil || strings.TrimSpace(event.ObjectID) == "" {
		return ErrMalformedWebhookPayload
	}
	_, apiKey, err := s.stripeCredentialAccess(ctx, credentialID)
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
	dispute, changed, err := s.convergenceStore.ConvergePaymentDispute(ctx, provider, s.runtime.now().UTC())
	if errors.Is(err, ErrDisputeOrderNotEligible) {
		return nil
	}
	if err != nil {
		return err
	}
	if changed {
		s.publishDisputeUpdate(dispute.UserID)
	}
	return s.expireDisputeCheckoutSessions(ctx, dispute)
}

func (s *StripeFactsWorkflow) publishDisputeUpdate(userID int64) {
	if s != nil && s.runtime.events != nil {
		s.runtime.events.Publish(userID, PaymentEvent{Status: "dispute_updated"})
	}
}

func (s *StripeFactsWorkflow) processRefundWebhook(ctx context.Context, event StripeWebhookEvent, credentialID string) error {
	if s == nil || s.convergenceStore == nil || s.refundGateway == nil || strings.TrimSpace(event.ObjectID) == "" {
		return ErrMalformedWebhookPayload
	}
	refund, err := s.convergenceStore.GetPaymentRefundByStripeID(ctx, event.ObjectID)
	if errors.Is(err, sql.ErrNoRows) {
		_, apiKey, configErr := s.stripeCredentialAccess(ctx, credentialID)
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
		if _, createErr := s.convergenceStore.CreateExternalPaymentRefund(ctx, providerRefund, s.runtime.now().UTC()); createErr != nil {
			if errors.Is(createErr, credits.ErrInsufficientCredits) || errors.Is(createErr, ErrRefundAmountExceeds) {
				userID, anomalyErr := s.convergenceStore.RecordPaymentRefundAnomaly(ctx, providerRefund, "Stripe Dashboard refund requires manual balance recovery", s.runtime.now().UTC())
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
		refund, err = s.convergenceStore.GetPaymentRefundByStripeID(ctx, event.ObjectID)
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
	_, apiKey, err := s.stripeOrderAccess(ctx, refund.PaymentOrderID)
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

func (s *CheckoutWorkflow) loadCurrentConfiguration(ctx context.Context) (StoredStripeConfiguration, bool, error) {
	stored, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StoredStripeConfiguration{}, false, nil
	}
	if err != nil {
		return StoredStripeConfiguration{}, false, fmt.Errorf("load current Stripe configuration: %w", err)
	}
	return stored, true, nil
}

func (s *CheckoutWorkflow) recordFailure(ctx context.Context, actorID int64, code, summary string, metadata map[string]any, publicErr error) error {
	auditErr := s.configuration.RecordConfigurationAudit(ctx, ConfigurationAudit{
		ActorID: actorID, Outcome: "failed", ErrorCode: boundedText(code, 64),
		ErrorSummary: boundedText(summary, 500), Metadata: safeAuditMetadata(metadata),
	})
	if auditErr != nil {
		return errors.Join(publicErr, auditErr)
	}
	return publicErr
}

func (s *StripeFactsWorkflow) stripeCredentialAccess(ctx context.Context, credentialID string) (StoredStripeConfiguration, string, error) {
	configuration, err := s.configuration.GetStripeCredential(ctx, credentialID)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrStripeConfigurationInvalid
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrStripeConfigurationInvalid
	}
	return configuration, apiKey, nil
}

func (s *StripeFactsWorkflow) stripeOrderAccess(ctx context.Context, orderID string) (StoredStripeConfiguration, string, error) {
	configuration, err := s.configuration.GetStripeCredentialForOrder(ctx, orderID)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrStripeConfigurationInvalid
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrStripeConfigurationInvalid
	}
	return configuration, apiKey, nil
}

func stripeConfigurationSummary(stored StoredStripeConfiguration) StripeConfigurationSummary {
	return StripeConfigurationSummary{
		Configured: true, APIKeyConfigured: stored.APIKeyCiphertext != "",
		WebhookSecretConfigured: stored.WebhookSecretCiphertext != "", AccountID: stored.AccountID,
		Mode: stored.Mode, ChargesEnabled: stored.ChargesEnabled, ProductTaxCode: stored.ProductTaxCode,
		PaymentMethodConfigurationID: stored.PaymentMethodConfigurationID,
		ProductReady:                 stored.StripeProductID != "", WebhookReady: stored.StripeWebhookEndpointID != "",
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
