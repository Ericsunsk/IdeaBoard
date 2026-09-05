package execution

import "testing"

func TestTaskExecutionPhaseMachineContract(t *testing.T) {
	machine := newTaskExecutionPhaseMachine("phase-contract")
	if got := machine.current(); got != taskExecutionPhasePreflight {
		t.Fatalf("initial phase = %q, want %q", got, taskExecutionPhasePreflight)
	}
	for _, phase := range []taskExecutionPhase{
		taskExecutionPhaseAssigned,
		taskExecutionPhaseProvider,
		taskExecutionPhaseFinalizing,
		taskExecutionPhaseCompleted,
	} {
		if err := machine.advance(phase); err != nil {
			t.Fatalf("advance to %s: %v", phase, err)
		}
	}
	if err := machine.advance(taskExecutionPhaseFailed); err == nil {
		t.Fatal("terminal phase must reject further transitions")
	}
}

func TestTaskExecutionPhaseMachineAllowsConvergenceOutcomes(t *testing.T) {
	for name, terminal := range map[string]taskExecutionPhase{
		"cancel":  taskExecutionPhaseCancelled,
		"requeue": taskExecutionPhaseRequeued,
		"drop":    taskExecutionPhaseDropped,
		"failure": taskExecutionPhaseFailed,
	} {
		t.Run(name, func(t *testing.T) {
			machine := newTaskExecutionPhaseMachine("phase-" + name)
			for _, phase := range []taskExecutionPhase{taskExecutionPhaseAssigned, taskExecutionPhaseProvider} {
				if err := machine.advance(phase); err != nil {
					t.Fatalf("advance to %s: %v", phase, err)
				}
			}
			if err := machine.advance(terminal); err != nil {
				t.Fatalf("advance to terminal %s: %v", terminal, err)
			}
		})
	}
}

func TestTaskExecutionPhaseMachineRejectsSkippingProvider(t *testing.T) {
	machine := newTaskExecutionPhaseMachine("phase-invalid")
	if err := machine.advance(taskExecutionPhaseFinalizing); err == nil {
		t.Fatal("preflight must not skip assignment/provider phases")
	}
}

func (m *taskExecutionPhaseMachine) current() taskExecutionPhase {
	if m == nil {
		return ""
	}
	return m.phase
}
