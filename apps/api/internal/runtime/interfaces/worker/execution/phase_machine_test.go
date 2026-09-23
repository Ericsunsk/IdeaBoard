package execution

import "testing"

func TestPhaseMachineContract(t *testing.T) {
	machine := newPhaseMachine("phase-contract")
	if got := machine.current(); got != phasePreflight {
		t.Fatalf("initial phase = %q, want %q", got, phasePreflight)
	}
	for _, p := range []phase{
		phaseAssigned,
		phaseProvider,
		phaseFinalizing,
		phaseCompleted,
	} {
		if err := machine.advance(p); err != nil {
			t.Fatalf("advance to %s: %v", p, err)
		}
	}
	if err := machine.advance(phaseFailed); err == nil {
		t.Fatal("terminal phase must reject further transitions")
	}
}

func TestPhaseMachineAllowsConvergenceOutcomes(t *testing.T) {
	for name, terminal := range map[string]phase{
		"cancel":  phaseCancelled,
		"requeue": phaseRequeued,
		"drop":    phaseDropped,
		"failure": phaseFailed,
	} {
		t.Run(name, func(t *testing.T) {
			machine := newPhaseMachine("phase-" + name)
			for _, p := range []phase{phaseAssigned, phaseProvider} {
				if err := machine.advance(p); err != nil {
					t.Fatalf("advance to %s: %v", p, err)
				}
			}
			if err := machine.advance(terminal); err != nil {
				t.Fatalf("advance to terminal %s: %v", terminal, err)
			}
		})
	}
}

func TestPhaseMachineRejectsSkippingProvider(t *testing.T) {
	machine := newPhaseMachine("phase-invalid")
	if err := machine.advance(phaseFinalizing); err == nil {
		t.Fatal("preflight must not skip assignment/provider phases")
	}
}

func TestPhaseMachineAllowsTerminalFailureFromPreflight(t *testing.T) {
	machine := newPhaseMachine("phase-preflight-failure")
	if err := machine.advance(phaseFailed); err != nil {
		t.Fatalf("deterministic preflight failure must reach failed: %v", err)
	}
}

func (m *phaseMachine) current() phase {
	if m == nil {
		return ""
	}
	return m.phase
}
