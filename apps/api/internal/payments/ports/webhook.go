package ports

import (
	"context"
	"errors"
	"strings"
	"time"
)

const StripeWebhookTolerance = 5 * time.Minute

var (
	ErrInvalidWebhookSignature = errors.New("无效的 Stripe Webhook 签名")
	ErrWebhookSignatureExpired = errors.New("检测到 Stripe Webhook 签名已过期")
	ErrMalformedWebhookPayload = errors.New("无效的 Stripe Webhook 数据")
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

func SupportedStripeWebhookEventTypes() []string {
	return append([]string(nil), stripeWebhookEventTypes[:]...)
}

type StripeWebhookEvent struct {
	ID              string
	Type            string
	ObjectID        string
	PaymentIntentID string
	ChargeID        string
	Livemode        bool
	CreatedAt       time.Time
}

type StripeWebhookReceipt struct {
	EventID   string `json:"event_id"`
	Duplicate bool   `json:"duplicate"`
	Ignored   bool   `json:"ignored"`
}

type StripeWebhookVerifier interface {
	Verify(context.Context, []byte, string, string, time.Duration, time.Time) (StripeWebhookEvent, error)
}

func ValidateStripeWebhookEvent(event StripeWebhookEvent) error {
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.CreatedAt.IsZero() {
		return ErrMalformedWebhookPayload
	}
	return nil
}

func IsRecognizedStripeWebhookEvent(eventType string) bool {
	_, ok := recognizedStripeWebhookEvents[eventType]
	return ok
}
