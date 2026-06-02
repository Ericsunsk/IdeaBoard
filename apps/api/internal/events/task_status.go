package events

import (
	"context"
	"strings"
	"time"
)

const (
	EventTypeTaskStatusChanged = "task.status_changed"
	DefaultPublishTimeout      = 2 * time.Second
)

type TaskStatusEvent struct {
	Type       string    `json:"type"`
	TaskID     string    `json:"task_id"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TaskStatusPublisher interface {
	PublishTaskStatus(ctx context.Context, event TaskStatusEvent) error
}

type TaskStatusHandler func(ctx context.Context, event TaskStatusEvent) error

func NewTaskStatusEvent(taskID, status string) TaskStatusEvent {
	return TaskStatusEvent{
		Type:       EventTypeTaskStatusChanged,
		TaskID:     strings.TrimSpace(taskID),
		Status:     strings.TrimSpace(status),
		OccurredAt: time.Now().UTC(),
	}
}

func NormalizeTaskStatusEvent(event TaskStatusEvent) TaskStatusEvent {
	event.TaskID = strings.TrimSpace(event.TaskID)
	event.Status = strings.TrimSpace(event.Status)
	if strings.TrimSpace(event.Type) == "" {
		event.Type = EventTypeTaskStatusChanged
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	return event
}

func PublishTaskStatusBestEffort(parent context.Context, publisher TaskStatusPublisher, event TaskStatusEvent) error {
	if publisher == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), DefaultPublishTimeout)
	defer cancel()
	return publisher.PublishTaskStatus(ctx, NormalizeTaskStatusEvent(event))
}
