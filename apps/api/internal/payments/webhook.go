package payments

import (
	"context"
	"errors"
	"strings"
	"time"
)

const StripeWebhookTolerance = 5 * time.Minute

var (
	ErrInvalidWebhookSignature = errors.New("Stripe Webhook 签名无效")
	ErrWebhookSignatureExpired = errors.New("Stripe Webhook 签名已过期")
	ErrMalformedWebhookPayload = errors.New("Stripe Webhook 数据无效")
)

var stripeWebhookEventTypes = [...]string{
	"checkout.session.completed",
	"checkout.session.async_payment_succeeded",
	"checkout.session.async_payment_failed",
	"checkout.session.expired",
	"refund.updated",
	"refund.failed",
	"charge.dispute.created",
	"charge.dispute.closed",
}

var recognizedStripeWebhookEvents = func() map[string]struct{} {
	events := make(map[string]struct{}, len(stripeWebhookEventTypes))
	for _, eventType := range stripeWebhookEventTypes {
		events[eventType] = struct{}{}
	}
	return events
}()

// SupportedStripeWebhookEventTypes returns the exact event contract configured on Stripe.
// A copy prevents adapters from mutating the Payments domain's canonical set.
func SupportedStripeWebhookEventTypes() []string {
	return append([]string(nil), stripeWebhookEventTypes[:]...)
}

type StripeWebhookEvent struct {
	ID        string
	Type      string
	ObjectID  string
	Livemode  bool
	CreatedAt time.Time
}

type StripeWebhookReceipt struct {
	EventID   string `json:"event_id"`
	Duplicate bool   `json:"duplicate"`
	Ignored   bool   `json:"ignored"`
}

type StripeWebhookVerifier interface {
	Verify(ctx context.Context, raw []byte, signature, secret string, tolerance time.Duration, now time.Time) (StripeWebhookEvent, error)
}

func validateStripeWebhookEvent(event StripeWebhookEvent) error {
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.CreatedAt.IsZero() {
		return ErrMalformedWebhookPayload
	}
	return nil
}

func isRecognizedStripeWebhookEvent(eventType string) bool {
	_, ok := recognizedStripeWebhookEvents[eventType]
	return ok
}
