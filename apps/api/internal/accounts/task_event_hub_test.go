package accounts

import (
	"testing"
	"time"

	"api/internal/tasks"
)

func TestTaskEventHub_SubscribeAndPublish(t *testing.T) {
	hub := NewTaskEventHub()

	ch, unsub := hub.Subscribe("task-1")
	defer unsub()

	event := TaskEvent{TaskID: "task-1", Status: "running", Task: &tasks.Task{TaskID: "task-1", Status: "running"}}
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

	ch, unsub := hub.Subscribe("task-1")
	unsub()

	// Verify channel is closed after unsubscribe
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after unsubscribe but it isn't")
			// drain
			for range ch {
			}
		}
	case <-time.After(100 * time.Millisecond):
		// Channel might still be open but subscriber removed
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

	ch1, unsub1 := hub.Subscribe("task-1")
	defer unsub1()
	ch2, unsub2 := hub.Subscribe("task-1")
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
	ch, unsub := hub.Subscribe("task-1")
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
	ch, unsub := hub.Subscribe("task-1")
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
