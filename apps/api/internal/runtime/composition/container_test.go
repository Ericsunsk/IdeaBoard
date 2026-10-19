package composition

import (
	"context"
	"strings"
	"testing"

	eventnats "ideaboard/api/internal/events/nats"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/composition/wiring"
	"ideaboard/api/internal/runtime/domain"
	natsinfra "ideaboard/api/internal/runtime/infrastructure/nats"
	"ideaboard/api/internal/runtime/ports"
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
	checker := natsinfra.NewEventBusFromUnderlying(&eventnats.NATSJetStreamEventBus{})
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

type stubRunner struct {
	name string
}

func (s stubRunner) Name() string                { return s.name }
func (s stubRunner) Kind() domain.ComponentKind  { return domain.ComponentKindHTTP }
func (s stubRunner) Start(context.Context) error { return nil }
func (s stubRunner) Stop(context.Context) error  { return nil }

var _ ports.Runner = stubRunner{}

// TestResolveServeModePrefersFlagOverEnv pins the ADR-0021 override chain:
// an empty flag keeps the loaded API_MODE value, an explicit flag replaces it.
func TestResolveServeModePrefersFlagOverEnv(t *testing.T) {
	cfg := &config.Config{Mode: config.ProcessModeHTTPOnly}
	resolveServeMode("")(cfg)
	if cfg.Mode != config.ProcessModeHTTPOnly {
		t.Fatalf("empty flag kept mode = %q, want http-only", cfg.Mode)
	}
	resolveServeMode("worker-only")(cfg)
	if cfg.Mode != config.ProcessModeWorkerOnly {
		t.Fatalf("explicit flag mode = %q, want worker-only", cfg.Mode)
	}
	resolveServeMode("")(nil)
}

// TestProcessModeRunnerTopology pins the ADR-0021 split: http-only starts
// listeners without queue consumption (asserted via consume wiring), and
// worker-only starts no HTTP listeners at all.
func TestProcessModeRunnerTopology(t *testing.T) {
	newApp := func(mode config.ProcessMode) *App {
		app := &App{workerRuntime: wiring.AppWorkerRuntime{}}
		if mode.ServesHTTP() {
			app.httpServer = stubRunner{name: "HTTP"}
			app.metricsServer = stubRunner{name: "metrics"}
		}
		return app
	}

	runnerNames := func(app *App) []string {
		var names []string
		for _, r := range app.startupRunners() {
			names = append(names, r.Name())
		}
		return names
	}

	if got := runnerNames(newApp(config.ProcessModeAll)); len(got) != 3 {
		t.Fatalf("all-mode runners = %v, want [river-worker HTTP metrics]", got)
	}
	if got := runnerNames(newApp(config.ProcessModeHTTPOnly)); len(got) != 3 {
		t.Fatalf("http-only runners = %v, want [river-worker HTTP metrics]", got)
	}
	workerOnly := newApp(config.ProcessModeWorkerOnly)
	if got := runnerNames(workerOnly); len(got) != 1 {
		t.Fatalf("worker-only runners = %v, want [river-worker] only", got)
	}
	if got := workerOnly.shutdownRunners(); len(got) != 1 {
		t.Fatalf("worker-only shutdown runners = %v, want a single runner", got)
	}
}
