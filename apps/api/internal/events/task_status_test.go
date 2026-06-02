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

func TestPublishTaskStatusBestEffortPublishesNormalizedEvent(t *testing.T) {
	publisher := &recordingPublisher{}
	if err := PublishTaskStatusBestEffort(context.TODO(), publisher, TaskStatusEvent{
		TaskID: " task-1 ",
		Status: " pending ",
	}); err != nil {
		t.Fatalf("PublishTaskStatusBestEffort failed: %v", err)
	}
	if publisher.event.Type != EventTypeTaskStatusChanged || publisher.event.TaskID != "task-1" || publisher.event.Status != "pending" {
		t.Fatalf("event=(%s,%s,%s), want=(%s,task-1,pending)", publisher.event.Type, publisher.event.TaskID, publisher.event.Status, EventTypeTaskStatusChanged)
	}
}

type recordingPublisher struct {
	event TaskStatusEvent
}

func (p *recordingPublisher) PublishTaskStatus(_ context.Context, event TaskStatusEvent) error {
	p.event = event
	return nil
}
