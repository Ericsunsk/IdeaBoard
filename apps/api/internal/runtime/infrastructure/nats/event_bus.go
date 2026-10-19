package nats

import (
	"context"
	"fmt"

	eventnats "ideaboard/api/internal/events/nats"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

var _ ports.ResourceCloser = (*EventBus)(nil)
var _ ports.HealthChecker = (*EventBus)(nil)

// EventBus wraps NATS JetStream event bus for the runtime platform.
type EventBus struct {
	bus *eventnats.NATSJetStreamEventBus
}

// NewEventBus connects to NATS JetStream using the provided configuration.
func NewEventBus(ctx context.Context, cfg config.EventBusConfig) (*EventBus, error) {
	if cfg.NATSURL == "" {
		return nil, nil
	}
	natsBus, err := eventnats.NewNATSJetStreamEventBus(ctx, eventnats.NATSJetStreamConfig{
		URL: cfg.NATSURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NATS JetStream event bus: %w", err)
	}
	return &EventBus{bus: natsBus}, nil
}

// NewEventBusFromUnderlying wraps an existing NATSJetStreamEventBus into an EventBus adapter.
func NewEventBusFromUnderlying(bus *eventnats.NATSJetStreamEventBus) *EventBus {
	return &EventBus{bus: bus}
}

// UnderlyingBus returns the wrapped NATSJetStreamEventBus.
func (b *EventBus) UnderlyingBus() *eventnats.NATSJetStreamEventBus {
	if b == nil {
		return nil
	}
	return b.bus
}

// CheckHealth evaluates the health of the NATS JetStream connection.
func (b *EventBus) CheckHealth(ctx context.Context) domain.ComponentStatus {
	if b == nil || b.bus == nil {
		return domain.ComponentStatus{
			Name:     "nats",
			Kind:     domain.ComponentKindNATS,
			Ready:    true,
			Required: false,
		}
	}
	ready := b.bus.IsConnected()
	detail := ""
	if !ready {
		detail = "nats connection is disconnected"
	}
	return domain.ComponentStatus{
		Name:     "nats",
		Kind:     domain.ComponentKindNATS,
		Ready:    ready,
		Required: true,
		Detail:   detail,
	}
}

// Close gracefully drains and shuts down the NATS JetStream connection.
func (b *EventBus) Close() error {
	if b == nil || b.bus == nil {
		return nil
	}
	return b.bus.Close()
}
