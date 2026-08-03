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

func TestVerifyExtractsRefundAndDisputeOrderReferences(t *testing.T) {
	adapter := New()
	secret := "whsec_fixture"
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name            string
		eventType       string
		object          string
		paymentIntentID string
		chargeID        string
	}{
		{name: "refund string references", eventType: "refund.updated", object: `{"id":"re_fixture","payment_intent":"pi_fixture","charge":"ch_fixture"}`, paymentIntentID: "pi_fixture", chargeID: "ch_fixture"},
		{name: "dispute expanded references", eventType: "charge.dispute.created", object: `{"id":"dp_fixture","payment_intent":{"id":"pi_expanded"},"charge":{"id":"ch_expanded"}}`, paymentIntentID: "pi_expanded", chargeID: "ch_expanded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"id":"evt_reference","object":"event","api_version":%q,"created":%d,"livemode":false,"type":%q,"data":{"object":%s}}`, stripe.APIVersion, now.Unix(), test.eventType, test.object))
			event, err := adapter.Verify(t.Context(), payload, signedHeader(now, payload, secret), secret, payments.StripeWebhookTolerance, now)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if event.PaymentIntentID != test.paymentIntentID || event.ChargeID != test.chargeID {
				t.Fatalf("event references = %+v", event)
			}
		})
	}
}

func signedHeader(timestamp time.Time, payload []byte, secret string) string {
	signature := webhook.ComputeSignature(timestamp, payload, secret)
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(signature))
}
