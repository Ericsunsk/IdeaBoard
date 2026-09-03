package payments

import (
	"encoding/json"
	"testing"
	"time"

	platformevents "ideaboard/api/internal/events"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
)

func NewEventHub() *paymentevents.Hub { return paymentevents.NewHub() }

type PaymentEvent = paymentports.PaymentEvent

func TestPaymentEventHubScopesEventsByUserAndKeepsLatest(t *testing.T) {
	hub := NewEventHub()
	userHub := platformevents.NewUserEventHub()
	hub.SetUserEventHub(userHub)
	first, unsubscribeFirst := subscribePaymentUserEvents(t, userHub, 42)
	defer unsubscribeFirst()
	second, unsubscribeSecond := subscribePaymentUserEvents(t, userHub, 99)
	defer unsubscribeSecond()

	hub.Publish(42, PaymentEvent{PublicToken: "first", Status: "processing"})
	select {
	case rawEvent := <-first:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.PublicToken != "first" {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("user 42 did not receive payment event")
	}
	select {
	case rawEvent := <-second:
		event := decodePaymentUserEvent(t, rawEvent)
		t.Fatalf("user 99 received another user's event: %+v", event)
	default:
	}

	for index := range 8 {
		hub.Publish(42, PaymentEvent{PublicToken: "latest", Status: string(rune('0' + index))})
	}
	deadline := time.After(time.Second)
	for {
		select {
		case rawEvent := <-first:
			latest := decodePaymentUserEvent(t, rawEvent)
			if latest.Status == "7" {
				return
			}
		case <-deadline:
			t.Fatal("latest event was not delivered")
		}
	}
}

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
