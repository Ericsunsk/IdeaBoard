package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
)

// Store is the durable boundary required to publish Stripe configuration.
// Replace is injected as a function because the resulting checkout shutdown
// is consumed by the checkout context and must not introduce a package cycle.
type Store interface {
	GetStripeConfiguration(context.Context) (StoredStripeConfiguration, error)
	RecordConfigurationAudit(context.Context, Audit) error
}

type ReplaceFunc func(context.Context, int64, StoredStripeConfiguration, Audit) (StoredStripeConfiguration, *PaymentCheckoutShutdown, error)
type ShutdownEnqueuer interface {
	EnqueuePaymentCheckoutShutdown(context.Context, string) error
}
type Dependencies struct {
	Store    Store
	Replace  ReplaceFunc
	Gateway  paymentports.StripeGateway
	Shutdown ShutdownEnqueuer
}

type Service struct {
	store    Store
	replace  ReplaceFunc
	gateway  paymentports.StripeGateway
	shutdown ShutdownEnqueuer
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.Store == nil || deps.Replace == nil || deps.Gateway == nil || deps.Shutdown == nil {
		return nil, fmt.Errorf("configuration service requires store, replace function, gateway, and shutdown enqueuer")
	}
	return &Service{store: deps.Store, replace: deps.Replace, gateway: deps.Gateway, shutdown: deps.Shutdown}, nil
}

func (s *Service) GetStripeConfiguration(ctx context.Context) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.store == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments service unavailable")
	}
	stored, err := s.store.GetStripeConfiguration(ctx)
	if IsNotFound(err) {
		return paymentcontracts.StripeConfigurationSummary{}, nil
	}
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("get Stripe configuration: %w", err)
	}
	return Summary(stored), nil
}

func (s *Service) ConfigureStripe(ctx context.Context, command paymentcontracts.ConfigureStripeCommand) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.store == nil || s.replace == nil || s.gateway == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	if command.ActorID <= 0 {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("invalid payment admin actor")
	}
	current, hasCurrent, err := s.loadCurrent(ctx)
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, err
	}
	if summary, handled, err := s.updatePurchaseAvailability(ctx, command, current, hasCurrent); handled || err != nil {
		return summary, err
	}
	input, err := s.normalizeInput(ctx, command)
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, err
	}
	input, err = s.resolveSecrets(ctx, command, current, hasCurrent, input)
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, err
	}
	prepared, err := s.gateway.PrepareConfiguration(ctx, input.prepare)
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, s.recordGatewayFailure(ctx, command.ActorID, input.mode, err)
	}
	if err := ValidatePreparedConfiguration(prepared, input.mode, input.paymentMethodConfigurationID); err != nil {
		code := "invalid_response"
		if errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable) {
			code = "capability_missing"
		}
		return paymentcontracts.StripeConfigurationSummary{}, s.recordFailure(ctx, command.ActorID, code, err.Error(), map[string]any{"mode": input.mode}, err)
	}
	return s.persist(ctx, command, current, hasCurrent, input, prepared)
}

type stripeConfigurationInput struct {
	taxCode, paymentMethodConfigurationID, webhookURL string
	apiKey, currentAPIKey, currentWebhookSecret       string
	mode                                              paymentports.StripeMode
	prepare                                           paymentports.PrepareStripeConfiguration
}

func (s *Service) updatePurchaseAvailability(ctx context.Context, command paymentcontracts.ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool) (paymentcontracts.StripeConfigurationSummary, bool, error) {
	if !hasCurrent || current.PaymentMethodConfigurationID == "" || current.ProductTaxCode != strings.TrimSpace(command.ProductTaxCode) || current.PaymentMethodConfigurationID != strings.TrimSpace(command.PaymentMethodConfigurationID) || current.PurchasesEnabled == command.PurchasesEnabled || strings.TrimSpace(command.APIKey) != "" {
		return paymentcontracts.StripeConfigurationSummary{}, false, nil
	}
	metadata := SafeAuditMetadata(map[string]any{"account_id": current.AccountID, "mode": current.Mode, "product_tax_code": current.ProductTaxCode, "payment_method_configuration_id": current.PaymentMethodConfigurationID, "purchases_enabled": command.PurchasesEnabled})
	stored, shutdown, err := s.replace(ctx, current.Revision, StoredStripeConfiguration{CredentialID: current.CredentialID, AccountID: current.AccountID, Mode: current.Mode, ChargesEnabled: current.ChargesEnabled, PaymentMethodConfigurationID: current.PaymentMethodConfigurationID, ProductTaxCode: current.ProductTaxCode, PurchasesEnabled: command.PurchasesEnabled, StripeProductID: current.StripeProductID, StripeWebhookEndpointID: current.StripeWebhookEndpointID, APIKeyCiphertext: current.APIKeyCiphertext, WebhookSecretCiphertext: current.WebhookSecretCiphertext, SecretMaterialKind: current.SecretMaterialKind, UpdatedBy: command.ActorID}, Audit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, true, fmt.Errorf("update Stripe purchase availability: %w", err)
	}
	s.enqueue(ctx, shutdown)
	return Summary(stored), true, nil
}

