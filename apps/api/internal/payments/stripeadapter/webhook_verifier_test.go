package stripeadapter

import (
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

func TestVerifyAcceptsExactRawBodyAndRejectsExpiredOrInvalidSignatures(t *testing.T) {
	adapter := New()
	secret := "whsec_fixture"
	now := time.Now().UTC().Truncate(time.Second)
	payload := []byte(fmt.Sprintf(`{"id":"evt_adapter","object":"event","api_version":%q,"created":%d,"livemode":false,"type":"checkout.session.completed","data":{"object":{"id":"cs_adapter"}}}`, stripe.APIVersion, now.Unix()))
	header := signedHeader(now, payload, secret)

	event, err := adapter.Verify(t.Context(), payload, header, secret, payments.StripeWebhookTolerance, now)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if event.ID != "evt_adapter" || event.Type != "checkout.session.completed" || event.ObjectID != "cs_adapter" || !event.CreatedAt.Equal(now) {
		t.Fatalf("event = %+v", event)
	}

	old := now.Add(-payments.StripeWebhookTolerance - time.Second)
	_, err = adapter.Verify(t.Context(), payload, signedHeader(old, payload, secret), secret, payments.StripeWebhookTolerance, now)
	if !errors.Is(err, payments.ErrWebhookSignatureExpired) {
		t.Fatalf("expired signature error = %v", err)
	}

	_, err = adapter.Verify(t.Context(), payload, header, "whsec_wrong", payments.StripeWebhookTolerance, now)
	if !errors.Is(err, payments.ErrInvalidWebhookSignature) {
		t.Fatalf("invalid signature error = %v", err)
	}
}

func signedHeader(timestamp time.Time, payload []byte, secret string) string {
	signature := webhook.ComputeSignature(timestamp, payload, secret)
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(signature))
}
