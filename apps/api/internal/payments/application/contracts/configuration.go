package contracts

import (
	"time"

	paymentports "ideaboard/api/internal/payments/ports"
)

// StripeConfigurationSummary is the sanitized configuration state exposed to
// application callers and HTTP adapters. It never contains credential data.
type StripeConfigurationSummary struct {
	Configured                   bool                    `json:"configured"`
	APIKeyConfigured             bool                    `json:"api_key_configured"`
	WebhookSecretConfigured      bool                    `json:"webhook_secret_configured"`
	AccountID                    string                  `json:"account_id,omitempty"`
	Mode                         paymentports.StripeMode `json:"mode,omitempty"`
	ChargesEnabled               bool                    `json:"charges_enabled"`
	PaymentMethodConfigurationID string                  `json:"payment_method_configuration_id,omitempty"`
	ProductTaxCode               string                  `json:"product_tax_code"`
	ProductReady                 bool                    `json:"product_ready"`
	WebhookReady                 bool                    `json:"webhook_ready"`
	PurchasesEnabled             bool                    `json:"purchases_enabled"`
	UpdatedAt                    time.Time               `json:"updated_at,omitempty"`
}

// ConfigureStripeCommand contains the normalized intent for replacing the
// active Stripe configuration. The application layer owns its validation.
type ConfigureStripeCommand struct {
	ActorID                      int64
	APIKey                       string
	PaymentMethodConfigurationID string
	ProductTaxCode               string
	PurchasesEnabled             bool
	WebhookURL                   string
}
