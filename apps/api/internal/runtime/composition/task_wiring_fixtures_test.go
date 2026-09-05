package composition

import (
	"context"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	taskservice "ideaboard/api/internal/tasking/application/task"
)

func newTaskStatusEventPublisher(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
) (events.TaskStatusPublisher, *events.NATSJetStreamEventBus, error) {
	return eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg, hub, nil)
}
