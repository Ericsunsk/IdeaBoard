package configuration

import (
	"encoding/json"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"
)

// PaymentCheckoutShutdown identifies the immutable cutoff used to expire
// sessions created before a configuration generation was disabled.
type PaymentCheckoutShutdown struct {
	ID                 string
	StripeCredentialID string
	StripeAccountID    string
	CutoffAt           time.Time
	CreatedAt          time.Time
	CompletedAt        *time.Time
}

// StoredStripeConfiguration is the immutable provider credential generation
// and its published checkout settings. Application workflows consume this
// snapshot without depending on PostgreSQL row types.
type StoredStripeConfiguration struct {
	Revision                     int64
	CredentialID                 string
	AccountID                    string
	Mode                         paymentports.StripeMode
	ChargesEnabled               bool
	PaymentMethodConfigurationID string
	ProductTaxCode               string
	PurchasesEnabled             bool
	StripeProductID              string
	StripeWebhookEndpointID      string
	APIKeyCiphertext             string
	WebhookSecretCiphertext      string
	SecretMaterialKind           string
	CreatedBy                    int64
	UpdatedBy                    int64
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

// Audit records the result of a configuration publication attempt.
type Audit struct {
	ActorID      int64
	Outcome      string
	ErrorCode    string
	ErrorSummary string
	Metadata     json.RawMessage
}

// Configuration publication outcomes. The wire words are unchanged
// ("succeeded","failed"); the constants only kill the raw literals at
// the three Audit construction sites.
const (
	AuditOutcomeSucceeded = "succeeded"
	AuditOutcomeFailed    = "failed"
)
