package accounts

import (
	"testing"
	"time"
)

func TestTaskEventHub_SubscribeAndPublish(t *testing.T) {
	hub := NewTaskEventHub()

	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsub()

	event := TaskEvent{TaskID: "task-1", Status: "running"}
	hub.Publish("task-1", event)

	select {
	case received := <-ch:
		if received.Status != "running" {
			t.Errorf("expected status 'running', got '%s'", received.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestTaskEventHub_Unsubscribe(t *testing.T) {
	hub := NewTaskEventHub()

	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	unsub()

	// Unsubscribe removes subscription and should not receive further events.
	hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "completed"})
	select {
	case event := <-ch:
		t.Fatalf("unexpected event after unsubscribe: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}

	if hub.SubscriberCount("task-1") != 0 {
		t.Errorf("expected 0 subscribers, got %d", hub.SubscriberCount("task-1"))
	}
}

func TestTaskEventHub_PublishNoSubscribers(t *testing.T) {
	hub := NewTaskEventHub()

	// Should not panic
	hub.Publish("nonexistent", TaskEvent{TaskID: "x", Status: "running"})
}

func TestTaskEventHub_MultipleSubscribers(t *testing.T) {
	hub := NewTaskEventHub()

	ch1, unsub1, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("first subscribe rejected unexpectedly")
	}
	defer unsub1()
	ch2, unsub2, ok := hub.Subscribe("task-1", 2)
	if !ok {
		t.Fatal("second subscribe rejected unexpectedly")
	}
	defer unsub2()

	event := TaskEvent{TaskID: "task-1", Status: "completed"}
	hub.Publish("task-1", event)

	received := 0
	timeout := time.After(time.Second)

	for received < 2 {
		select {
		case <-ch1:
			received++
			continue
		case <-ch2:
			received++
			continue
		case <-timeout:
			t.Fatalf("timeout: only received %d/2 events", received)
		}
	}
}

func TestTaskEventHub_NonBlockingPublish(t *testing.T) {
	hub := NewTaskEventHub()

	// Create a subscriber with a full channel (buffer size 8), then fill it
	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsub()

	// Fill the buffer
	for i := 0; i < 10; i++ {
		event := TaskEvent{TaskID: "task-1", Status: "running"}
		hub.Publish("task-1", event)
	}

	// Drain to avoid goroutine leak
	drained := 0
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case <-ch:
			drained++
		case <-timeout:
			goto done
		}
	}
done:
	if drained == 0 {
		t.Error("expected at least some events to be received")
	}
}

func TestTaskEventHub_SlowConsumerKeepsNewestEvent(t *testing.T) {
	hub := NewTaskEventHub()
	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsub()

	for i := 0; i < 20; i++ {
		hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "running"})
	}
	hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "completed"})

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-ch:
			if event.Status == "completed" {
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for newest completed event")
		}
	}
}

func TestTaskEventHub_RejectsWhenPerUserLimitReached(t *testing.T) {
	hub := NewTaskEventHubWithLimits(10, 1)
	_, unsub, ok := hub.Subscribe("task-1", 9)
	if !ok {
		t.Fatal("first subscribe rejected unexpectedly")
	}
	defer unsub()

	if _, _, ok := hub.Subscribe("task-2", 9); ok {
		t.Fatal("expected second subscribe to be rejected by per-user limit")
	}
}

func TestTaskEventHub_RejectsWhenGlobalLimitReached(t *testing.T) {
	hub := NewTaskEventHubWithLimits(1, 10)
	_, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("first subscribe rejected unexpectedly")
	}
	defer unsub()

	if _, _, ok := hub.Subscribe("task-2", 2); ok {
		t.Fatal("expected subscribe to be rejected by global limit")
	}
}
