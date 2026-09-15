package orchestration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/application/lifecycle"
	"ideaboard/api/internal/runtime/application/orchestration"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

type fakeRunner struct {
	name     string
	kind     domain.ComponentKind
	startErr error
	started  bool
	stopped  bool
}

func (f *fakeRunner) Name() string               { return f.name }
func (f *fakeRunner) Kind() domain.ComponentKind { return f.kind }
func (f *fakeRunner) Start(context.Context) error {
	f.started = true
	return f.startErr
}
func (f *fakeRunner) Stop(context.Context) error { f.stopped = true; return nil }

type fakeHealthChecker struct {
	status domain.ComponentStatus
}

func (f *fakeHealthChecker) CheckHealth(context.Context) domain.ComponentStatus {
	return f.status
}

type fakeRecoveryCoordinator struct {
	recovered int
	err       error
}

func (f *fakeRecoveryCoordinator) RecoverRunningTasks(context.Context) (int, error) {
	return f.recovered, f.err
}

func TestExecuteStartupHappyPath(t *testing.T) {
	ctx := context.Background()
	coord := lifecycle.NewCoordinator(10 * time.Millisecond)

	recovery := &fakeRecoveryCoordinator{recovered: 3}
	runner := &fakeRunner{name: "river", kind: domain.ComponentKindRiver}
	checker := &fakeHealthChecker{status: domain.ComponentStatus{
		Name:     "river",
		Kind:     domain.ComponentKindRiver,
		Ready:    true,
		Required: true,
	}}

	err := orchestration.ExecuteStartup(ctx, coord, orchestration.StartupDependencies{
		Recovery:       recovery,
		Runners:        []ports.Runner{runner},
		HealthCheckers: []ports.HealthChecker{checker},
	})
	if err != nil {
		t.Fatalf("ExecuteStartup failed: %v", err)
	}

	if coord.State() != domain.StateReady {
		t.Fatalf("state = %s, want ready", coord.State())
	}
	if !runner.started {
		t.Fatal("runner was not started")
	}
}

func TestExecuteStartupFailsOnRecoveryError(t *testing.T) {
	ctx := context.Background()
	coord := lifecycle.NewCoordinator(10 * time.Millisecond)

	recoveryErr := errors.New("recovery failed")
	recovery := &fakeRecoveryCoordinator{err: recoveryErr}
	runner := &fakeRunner{name: "river", kind: domain.ComponentKindRiver}

	err := orchestration.ExecuteStartup(ctx, coord, orchestration.StartupDependencies{
		Recovery: recovery,
		Runners:  []ports.Runner{runner},
	})
	if err == nil {
		t.Fatal("ExecuteStartup succeeded, want recovery error")
	}
	if !errors.Is(err, recoveryErr) {
		t.Fatalf("error = %v, want %v", err, recoveryErr)
	}
	if runner.started {
		t.Fatal("runner should not be started when recovery fails")
	}
	if coord.State() != domain.StateRecovering {
		t.Fatalf("state = %s, want recovering", coord.State())
	}
}

func TestExecuteStartupRollsBackStartedRunners(t *testing.T) {
	coord := lifecycle.NewCoordinator(10 * time.Millisecond)
	first := &fakeRunner{name: "first", kind: domain.ComponentKindRiver}
	second := &fakeRunner{name: "second", kind: domain.ComponentKindHTTP, startErr: errors.New("bind failed")}

	err := orchestration.ExecuteStartup(context.Background(), coord, orchestration.StartupDependencies{
		Runners: []ports.Runner{first, second},
	})
	if err == nil {
		t.Fatal("ExecuteStartup succeeded, want startup error")
	}
	if !first.stopped {
		t.Fatal("already-started runner was not rolled back")
	}
}
