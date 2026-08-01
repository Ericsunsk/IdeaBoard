package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/secrets"
)

type Service struct {
	repo            *Repository
	gateway         StripeGateway
	checkoutGateway CheckoutGateway
	verifier        StripeWebhookVerifier
	wallets         credits.WalletReader
	events          *EventHub
	now             func() time.Time
}

type ServiceDependencies struct {
	Repository           *Repository
	ConfigurationGateway StripeGateway
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
	return &Service{
		repo: dependencies.Repository, gateway: dependencies.ConfigurationGateway,
		checkoutGateway: dependencies.CheckoutGateway, verifier: dependencies.WebhookVerifier,
		wallets: dependencies.Wallets, events: events, now: now,
	}
}

func (s *Service) EventHub() *EventHub {
	if s == nil {
		return nil
	}
	return s.events
}

func (s *Service) GetStripeConfiguration(ctx context.Context) (StripeConfigurationSummary, error) {
	if s == nil || s.repo == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments service unavailable")
	}
	stored, err := s.repo.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StripeConfigurationSummary{}, nil
	}
	if err != nil {
		return StripeConfigurationSummary{}, fmt.Errorf("get Stripe configuration: %w", err)
	}
	return stripeConfigurationSummary(stored), nil
}

func (s *Service) ConfigureStripe(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.repo == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	var result StripeConfigurationSummary
	err := s.repo.WithStripeConfigurationExclusiveLock(ctx, func(lockedContext context.Context) error {
		var err error
		result, err = s.configureStripeLocked(lockedContext, command)
		return err
	})
	return result, err
}

func (s *Service) configureStripeLocked(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.repo == nil || s.gateway == nil {
		return StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	if command.ActorID <= 0 {
		return StripeConfigurationSummary{}, fmt.Errorf("invalid payment admin actor")
	}
	taxCode, err := normalizeProductTaxCode(command.ProductTaxCode)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "invalid_tax_code", err.Error(), nil, err)
	}
	webhookURL, err := normalizeWebhookURL(command.WebhookURL)
	if err != nil {
		return StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, "invalid_webhook_url", err.Error(), nil, err)
	}

	current, hasCurrent, err := s.loadCurrentConfiguration(ctx)
	if err != nil {
		return StripeConfigurationSummary{}, err
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

	prepare := PrepareStripeConfiguration{APIKey: apiKey, Mode: mode, ProductTaxCode: taxCode, WebhookURL: webhookURL}
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
		publicErr, code, summary := gatewayFailure(err)
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
	stored, err := s.repo.ReplaceStripeConfiguration(ctx, StoredStripeConfiguration{
		AccountID: prepared.AccountID, Mode: prepared.Mode, ChargesEnabled: prepared.ChargesEnabled,
		ProductTaxCode: taxCode, PurchasesEnabled: command.PurchasesEnabled,
		StripeProductID: prepared.ProductID, StripeWebhookEndpointID: prepared.WebhookEndpointID,
		APIKeyCiphertext: encryptedAPIKey, WebhookSecretCiphertext: encryptedWebhookSecret,
		SecretMaterialKind: secrets.MaterialKindEncrypted, UpdatedBy: command.ActorID,
	}, ConfigurationAudit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return StripeConfigurationSummary{}, fmt.Errorf("publish Stripe configuration: %w", err)
	}
	return stripeConfigurationSummary(stored), nil
}

func (s *Service) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.repo == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	var receipt StripeWebhookReceipt
	err := s.repo.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		var err error
		receipt, err = s.receiveStripeWebhookLocked(lockedContext, raw, signature)
		return err
	})
	return receipt, err
}

func (s *Service) receiveStripeWebhookLocked(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.repo == nil || s.verifier == nil {
		return StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	if len(raw) == 0 || strings.TrimSpace(signature) == "" {
		return StripeWebhookReceipt{}, ErrMalformedWebhookPayload
	}
	stored, err := s.repo.GetStripeConfiguration(ctx)
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
	duplicate, err := s.repo.InsertStripeWebhookEvent(ctx, event, result, processedAt)
	if err != nil {
		return StripeWebhookReceipt{}, err
	}
	receipt := StripeWebhookReceipt{EventID: event.ID, Duplicate: duplicate, Ignored: !recognized}
	if !recognized {
		return receipt, nil
	}
	if event.Type == "checkout.session.async_payment_failed" || event.Type == "checkout.session.expired" {
		failed, changed, err := s.repo.MarkPaymentOrderFailedBySession(ctx, stored.AccountID, event.ObjectID)
		if err != nil {
			return StripeWebhookReceipt{}, err
		}
		if changed {
			s.events.Publish(failed.UserID, PaymentEvent{PublicToken: failed.PublicToken, Status: "failed"})
		}
		if err := s.repo.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now); err != nil {
			return StripeWebhookReceipt{}, err
		}
		return receipt, nil
	}
	if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" {
		if err := s.repo.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now); err != nil {
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
	if session.IsPaid() {
		converged, err := s.repo.ConvergePaidPaymentOrder(ctx, paidCheckoutFactsFromSession(stored.AccountID, session, now), now)
		if err != nil {
			return StripeWebhookReceipt{}, err
		}
		if !converged.Replayed {
			s.events.Publish(converged.Order.UserID, PaymentEvent{PublicToken: converged.Order.PublicToken, Status: "credited"})
		}
	} else if event.Type == "checkout.session.completed" {
		pending, changed, err := s.repo.MarkPaymentOrderAsyncPendingBySession(ctx, stored.AccountID, session.ID)
		if err != nil {
			return StripeWebhookReceipt{}, err
		}
		if changed {
			s.events.Publish(pending.UserID, PaymentEvent{PublicToken: pending.PublicToken, Status: "processing"})
		}
	}
	if err := s.repo.MarkStripeWebhookEventProcessed(ctx, event.ID, "processed", "", now); err != nil {
		return StripeWebhookReceipt{}, err
	}
	return receipt, nil
}

func (s *Service) loadCurrentConfiguration(ctx context.Context) (StoredStripeConfiguration, bool, error) {
	stored, err := s.repo.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StoredStripeConfiguration{}, false, nil
	}
	if err != nil {
		return StoredStripeConfiguration{}, false, fmt.Errorf("load current Stripe configuration: %w", err)
	}
	return stored, true, nil
}

func (s *Service) recordFailure(ctx context.Context, actorID int64, code, summary string, metadata map[string]any, publicErr error) error {
	auditErr := s.repo.RecordConfigurationAudit(ctx, ConfigurationAudit{
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