func (s *Service) normalizeInput(ctx context.Context, command paymentcontracts.ConfigureStripeCommand) (stripeConfigurationInput, error) {
	taxCode, err := NormalizeProductTaxCode(command.ProductTaxCode)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_tax_code", err.Error(), nil, err)
	}
	pmID, err := NormalizePaymentMethodConfigurationID(command.PaymentMethodConfigurationID)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_payment_method_configuration", err.Error(), nil, err)
	}
	webhookURL, err := NormalizeWebhookURL(command.WebhookURL)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_webhook_url", err.Error(), nil, err)
	}
	return stripeConfigurationInput{taxCode: taxCode, paymentMethodConfigurationID: pmID, webhookURL: webhookURL}, nil
}

func (s *Service) resolveSecrets(ctx context.Context, command paymentcontracts.ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool, input stripeConfigurationInput) (stripeConfigurationInput, error) {
	if hasCurrent {
		var err error
		input.currentAPIKey, err = secrets.DecryptMaterialString(current.APIKeyCiphertext, current.SecretMaterialKind)
		if err != nil {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe API Key 无法读取", nil, paymentcontracts.ErrStripeConfigurationInvalid)
		}
		input.currentWebhookSecret, err = secrets.DecryptMaterialString(current.WebhookSecretCiphertext, current.SecretMaterialKind)
		if err != nil {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "secret_unavailable", "已保存的 Stripe Webhook Secret 无法读取", nil, paymentcontracts.ErrStripeConfigurationInvalid)
		}
	}
	input.apiKey = strings.TrimSpace(command.APIKey)
	if input.apiKey == "" {
		if !hasCurrent {
			return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "api_key_required", paymentcontracts.ErrStripeAPIKeyRequired.Error(), nil, paymentcontracts.ErrStripeAPIKeyRequired)
		}
		input.apiKey = input.currentAPIKey
	}
	mode, err := paymentports.ClassifyStripeAPIKey(input.apiKey)
	if err != nil {
		return stripeConfigurationInput{}, s.recordFailure(ctx, command.ActorID, "invalid_key", err.Error(), map[string]any{"mode": "unknown"}, err)
	}
	input.mode = mode
	input.prepare = paymentports.PrepareStripeConfiguration{APIKey: input.apiKey, Mode: input.mode, PurchasesEnabled: command.PurchasesEnabled, ProductTaxCode: input.taxCode, PaymentMethodConfigurationID: input.paymentMethodConfigurationID, WebhookURL: input.webhookURL}
	if hasCurrent {
		input.prepare.ExistingAccountID = current.AccountID
		input.prepare.ExistingProductID = current.StripeProductID
		input.prepare.ExistingWebhookEndpointID = current.StripeWebhookEndpointID
		input.prepare.ExistingWebhookSecret = input.currentWebhookSecret
	}
	return input, nil
}

func (s *Service) recordGatewayFailure(ctx context.Context, actorID int64, mode paymentports.StripeMode, err error) error {
	code, summary, publicErr := GatewayFailure(err)
	return s.recordFailure(ctx, actorID, string(code), summary, map[string]any{"mode": mode}, WrapPublicError(publicErr, summary))
}

