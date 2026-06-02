package events

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeTaskStatusEvent(t *testing.T) {
	event := NormalizeTaskStatusEvent(TaskStatusEvent{
		TaskID: " task-1 ",
		Status: " running ",
	})

	if event.Type != EventTypeTaskStatusChanged {
		t.Fatalf("Type=%q, want %q", event.Type, EventTypeTaskStatusChanged)
	}
	if event.TaskID != "task-1" {
		t.Fatalf("TaskID=%q, want task-1", event.TaskID)
	}
	if event.Status != "running" {
		t.Fatalf("Status=%q, want running", event.Status)
	}
	if event.OccurredAt.IsZero() {
		t.Fatal("OccurredAt should be set")
	}
	if event.OccurredAt.Location() != time.UTC {
		t.Fatalf("OccurredAt location=%v, want UTC", event.OccurredAt.Location())
	}
}

func TestPublishTaskStatusBestEffortAcceptsNilContext(t *testing.T) {
	publisher := &recordingPublisher{}
	if err := PublishTaskStatusBestEffort(nil, publisher, NewTaskStatusEvent("task-1", "pending")); err != nil {
		t.Fatalf("PublishTaskStatusBestEffort failed: %v", err)
	}
	if publisher.event.TaskID != "task-1" || publisher.event.Status != "pending" {
		t.Fatalf("event=(%s,%s), want=(task-1,pending)", publisher.event.TaskID, publisher.event.Status)
	}
}

type recordingPublisher struct {
	event TaskStatusEvent
}

func (p *recordingPublisher) PublishTaskStatus(_ context.Context, event TaskStatusEvent) error {
	p.event = event
	return nil
}
