package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86/webhook"
)

func (a *Adapter) Verify(_ context.Context, raw []byte, signature, secret string, tolerance time.Duration, _ time.Time) (paymentports.StripeWebhookEvent, error) {
	event, err := webhook.ConstructEventWithOptions(raw, signature, secret, webhook.ConstructEventOptions{
		Tolerance: tolerance,
	})
	if err != nil {
		switch {
		case errors.Is(err, webhook.ErrTooOld):
			return paymentports.StripeWebhookEvent{}, paymentports.ErrWebhookSignatureExpired
		case errors.Is(err, webhook.ErrInvalidHeader), errors.Is(err, webhook.ErrNoValidSignature), errors.Is(err, webhook.ErrNotSigned):
			return paymentports.StripeWebhookEvent{}, paymentports.ErrInvalidWebhookSignature
		default:
			return paymentports.StripeWebhookEvent{}, paymentports.ErrMalformedWebhookPayload
		}
	}
	var object struct {
		ID            string          `json:"id"`
		PaymentIntent json.RawMessage `json:"payment_intent"`
		Charge        json.RawMessage `json:"charge"`
	}
	if len(event.Data.Raw) > 0 {
		if err := json.Unmarshal(event.Data.Raw, &object); err != nil {
			return paymentports.StripeWebhookEvent{}, paymentports.ErrMalformedWebhookPayload
		}
	}
	paymentIntentID, err := stripeObjectReferenceID(object.PaymentIntent)
	if err != nil {
		return paymentports.StripeWebhookEvent{}, paymentports.ErrMalformedWebhookPayload
	}
	chargeID, err := stripeObjectReferenceID(object.Charge)
	if err != nil {
		return paymentports.StripeWebhookEvent{}, paymentports.ErrMalformedWebhookPayload
	}
	return paymentports.StripeWebhookEvent{
		ID: event.ID, Type: string(event.Type), ObjectID: object.ID,
		PaymentIntentID: paymentIntentID, ChargeID: chargeID,
		Livemode: event.Livemode, CreatedAt: time.Unix(event.Created, 0).UTC(),
	}, nil
}

func stripeObjectReferenceID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var id string
	if err := json.Unmarshal(raw, &id); err == nil {
		id = strings.TrimSpace(id)
		if id == "" {
			return "", errors.New("empty Stripe object reference")
		}
		return id, nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", fmt.Errorf("decode Stripe object reference: %w", err)
	}
	object.ID = strings.TrimSpace(object.ID)
	if object.ID == "" {
		return "", errors.New("missing Stripe object reference ID")
	}
	return object.ID, nil
}
