package ports

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidStripeAPIKey = errors.New("无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key")

const maxStripeAPIKeyLength = 512

// StripeMode identifies the Stripe environment associated with a credential.
// It is intentionally provider-neutral so application code can depend on the
// configuration seam without importing the Stripe SDK or infrastructure.
type StripeMode string

const (
	StripeModeTest StripeMode = "test"
	StripeModeLive StripeMode = "live"
)

// ClassifyStripeAPIKey validates a Stripe secret key and identifies its mode.
// Keeping this at the configuration port gives callers a provider-neutral
// credential check without importing the payments composition package.
func ClassifyStripeAPIKey(raw string) (StripeMode, error) {
	key := strings.TrimSpace(raw)
	if len(key) > maxStripeAPIKeyLength {
		return "", ErrInvalidStripeAPIKey
	}
	switch {
	case hasStripeKeyPrefix(key, "sk_test_"), hasStripeKeyPrefix(key, "rk_test_"):
		return StripeModeTest, nil
	case hasStripeKeyPrefix(key, "sk_live_"), hasStripeKeyPrefix(key, "rk_live_"):
		return StripeModeLive, nil
	default:
		return "", ErrInvalidStripeAPIKey
	}
}

func hasStripeKeyPrefix(key, prefix string) bool {
	return strings.HasPrefix(key, prefix) && len(key) > len(prefix)
}

// PrepareStripeConfiguration contains the provider inputs needed to validate
// and materialize the active Stripe configuration.
type PrepareStripeConfiguration struct {
	APIKey                       string
	Mode                         StripeMode
	PurchasesEnabled             bool
	ProductTaxCode               string
	PaymentMethodConfigurationID string
	WebhookURL                   string
	ExistingAccountID            string
	ExistingProductID            string
	ExistingWebhookEndpointID    string
	ExistingWebhookSecret        string
}

// PreparedStripeConfiguration contains normalized provider facts that are
// safe for the application layer to persist after validation.
type PreparedStripeConfiguration struct {
	AccountID                    string
	Mode                         StripeMode
	ChargesEnabled               bool
	PaymentMethodConfigurationID string
	ProductID                    string
	WebhookEndpointID            string
	WebhookSecret                string
}

// StripeGateway is the provider boundary for Stripe configuration operations.
type StripeGateway interface {
	PrepareConfiguration(context.Context, PrepareStripeConfiguration) (PreparedStripeConfiguration, error)
}
