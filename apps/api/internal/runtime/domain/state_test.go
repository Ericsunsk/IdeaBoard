package domain_test

import (
	"errors"
	"testing"

	"ideaboard/api/internal/runtime/domain"
)

func TestStateMachineLifecycleHappyPath(t *testing.T) {
	sm := domain.NewStateMachine()
	if sm.Current() != domain.StateCreated {
		t.Fatalf("initial state = %s, want %s", sm.Current(), domain.StateCreated)
	}

	transitions := []struct {
		target        domain.State
		canAcceptWork bool
		isDraining    bool
		isStopped     bool
	}{
		{target: domain.StateInitializing, canAcceptWork: false, isDraining: false, isStopped: false},
		{target: domain.StateRecovering, canAcceptWork: false, isDraining: false, isStopped: false},
		{target: domain.StateReady, canAcceptWork: true, isDraining: false, isStopped: false},
		{target: domain.StateDraining, canAcceptWork: false, isDraining: true, isStopped: false},
		{target: domain.StateStopped, canAcceptWork: false, isDraining: false, isStopped: true},
	}

	for _, step := range transitions {
		if err := sm.TransitionTo(step.target); err != nil {
			t.Fatalf("failed transition to %s: %v", step.target, err)
		}
		if sm.Current() != step.target {
			t.Fatalf("state = %s, want %s", sm.Current(), step.target)
		}
		if sm.CanAcceptTasks() != step.canAcceptWork {
			t.Fatalf("state %s CanAcceptTasks = %v, want %v", step.target, sm.CanAcceptTasks(), step.canAcceptWork)
		}
		if sm.IsDraining() != step.isDraining {
			t.Fatalf("state %s IsDraining = %v, want %v", step.target, sm.IsDraining(), step.isDraining)
		}
		if sm.IsStopped() != step.isStopped {
			t.Fatalf("state %s IsStopped = %v, want %v", step.target, sm.IsStopped(), step.isStopped)
		}
	}
}

func TestStateMachineRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		from   domain.State
		target domain.State
	}{
		{from: domain.StateCreated, target: domain.StateReady},
		{from: domain.StateCreated, target: domain.StateRecovering},
		{from: domain.StateInitializing, target: domain.StateReady},
		{from: domain.StateRecovering, target: domain.StateStopped},
		{from: domain.StateReady, target: domain.StateInitializing},
		{from: domain.StateDraining, target: domain.StateReady},
		{from: domain.StateStopped, target: domain.StateReady},
	}

	for _, tc := range tests {
		sm := domain.NewStateMachine()
		setupToState(t, sm, tc.from)
		err := sm.TransitionTo(tc.target)
		if err == nil {
			t.Errorf("transition from %s to %s succeeded, want error", tc.from, tc.target)
		}
		if !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("error = %v, want ErrInvalidTransition", err)
		}
	}
}

func TestEvaluateReadiness(t *testing.T) {
	t.Run("all required components ready", func(t *testing.T) {
		err := domain.EvaluateReadiness([]domain.ComponentStatus{
			{Name: "postgres", Kind: domain.ComponentKindDatabase, Ready: true, Required: true},
			{Name: "river", Kind: domain.ComponentKindRiver, Ready: true, Required: true},
			{Name: "optional_adapter", Kind: domain.ComponentKindAdapter, Ready: false, Required: false},
		})
		if err != nil {
			t.Fatalf("EvaluateReadiness returned error: %v", err)
		}
	})

	t.Run("required component unready", func(t *testing.T) {
		err := domain.EvaluateReadiness([]domain.ComponentStatus{
			{Name: "postgres", Kind: domain.ComponentKindDatabase, Ready: true, Required: true},
			{Name: "nats", Kind: domain.ComponentKindNATS, Ready: false, Required: true, Detail: "connection refused"},
		})
		if err == nil {
			t.Fatal("EvaluateReadiness succeeded, want error")
		}
		if !errors.Is(err, domain.ErrComponentsUnready) {
			t.Fatalf("error = %v, want ErrComponentsUnready", err)
		}
	})
}

func setupToState(t *testing.T, sm *domain.StateMachine, target domain.State) {
	t.Helper()
	sequence := []domain.State{
		domain.StateInitializing,
		domain.StateRecovering,
		domain.StateReady,
		domain.StateDraining,
		domain.StateStopped,
	}
	for _, s := range sequence {
		if sm.Current() == target {
			return
		}
		if err := sm.TransitionTo(s); err != nil {
			t.Fatalf("setup transition to %s failed: %v", s, err)
		}
	}
}
