package domain

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// State represents the explicit lifecycle stage of the runtime platform.
type State string

const (
	StateCreated      State = "created"
	StateInitializing State = "initializing"
	StateRecovering   State = "recovering"
	StateReady        State = "ready"
	StateDraining     State = "draining"
	StateStopped      State = "stopped"
)

var (
	// ErrInvalidTransition is returned when a requested state transition violates lifecycle invariants.
	ErrInvalidTransition = errors.New("invalid runtime state transition")
)

// StateMachine coordinates lifecycle state transitions and enforces runtime invariants.
type StateMachine struct {
	mu      sync.RWMutex
	current State
}

// NewStateMachine creates a state machine initialized in StateCreated.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		current: StateCreated,
	}
}

// Current returns the current lifecycle state.
func (sm *StateMachine) Current() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// TransitionTo attempts to transition the runtime to target state according to allowed invariants.
func (sm *StateMachine) TransitionTo(target State) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !isAllowedTransition(sm.current, target) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, sm.current, target)
	}

	sm.current = target
	return nil
}

// CanAcceptTasks returns true only when the runtime is fully ready.
// In recovering, draining, or stopped states, no new tasks can be accepted.
func (sm *StateMachine) CanAcceptTasks() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateReady
}

// IsDraining returns whether runtime is draining in-flight work.
func (sm *StateMachine) IsDraining() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateDraining
}

// IsStopped returns whether runtime is fully stopped and resources released.
func (sm *StateMachine) IsStopped() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateStopped
}

func isAllowedTransition(from, to State) bool {
	switch from {
	case StateCreated:
		return to == StateInitializing
	case StateInitializing:
		return to == StateRecovering || to == StateDraining
	case StateRecovering:
		return to == StateReady || to == StateDraining
	case StateReady:
		return to == StateDraining
	case StateDraining:
		return to == StateStopped
	default:
		return false
	}
}

// ComponentKind identifies categories of runtime infrastructure components.
type ComponentKind string

const (
	ComponentKindDatabase ComponentKind = "database"
	ComponentKindRiver    ComponentKind = "river"
	ComponentKindNATS     ComponentKind = "nats"
	ComponentKindAdapter  ComponentKind = "adapter"
	ComponentKindHTTP     ComponentKind = "http"
)

// ComponentStatus represents the health/readiness fact of a runtime component.
type ComponentStatus struct {
	Name     string
	Kind     ComponentKind
	Ready    bool
	Required bool
	Detail   string
}

// EvaluateReadiness verifies that all required components are healthy.
func EvaluateReadiness(components []ComponentStatus) error {
	var unready []string
	for _, comp := range components {
		if comp.Required && !comp.Ready {
			detail := comp.Detail
			if detail == "" {
				detail = "not ready"
			}
			unready = append(unready, fmt.Sprintf("%s (%s: %s)", comp.Name, comp.Kind, detail))
		}
	}
	if len(unready) > 0 {
		return fmt.Errorf("%w: %s", ErrComponentsUnready, strings.Join(unready, ", "))
	}
	return nil
}

// ErrComponentsUnready is returned when one or more required runtime components fail readiness evaluation.
var ErrComponentsUnready = errors.New("required runtime components are not ready")
