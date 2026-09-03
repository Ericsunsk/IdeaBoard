package task

import (
	"testing"
	"time"
)

func subscriberCountForTest(h *EventHub, taskID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[taskID])
}

func TestEventHubSubscribeAndPublish(t *testing.T) {
	hub := NewEventHub()
	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsub()
	hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "running"})
	select {
	case received := <-ch:
		if received.Status != "running" {
			t.Errorf("expected status 'running', got '%s'", received.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventHubUnsubscribe(t *testing.T) {
	hub := NewEventHub()
	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	unsub()
	hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "completed"})
	select {
	case event := <-ch:
		t.Fatalf("unexpected event after unsubscribe: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
	if subscriberCountForTest(hub, "task-1") != 0 {
		t.Errorf("expected 0 subscribers, got %d", subscriberCountForTest(hub, "task-1"))
	}
}

func TestEventHubPublishNoSubscribers(t *testing.T) {
	NewEventHub().Publish("nonexistent", TaskEvent{TaskID: "x", Status: "running"})
}

func TestEventHubMultipleSubscribers(t *testing.T) {
	hub := NewEventHub()
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
	hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "completed"})
	received := 0
	timeout := time.After(time.Second)
	for received < 2 {
		select {
		case <-ch1:
			received++
		case <-ch2:
			received++
		case <-timeout:
			t.Fatalf("timeout: only received %d/2 events", received)
		}
	}
}

func TestEventHubNonBlockingPublish(t *testing.T) {
	hub := NewEventHub()
	ch, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsub()
	for i := 0; i < 10; i++ {
		hub.Publish("task-1", TaskEvent{TaskID: "task-1", Status: "running"})
	}
	drained := 0
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case <-ch:
			drained++
		case <-timeout:
			if drained == 0 {
				t.Error("expected at least some events to be received")
			}
			return
		}
	}
}

func TestEventHubSlowConsumerKeepsNewestEvent(t *testing.T) {
	hub := NewEventHub()
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

func TestEventHubRejectsWhenPerUserLimitReached(t *testing.T) {
	hub := newEventHubWithLimits(10, 1)
	_, unsub, ok := hub.Subscribe("task-1", 9)
	if !ok {
		t.Fatal("first subscribe rejected unexpectedly")
	}
	defer unsub()
	if _, _, ok := hub.Subscribe("task-2", 9); ok {
		t.Fatal("expected second subscribe to be rejected by per-user limit")
	}
}

func TestEventHubRejectsWhenGlobalLimitReached(t *testing.T) {
	hub := newEventHubWithLimits(1, 10)
	_, unsub, ok := hub.Subscribe("task-1", 1)
	if !ok {
		t.Fatal("first subscribe rejected unexpectedly")
	}
	defer unsub()
	if _, _, ok := hub.Subscribe("task-2", 2); ok {
		t.Fatal("expected subscribe to be rejected by global limit")
	}
}
