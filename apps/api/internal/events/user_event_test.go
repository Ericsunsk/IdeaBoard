package events

import (
	"testing"
	"time"
)

func TestUserEventHubPublishesAndCleansUp(t *testing.T) {
	hub := NewUserEventHubWithLimits(2, 1)
	stream, unsubscribe, accepted := hub.Subscribe(42)
	if !accepted || stream == nil {
		t.Fatal("expected user event subscription")
	}
	if _, _, accepted := hub.Subscribe(42); accepted {
		t.Fatal("per-user connection limit should reject second subscription")
	}
	event := UserEvent{UserID: 42, Type: EventTypeTaskUpdated, Data: []byte(`{"task_id":"task-1"}`), OccurredAt: time.Now().UTC()}
	hub.Publish(event)
	select {
	case got := <-stream:
		if got.Type != event.Type || string(got.Data) != string(event.Data) {
			t.Fatalf("event=%+v, want %+v", got, event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user event")
	}
	unsubscribe()
	if _, _, accepted := hub.Subscribe(42); !accepted {
		t.Fatal("subscription should be accepted after cleanup")
	}
}

func TestUserEventHubDropsOldestAndDoesNotForwardRemote(t *testing.T) {
	hub := NewUserEventHubWithLimits(1, 1)
	stream, unsubscribe, accepted := hub.Subscribe(7)
	if !accepted {
		t.Fatal("expected subscription")
	}
	defer unsubscribe()
	var forwarded int
	hub.SetRemotePublisher(func(UserEvent) { forwarded++ })
	for index := 0; index < 20; index++ {
		hub.Publish(UserEvent{UserID: 7, Type: EventTypePaymentUpdated, Data: []byte{byte(index)}})
	}
	if forwarded != 20 {
		t.Fatalf("forwarded=%d, want 20", forwarded)
	}
	hub.PublishRemote(UserEvent{UserID: 7, Type: EventTypeMediaDerivativeUpdated, Data: []byte(`{}`)})
	if forwarded != 20 {
		t.Fatalf("remote event was forwarded, forwarded=%d", forwarded)
	}
	if len(stream) != 16 {
		t.Fatalf("buffer length=%d, want 16", len(stream))
	}
}
