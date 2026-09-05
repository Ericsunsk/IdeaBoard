package ports

import (
	"context"
)

// EventPublisher publishes runtime and business domain events to the platform event stream.
type EventPublisher interface {
	PublishEvent(ctx context.Context, topic string, payload []byte) error
}

// EventHandler processes an incoming event envelope.
type EventHandler func(ctx context.Context, topic string, payload []byte) error

// EventSubscriber handles subscription and lifecycle for runtime event listeners.
type EventSubscriber interface {
	Subscribe(ctx context.Context, topic string, handler EventHandler) error
	Close() error
}
