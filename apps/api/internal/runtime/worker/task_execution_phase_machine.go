package worker

import "fmt"

// taskExecutionPhase is the worker's in-process execution lifecycle. The
// database task status and River job remain the durable facts; this machine
// only makes the ordering of worker side effects explicit and fail-closed.
type taskExecutionPhase string

const (
	taskExecutionPhasePreflight  taskExecutionPhase = "preflight"
	taskExecutionPhaseAssigned   taskExecutionPhase = "assigned"
	taskExecutionPhaseProvider   taskExecutionPhase = "provider"
	taskExecutionPhaseFinalizing taskExecutionPhase = "finalizing"
	taskExecutionPhaseCompleted  taskExecutionPhase = "completed"
	taskExecutionPhaseFailed     taskExecutionPhase = "failed"
	taskExecutionPhaseCancelled  taskExecutionPhase = "cancelled"
	taskExecutionPhaseRequeued   taskExecutionPhase = "requeued"
	taskExecutionPhaseDropped    taskExecutionPhase = "dropped"
)

var taskExecutionPhaseTransitions = map[taskExecutionPhase]map[taskExecutionPhase]struct{}{
	taskExecutionPhasePreflight: {
		taskExecutionPhaseAssigned:  {},
		taskExecutionPhaseCancelled: {},
		taskExecutionPhaseRequeued:  {},
		taskExecutionPhaseDropped:   {},
	},
	taskExecutionPhaseAssigned: {
		taskExecutionPhaseProvider:  {},
		taskExecutionPhaseRequeued:  {},
		taskExecutionPhaseCancelled: {},
		taskExecutionPhaseDropped:   {},
	},
	taskExecutionPhaseProvider: {
		taskExecutionPhaseFinalizing: {},
		taskExecutionPhaseFailed:     {},
		taskExecutionPhaseCancelled:  {},
		taskExecutionPhaseRequeued:   {},
		taskExecutionPhaseDropped:    {},
	},
	taskExecutionPhaseFinalizing: {
		taskExecutionPhaseCompleted: {},
		taskExecutionPhaseFailed:    {},
		taskExecutionPhaseCancelled: {},
		taskExecutionPhaseRequeued:  {},
		taskExecutionPhaseDropped:   {},
	},
}

type taskExecutionPhaseMachine struct {
	taskID string
	phase  taskExecutionPhase
}

func newTaskExecutionPhaseMachine(taskID string) *taskExecutionPhaseMachine {
	return &taskExecutionPhaseMachine{taskID: taskID, phase: taskExecutionPhasePreflight}
}

func (m *taskExecutionPhaseMachine) advance(next taskExecutionPhase) error {
	if m == nil {
		return fmt.Errorf("task execution phase machine is unavailable")
	}
	if _, terminal := map[taskExecutionPhase]struct{}{
		taskExecutionPhaseCompleted: {},
		taskExecutionPhaseFailed:    {},
		taskExecutionPhaseCancelled: {},
		taskExecutionPhaseRequeued:  {},
		taskExecutionPhaseDropped:   {},
	}[m.phase]; terminal {
		return fmt.Errorf("task %s execution phase is terminal: %s", m.taskID, m.phase)
	}
	if _, ok := taskExecutionPhaseTransitions[m.phase][next]; !ok {
		return fmt.Errorf("task %s invalid execution phase transition: %s -> %s", m.taskID, m.phase, next)
	}
	m.phase = next
	return nil
}