func (s *Service) persist(ctx context.Context, command paymentcontracts.ConfigureStripeCommand, current StoredStripeConfiguration, hasCurrent bool, input stripeConfigurationInput, prepared paymentports.PreparedStripeConfiguration) (paymentcontracts.StripeConfigurationSummary, error) {
	credentialChanged := !hasCurrent || input.apiKey != input.currentAPIKey || prepared.AccountID != current.AccountID || prepared.Mode != current.Mode || prepared.ChargesEnabled != current.ChargesEnabled || prepared.PaymentMethodConfigurationID != current.PaymentMethodConfigurationID || prepared.ProductID != current.StripeProductID || prepared.WebhookEndpointID != current.StripeWebhookEndpointID || prepared.WebhookSecret != input.currentWebhookSecret
	credentialID, encryptedAPIKey, encryptedWebhookSecret, err := s.persistCredentialMaterial(ctx, command, current, input, prepared, credentialChanged)
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, err
	}
	metadata := SafeAuditMetadata(map[string]any{"account_id": prepared.AccountID, "mode": input.mode, "product_tax_code": input.taxCode, "payment_method_configuration_id": prepared.PaymentMethodConfigurationID, "purchases_enabled": command.PurchasesEnabled})
	expectedRevision := int64(0)
	if hasCurrent {
		expectedRevision = current.Revision
	}
	stored, shutdown, err := s.replace(ctx, expectedRevision, StoredStripeConfiguration{CredentialID: credentialID, AccountID: prepared.AccountID, Mode: prepared.Mode, ChargesEnabled: prepared.ChargesEnabled, PaymentMethodConfigurationID: prepared.PaymentMethodConfigurationID, ProductTaxCode: input.taxCode, PurchasesEnabled: command.PurchasesEnabled, StripeProductID: prepared.ProductID, StripeWebhookEndpointID: prepared.WebhookEndpointID, APIKeyCiphertext: encryptedAPIKey, WebhookSecretCiphertext: encryptedWebhookSecret, SecretMaterialKind: secrets.MaterialKindEncrypted, UpdatedBy: command.ActorID}, Audit{ActorID: command.ActorID, Outcome: "succeeded", Metadata: metadata})
	if err != nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("publish Stripe configuration: %w", err)
	}
	s.enqueue(ctx, shutdown)
	return Summary(stored), nil
}

func (s *Service) persistCredentialMaterial(ctx context.Context, command paymentcontracts.ConfigureStripeCommand, current StoredStripeConfiguration, input stripeConfigurationInput, prepared paymentports.PreparedStripeConfiguration, changed bool) (string, string, string, error) {
	if !changed {
		return current.CredentialID, current.APIKeyCiphertext, current.WebhookSecretCiphertext, nil
	}
	apiKey, err := secrets.EncryptString(input.apiKey)
	if err != nil {
		return "", "", "", s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe API Key 加密失败", map[string]any{"mode": input.mode}, paymentcontracts.ErrStripeConfigurationInvalid)
	}
	webhook, err := secrets.EncryptString(prepared.WebhookSecret)
	if err != nil {
		return "", "", "", s.recordFailure(ctx, command.ActorID, "encryption_failed", "Stripe Webhook Secret 加密失败", map[string]any{"mode": input.mode}, paymentcontracts.ErrStripeConfigurationInvalid)
	}
	return "", apiKey, webhook, nil
}

func (s *Service) enqueue(ctx context.Context, shutdown *PaymentCheckoutShutdown) {
	if shutdown == nil || s == nil || s.shutdown == nil {
		return
	}
	if err := s.shutdown.EnqueuePaymentCheckoutShutdown(ctx, shutdown.ID); err != nil {
		slog.Error("enqueue Checkout shutdown failed; periodic River recovery will retry", "shutdown_id", shutdown.ID, "error", err)
	}
}
func (s *Service) loadCurrent(ctx context.Context) (StoredStripeConfiguration, bool, error) {
	stored, err := s.store.GetStripeConfiguration(ctx)
	if IsNotFound(err) {
		return StoredStripeConfiguration{}, false, nil
	}
	if err != nil {
		return StoredStripeConfiguration{}, false, fmt.Errorf("load current Stripe configuration: %w", err)
	}
	return stored, true, nil
}
func (s *Service) recordFailure(ctx context.Context, actorID int64, code, summary string, metadata map[string]any, publicErr error) error {
	auditErr := s.store.RecordConfigurationAudit(ctx, Audit{ActorID: actorID, Outcome: "failed", ErrorCode: BoundedText(code, 64), ErrorSummary: BoundedText(summary, 500), Metadata: SafeAuditMetadata(metadata)})
	if auditErr != nil {
		return errors.Join(publicErr, auditErr)
	}
	return publicErr
}

func Summary(stored StoredStripeConfiguration) paymentcontracts.StripeConfigurationSummary {
	return paymentcontracts.StripeConfigurationSummary{Configured: true, APIKeyConfigured: stored.APIKeyCiphertext != "", WebhookSecretConfigured: stored.WebhookSecretCiphertext != "", AccountID: stored.AccountID, Mode: stored.Mode, ChargesEnabled: stored.ChargesEnabled, ProductTaxCode: stored.ProductTaxCode, PaymentMethodConfigurationID: stored.PaymentMethodConfigurationID, ProductReady: stored.StripeProductID != "", WebhookReady: stored.StripeWebhookEndpointID != "", PurchasesEnabled: stored.PurchasesEnabled, UpdatedAt: stored.UpdatedAt}
}
func SafeAuditMetadata(value map[string]any) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}
func BoundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
