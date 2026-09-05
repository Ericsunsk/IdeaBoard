package composition

import (
	"strings"
	"testing"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/domain"
	natsinfra "ideaboard/api/internal/runtime/infrastructure/nats"
)

func TestNewAppContainerRejectsMissingPublicOriginsBeforeOpeningResources(t *testing.T) {
	_, err := newAppContainer(&config.Config{})
	if err == nil || !strings.Contains(err.Error(), "PUBLIC_ORIGIN is required") {
		t.Fatalf("newAppContainer() error = %v, want missing public origin", err)
	}
}

func TestNATSHealthCheckerWhenDisabled(t *testing.T) {
	checker := natsinfra.NewEventBusFromUnderlying(nil)
	status := checker.CheckHealth(t.Context())

	if status.Name != "nats" {
		t.Fatalf("status.Name = %q, want %q", status.Name, "nats")
	}
	if status.Kind != domain.ComponentKindNATS {
		t.Fatalf("status.Kind = %q, want %q", status.Kind, domain.ComponentKindNATS)
	}
	if !status.Ready {
		t.Fatal("disabled nats checker should report Ready = true")
	}
	if status.Required {
		t.Fatal("disabled nats checker should report Required = false")
	}

	if err := domain.EvaluateReadiness([]domain.ComponentStatus{status}); err != nil {
		t.Fatalf("EvaluateReadiness failed for disabled nats: %v", err)
	}
}

func TestNATSHealthCheckerWhenEnabledDisconnected(t *testing.T) {
	// Constructing an EventBus with an uninitialized bus pointer (no active connection)
	checker := natsinfra.NewEventBusFromUnderlying(&events.NATSJetStreamEventBus{})
	status := checker.CheckHealth(t.Context())

	if status.Name != "nats" {
		t.Fatalf("status.Name = %q, want %q", status.Name, "nats")
	}
	if !status.Required {
		t.Fatal("enabled nats checker should report Required = true")
	}
	if status.Ready {
		t.Fatal("disconnected nats checker should report Ready = false")
	}

	err := domain.EvaluateReadiness([]domain.ComponentStatus{status})
	if err == nil {
		t.Fatal("EvaluateReadiness should fail when required nats is not ready")
	}
	if !strings.Contains(err.Error(), "nats") {
		t.Fatalf("error message %q should contain 'nats'", err.Error())
	}
}
