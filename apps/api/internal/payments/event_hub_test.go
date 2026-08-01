package payments

import "testing"

func TestPaymentEventHubScopesEventsByUserAndKeepsLatest(t *testing.T) {
	hub := NewEventHub()
	first, unsubscribeFirst := hub.Subscribe(42)
	defer unsubscribeFirst()
	second, unsubscribeSecond := hub.Subscribe(99)
	defer unsubscribeSecond()

	hub.Publish(42, PaymentEvent{PublicToken: "first", Status: "processing"})
	select {
	case event := <-first:
		if event.PublicToken != "first" {
			t.Fatalf("event = %+v", event)
		}
	default:
		t.Fatal("user 42 did not receive payment event")
	}
	select {
	case event := <-second:
		t.Fatalf("user 99 received another user's event: %+v", event)
	default:
	}

	for index := range 8 {
		hub.Publish(42, PaymentEvent{PublicToken: "latest", Status: string(rune('0' + index))})
	}
	var latest PaymentEvent
	for len(first) > 0 {
		latest = <-first
	}
	if latest.Status != "7" {
		t.Fatalf("latest event = %+v", latest)
	}
}
