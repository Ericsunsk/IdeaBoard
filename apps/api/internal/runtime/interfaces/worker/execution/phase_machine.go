package execution

import "fmt"

// phase is the worker's in-process execution lifecycle. The
// database task status and River job remain the durable facts; this machine
// only makes the ordering of worker side effects explicit and fail-closed.
type phase string

const (
	phasePreflight  phase = "preflight"
	phaseAssigned   phase = "assigned"
	phaseProvider   phase = "provider"
	phaseFinalizing phase = "finalizing"
	phaseCompleted  phase = "completed"
	phaseFailed     phase = "failed"
	phaseCancelled  phase = "cancelled"
	phaseRequeued   phase = "requeued"
	phaseDropped    phase = "dropped"
)

var phaseTransitions = map[phase]map[phase]struct{}{
	phasePreflight: {
		phaseAssigned:  {},
		phaseCancelled: {},
		phaseRequeued:  {},
		phaseDropped:   {},
		// Deterministic preflight failures (unparseable params, unconfigured
		// model) terminate here without ever reaching assignment.
		phaseFailed: {},
	},
	phaseAssigned: {
		phaseProvider:  {},
		phaseRequeued:  {},
		phaseCancelled: {},
		phaseDropped:   {},
	},
	phaseProvider: {
		phaseFinalizing: {},
		phaseFailed:     {},
		phaseCancelled:  {},
		phaseRequeued:   {},
		phaseDropped:    {},
	},
	phaseFinalizing: {
		phaseCompleted: {},
		phaseFailed:    {},
		phaseCancelled: {},
		phaseRequeued:  {},
		phaseDropped:   {},
	},
}

type phaseMachine struct {
	taskID string
	phase  phase
}

func newPhaseMachine(taskID string) *phaseMachine {
	return &phaseMachine{taskID: taskID, phase: phasePreflight}
}

func (m *phaseMachine) advance(next phase) error {
	if m == nil {
		return fmt.Errorf("phase machine is unavailable")
	}
	if _, terminal := map[phase]struct{}{
		phaseCompleted: {},
		phaseFailed:    {},
		phaseCancelled: {},
		phaseRequeued:  {},
		phaseDropped:   {},
	}[m.phase]; terminal {
		return fmt.Errorf("task %s execution phase is terminal: %s", m.taskID, m.phase)
	}
	if _, ok := phaseTransitions[m.phase][next]; !ok {
		return fmt.Errorf("task %s invalid execution phase transition: %s -> %s", m.taskID, m.phase, next)
	}
	m.phase = next
	return nil
}
