package events

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const (
	EventTypeTaskStatusChanged = "task.status_changed"
	DefaultPublishTimeout      = 2 * time.Second
)

// TaskStatus is the workspace task lifecycle word carried by
// TaskStatusEvent. The wire words are unchanged
// ("pending","running","completed","failed","cancelled") and mirror
// workspacetasks.Status; the type only documents the closed vocabulary
// at compile time. The canonical transition table lives in the
// workspacetasks package (see CanTransition there, verified against
// internal/db/queries/workspace_tasks.sql); events only transports the
// words and never gates them, so no second table is kept here.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type TaskStatusEvent struct {
	Type       string     `json:"type"`
	TaskID     string     `json:"task_id"`
	Status     TaskStatus `json:"status"`
	OccurredAt time.Time  `json:"occurred_at"`
}

type TaskStatusPublisher interface {
	PublishTaskStatus(ctx context.Context, event TaskStatusEvent) error
}

type TaskStatusHandler func(ctx context.Context, event TaskStatusEvent) error

func NewTaskStatusEvent(taskID, status string) TaskStatusEvent {
	return TaskStatusEvent{
		Type:       EventTypeTaskStatusChanged,
		TaskID:     strings.TrimSpace(taskID),
		Status:     TaskStatus(strings.TrimSpace(status)),
		OccurredAt: time.Now().UTC(),
	}
}

func NormalizeTaskStatusEvent(event TaskStatusEvent) TaskStatusEvent {
	event.TaskID = strings.TrimSpace(event.TaskID)
	event.Status = TaskStatus(strings.TrimSpace(string(event.Status)))
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

// PublishTaskStatusLogWarn publishes a task status event and logs a warning on failure.
// Use this as a shared helper instead of duplicating the publish+warn pattern.
func PublishTaskStatusLogWarn(ctx context.Context, publisher TaskStatusPublisher, taskID, status string) {
	if err := PublishTaskStatusBestEffort(ctx, publisher, NewTaskStatusEvent(taskID, status)); err != nil {
		slog.Warn("publish task status event failed", "task_id", taskID, "status", status, "err", err)
	}
}
