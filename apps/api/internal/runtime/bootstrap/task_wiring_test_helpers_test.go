package bootstrap

import (
	"context"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	taskservice "ideaboard/api/internal/tasking/application/task"
)

func newTaskStatusEventPublisher(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
) (events.TaskStatusPublisher, *events.NATSJetStreamEventBus, error) {
	return newTaskStatusEventPublisherWithResolver(ctx, cfg, hub, nil)
}
