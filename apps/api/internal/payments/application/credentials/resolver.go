// Package credentials is the single boundary for Stripe credential material
// access in Payments.
//
// Every Stripe API-key / webhook-secret lookup and every plaintext recovery
// from ciphertext goes through this package. Callers must not import
// platform/secrets for reads and must not call the configuration store's
// credential getters directly for request-time material: use Resolver.
//
// Error normalization stays explicit per method so existing behavior is
// preserved exactly:
//   - ForCredentialID / ForOrder map any store or decrypt failure to
//     ErrStripeConfigurationInvalid (the historical workflows helper mapping
//     shared by webhook dispatch, dispute expiry, and reconciliation).
//   - ForOrderNumber maps store failures to ErrRefundOrderNotEligible (the
//     historical refund intake mapping); decrypt failures stay
//     ErrStripeConfigurationInvalid.
//   - DecryptAPIKey / DecryptWebhookSecret return the raw decrypt error so
//     callers with their own taxonomy (checkout availability, configuration
//     change detection) keep their exact mapping. They still funnel the
//     secrets seam through one place.
//
// The configuration service remains the only writer of credential material
// (encryption on publish); this package only reads.
package credentials

import (
	"context"
	"fmt"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/secrets"
)

// Store is the minimal credential persistence seam required by Resolver.
// It is satisfied by the payments persistence contracts without pulling the
// full repository surface into application services.
type Store interface {
	GetStripeCredential(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredentialForOrder(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredentialForOrderNumber(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	ListStripeCredentials(context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error)
}

// Resolver loads immutable Stripe credential generations and recovers their
// plaintext API material. Methods take context.Context first.
type Resolver struct {
	store Store
}

// WebhookCredential is a stored credential paired with its recovered
// webhook secret. Callers use this value for signature probing without
// touching the persistence or secrets seams directly.
type WebhookCredential struct {
	Stored        paymentconfiguration.StoredStripeConfiguration
	WebhookSecret string
}

// NewResolver builds the single credential boundary. It rejects a nil store
// so composition fails fast instead of panicking at request time.
func NewResolver(store Store) (*Resolver, error) {
	if store == nil {
		return nil, fmt.Errorf("credential resolver requires a store")
	}
	return &Resolver{store: store}, nil
}

// DecryptAPIKey recovers the Stripe API key from stored material. The raw
// decrypt error is returned unwrapped so callers keep their own taxonomy.
func DecryptAPIKey(ciphertext, materialKind string) (string, error) {
	return secrets.DecryptMaterialString(ciphertext, materialKind)
}

// DecryptWebhookSecret recovers the Stripe webhook secret from stored
// material. The raw decrypt error is returned unwrapped so callers keep
// their own taxonomy.
func DecryptWebhookSecret(ciphertext, materialKind string) (string, error) {
	return secrets.DecryptMaterialString(ciphertext, materialKind)
}

// ForCredentialID resolves the API key for one credential generation.
// Any failure normalizes to ErrStripeConfigurationInvalid.
func (r *Resolver) ForCredentialID(ctx context.Context, credentialID string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
	if r == nil || r.store == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	stored, err := r.store.GetStripeCredential(ctx, credentialID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	key, err := DecryptAPIKey(stored.APIKeyCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	return stored, key, nil
}

// ForOrder resolves the API key through the immutable generation referenced
// by one payment order. Any failure normalizes to
// ErrStripeConfigurationInvalid.
func (r *Resolver) ForOrder(ctx context.Context, orderID string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
	if r == nil || r.store == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	stored, err := r.store.GetStripeCredentialForOrder(ctx, orderID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	key, err := DecryptAPIKey(stored.APIKeyCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	return stored, key, nil
}

// ForOrderNumber resolves the API key through the order referenced by its
// public number. Store failures normalize to ErrRefundOrderNotEligible to
// preserve the refund intake taxonomy; decrypt failures stay
// ErrStripeConfigurationInvalid.
func (r *Resolver) ForOrderNumber(ctx context.Context, orderNumber string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
	if r == nil || r.store == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrRefundOrderNotEligible
	}
	stored, err := r.store.GetStripeCredentialForOrderNumber(ctx, orderNumber)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrRefundOrderNotEligible
	}
	key, err := DecryptAPIKey(stored.APIKeyCiphertext, stored.SecretMaterialKind)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	return stored, key, nil
}

// ListCredentials exposes the stored generations for webhook signature
// probing. Plaintext recovery still goes through DecryptWebhookSecret per
// candidate so no caller touches secrets directly.
func (r *Resolver) ListCredentials(ctx context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error) {
	if r == nil || r.store == nil {
		return nil, paymentcontracts.ErrStripeConfigurationInvalid
	}
	return r.store.ListStripeCredentials(ctx)
}

// ListWebhookCredentials resolves every retained generation and recovers its
// webhook secret through this boundary. Any corrupt generation fails closed.
func (r *Resolver) ListWebhookCredentials(ctx context.Context) ([]WebhookCredential, error) {
	stored, err := r.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]WebhookCredential, 0, len(stored))
	for _, credential := range stored {
		secret, decryptErr := DecryptWebhookSecret(credential.WebhookSecretCiphertext, credential.SecretMaterialKind)
		if decryptErr != nil {
			return nil, paymentcontracts.ErrStripeConfigurationInvalid
		}
		resolved = append(resolved, WebhookCredential{Stored: credential, WebhookSecret: secret})
	}
	return resolved, nil
}
