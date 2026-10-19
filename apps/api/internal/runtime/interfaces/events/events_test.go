package eventbus_test

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	taskservice "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
)

type mockTaskResolver struct {
	task *tasks.Task
	err  error
}

func (m mockTaskResolver) GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error) {
	return m.task, m.err
}

func TestTaskEventHubPublisher(t *testing.T) {
	hub := taskservice.NewEventHub()
	userHub := events.NewUserEventHub()

	resolver := mockTaskResolver{
		task: &tasks.Task{
			TaskID: "task-123",
			UserID: 42,
		},
	}

	publisher := eventinterfaces.TaskEventHubPublisher{
		Hub:      hub,
		UserHub:  userHub,
		Resolver: resolver,
	}

	userEvents, unsubscribe, ok := userHub.Subscribe(42)
	if !ok {
		t.Fatal("expected user hub subscribe to succeed")
	}
	defer unsubscribe()

	ctx := context.Background()
	event := events.TaskStatusEvent{
		TaskID:     "task-123",
		Status:     tasks.StatusRunning,
		OccurredAt: time.Now().UTC(),
	}

	if err := publisher.PublishTaskStatus(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case received := <-userEvents:
		if received.UserID != 42 {
			t.Fatalf("expected user ID 42, got %d", received.UserID)
		}
		if received.Type != events.EventTypeTaskUpdated {
			t.Fatalf("expected task updated, got %s", received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user event")
	}
}

func TestCompositeTaskStatusPublisher(t *testing.T) {
	hub := taskservice.NewEventHub()
	localPublisher := eventinterfaces.TaskEventHubPublisher{Hub: hub}
	composite := eventinterfaces.CompositeTaskStatusPublisher{
		Local:  localPublisher,
		Remote: nil,
	}

	err := composite.PublishTaskStatus(context.Background(), events.TaskStatusEvent{
		TaskID: "task-abc",
		Status: tasks.StatusCompleted,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewTaskStatusEventPublisherDisabled(t *testing.T) {
	hub := taskservice.NewEventHub()
	pub, bus, err := eventinterfaces.NewTaskStatusEventPublisher(context.Background(), config.EventBusConfig{}, hub, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus != nil {
		t.Fatalf("expected nil bus when disabled")
	}
	if pub == nil {
		t.Fatal("expected non-nil local publisher")
	}
}

func TestConfigureUserEventFanoutRequiresDependencies(t *testing.T) {
	err := eventinterfaces.ConfigureUserEventFanout(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when user hub or payment service is nil")
	}

	userHub := events.NewUserEventHub()
	err = eventinterfaces.ConfigureUserEventFanout(context.Background(), nil, userHub, nil)
	if err == nil {
		t.Fatal("expected error when payment service is nil")
	}
}
