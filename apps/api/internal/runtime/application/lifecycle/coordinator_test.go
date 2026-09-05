package lifecycle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/application/lifecycle"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

type fakeRunner struct {
	name    string
	kind    domain.ComponentKind
	stopErr error
	stopped bool
}

func (f *fakeRunner) Name() string                { return f.name }
func (f *fakeRunner) Kind() domain.ComponentKind  { return f.kind }
func (f *fakeRunner) Start(context.Context) error { return nil }
func (f *fakeRunner) Stop(context.Context) error {
	f.stopped = true
	return f.stopErr
}

type fakeResourceCloser struct {
	closed bool
	err    error
}

func (f *fakeResourceCloser) Close() error {
	f.closed = true
	return f.err
}

func TestLifecycleCoordinatorFlow(t *testing.T) {
	coord := lifecycle.NewCoordinator(50 * time.Millisecond)
	assertInitialState(t, coord)
	assertInitializationAndRecovery(t, coord)
	assertReadinessTransition(t, coord)
	assertDrainAndShutdown(t, coord)
	assertResourceClose(t, coord)
}

func assertInitialState(t *testing.T, coord *lifecycle.Coordinator) {
	t.Helper()
	if coord.State() != domain.StateCreated {
		t.Fatalf("state = %s, want created", coord.State())
	}
}

func assertInitializationAndRecovery(t *testing.T, coord *lifecycle.Coordinator) {
	t.Helper()
	if err := coord.BeginInitialization(); err != nil {
		t.Fatalf("BeginInitialization failed: %v", err)
	}
	if coord.State() != domain.StateInitializing {
		t.Fatalf("state = %s, want initializing", coord.State())
	}
	if err := coord.BeginRecovery(); err != nil {
		t.Fatalf("BeginRecovery failed: %v", err)
	}
	if coord.CanAcceptTasks() {
		t.Fatal("CanAcceptTasks should be false during recovering")
	}
}

func assertReadinessTransition(t *testing.T, coord *lifecycle.Coordinator) {
	t.Helper()
	healthyComponents := []domain.ComponentStatus{
		{Name: "postgres", Kind: domain.ComponentKindDatabase, Ready: true, Required: true},
	}
	if err := coord.MarkReady(healthyComponents); err != nil {
		t.Fatalf("MarkReady failed: %v", err)
	}
	if !coord.CanAcceptTasks() {
		t.Fatal("CanAcceptTasks should be true after ready")
	}
}

func assertDrainAndShutdown(t *testing.T, coord *lifecycle.Coordinator) {
	t.Helper()
	runner := &fakeRunner{name: "http", kind: domain.ComponentKindHTTP}
	if err := coord.DrainAndShutdown(context.Background(), []ports.Runner{runner}); err != nil {
		t.Fatalf("DrainAndShutdown failed: %v", err)
	}
	if !runner.stopped {
		t.Fatal("runner was not stopped")
	}
	if coord.State() != domain.StateDraining {
		t.Fatalf("state = %s, want draining", coord.State())
	}
}

func assertResourceClose(t *testing.T, coord *lifecycle.Coordinator) {
	t.Helper()
	closer := &fakeResourceCloser{}
	if err := coord.StopResources(closer); err != nil {
		t.Fatalf("StopResources failed: %v", err)
	}
	if !closer.closed {
		t.Fatal("resource closer was not closed")
	}
	if coord.State() != domain.StateStopped {
		t.Fatalf("state = %s, want stopped", coord.State())
	}
}

func TestLifecycleCoordinatorMarkReadyFailsWhenUnready(t *testing.T) {
	coord := lifecycle.NewCoordinator(10 * time.Millisecond)
	_ = coord.BeginInitialization()
	_ = coord.BeginRecovery()

	unreadyComponents := []domain.ComponentStatus{
		{Name: "nats", Kind: domain.ComponentKindNATS, Ready: false, Required: true},
	}
	err := coord.MarkReady(unreadyComponents)
	if err == nil {
		t.Fatal("MarkReady succeeded, want unready error")
	}
	if coord.State() != domain.StateRecovering {
		t.Fatalf("state should remain recovering on failure, got %s", coord.State())
	}
}

func TestLifecycleCoordinatorDrainErrorAggregation(t *testing.T) {
	coord := lifecycle.NewCoordinator(10 * time.Millisecond)
	_ = coord.BeginInitialization()
	_ = coord.BeginRecovery()
	_ = coord.MarkReady([]domain.ComponentStatus{})

	stopErr := errors.New("timeout")
	runner := &fakeRunner{name: "failing", kind: domain.ComponentKindRiver, stopErr: stopErr}

	err := coord.DrainAndShutdown(context.Background(), []ports.Runner{runner})
	if err == nil {
		t.Fatal("DrainAndShutdown succeeded, want error")
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("error = %v, want %v", err, stopErr)
	}
}
