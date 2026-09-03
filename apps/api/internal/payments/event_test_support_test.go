package payments

import (
	"encoding/json"
	"testing"

	platformevents "ideaboard/api/internal/events"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
)

// NewEventHub is a test-only composition helper for integration tests that
// exercise multiple Payments application services together.
func NewEventHub() *paymentevents.Hub { return paymentevents.NewHub() }

type PaymentEvent = paymentports.PaymentEvent

type paymentEventSubscriber interface {
	Subscribe(int64) (<-chan platformevents.UserEvent, func(), bool)
}

func subscribePaymentUserEvents(t *testing.T, hub paymentEventSubscriber, userID int64) (<-chan platformevents.UserEvent, func()) {
	t.Helper()
	if hub == nil {
		t.Fatal("payment user event hub is unavailable")
	}
	stream, unsubscribe, accepted := hub.Subscribe(userID)
	if !accepted || stream == nil {
		t.Fatal("payment user event subscription rejected")
	}
	return stream, unsubscribe
}

func decodePaymentUserEvent(t *testing.T, event platformevents.UserEvent) PaymentEvent {
	t.Helper()
	if event.Type != platformevents.EventTypePaymentUpdated {
		t.Fatalf("event type=%q, want %q", event.Type, platformevents.EventTypePaymentUpdated)
	}
	var paymentEvent PaymentEvent
	if err := json.Unmarshal(event.Data, &paymentEvent); err != nil {
		t.Fatalf("decode payment user event: %v", err)
	}
	return paymentEvent
}
