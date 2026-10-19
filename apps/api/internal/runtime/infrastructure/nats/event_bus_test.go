package nats_test

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/infrastructure/nats"
)

func TestDisabledEventBusReturnsNil(t *testing.T) {
	bus, err := nats.NewEventBus(context.Background(), config.EventBusConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus != nil {
		t.Fatalf("expected nil bus when disabled, got %v", bus)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("nil bus close error: %v", err)
	}
	health := bus.CheckHealth(context.Background())
	if !health.Ready {
		t.Fatalf("expected ready for nil bus, got %+v", health)
	}
}
