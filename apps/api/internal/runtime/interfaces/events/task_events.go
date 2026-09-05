package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	taskservice "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
)

// TaskResolver resolves a task domain entity by task ID.
type TaskResolver interface {
	GetByTaskID(context.Context, string) (*tasks.Task, error)
}

// TaskEventHubPublisher broadcasts normalized task events to local task and user hubs.
type TaskEventHubPublisher struct {
	Hub      *taskservice.EventHub
	UserHub  *events.UserEventHub
	Resolver TaskResolver
}

// PublishTaskStatus broadcasts task status change to internal subscriber hubs.
func (p TaskEventHubPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	if p.Hub == nil {
		return nil
	}
	event = events.NormalizeTaskStatusEvent(event)
	taskEvent := taskservice.TaskEvent{TaskID: event.TaskID, Status: event.Status}
	if p.Resolver != nil {
		if task, err := p.Resolver.GetByTaskID(ctx, event.TaskID); err == nil && task != nil {
			p.Hub.Publish(event.TaskID, taskEvent)
			if p.UserHub != nil {
				userEvent, eventErr := events.NewUserEvent(task.UserID, events.EventTypeTaskUpdated, taskEvent)
				if eventErr != nil {
					return eventErr
				}
				userEvent.OccurredAt = event.OccurredAt
				p.UserHub.PublishRemote(userEvent)
			}
			return nil
		}
	}
	p.Hub.Publish(event.TaskID, taskEvent)
	return nil
}

// CompositeTaskStatusPublisher broadcasts events across both local and remote event publishers.
type CompositeTaskStatusPublisher struct {
	Local  events.TaskStatusPublisher
	Remote events.TaskStatusPublisher
}

// PublishTaskStatus sends event to both local and remote publishers sequentially.
func (p CompositeTaskStatusPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	event = events.NormalizeTaskStatusEvent(event)
	var firstErr error
	if p.Local != nil {
		if err := p.Local.PublishTaskStatus(ctx, event); err != nil {
			firstErr = err
		}
	}
	if p.Remote != nil {
		if err := p.Remote.PublishTaskStatus(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NewTaskStatusEventPublisher configures local and optional remote NATS task status publishing.
func NewTaskStatusEventPublisher(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
	resolver TaskResolver,
	userHubs ...*events.UserEventHub,
) (events.TaskStatusPublisher, *events.NATSJetStreamEventBus, error) {
	var userHub *events.UserEventHub
	if len(userHubs) > 0 {
		userHub = userHubs[0]
	}
	localPublisher := TaskEventHubPublisher{Hub: hub, UserHub: userHub, Resolver: resolver}
	if !cfg.Enabled {
		return localPublisher, nil, nil
	}
	if cfg.NATSURL == "" {
		return nil, nil, fmt.Errorf("NATS_EVENT_BUS_ENABLED=true requires NATS_URL")
	}

	bus, err := events.NewNATSJetStreamEventBus(ctx, events.NATSJetStreamConfig{
		URL:            cfg.NATSURL,
		StreamName:     cfg.NATSStreamName,
		SubjectPrefix:  cfg.NATSSubjectPrefix,
		ConsumerName:   cfg.NATSConsumerName,
		ConnectTimeout: time.Duration(cfg.NATSConnectTimeout) * time.Second,
		RequestTimeout: time.Duration(cfg.NATSRequestTimeout) * time.Second,
		DrainTimeout:   time.Duration(cfg.NATSDrainTimeout) * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}
	remotePublisher := TaskEventHubPublisher{Hub: hub, UserHub: userHub, Resolver: resolver}
	if err := bus.SubscribeTaskStatus(ctx, func(ctx context.Context, event events.TaskStatusEvent) error {
		return remotePublisher.PublishTaskStatus(ctx, event)
	}); err != nil {
		_ = bus.Close()
		return nil, nil, err
	}
	slog.Info("NATS JetStream task events enabled")
	return CompositeTaskStatusPublisher{Local: localPublisher, Remote: bus}, bus, nil
}
