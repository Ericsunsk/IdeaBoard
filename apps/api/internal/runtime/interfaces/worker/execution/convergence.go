package execution

type convergenceDecider interface {
	DecideConvergence(input convergenceInput) convergenceDecision
}

type convergencePolicy struct{}

var _ convergenceDecider = convergencePolicy{}

type convergenceOutcome string

const (
	convergenceCancel                        convergenceOutcome = "cancel"
	convergenceInterruptRequeue              convergenceOutcome = "interrupt_requeue"
	convergenceFinalFail                     convergenceOutcome = "final_fail"
	convergenceRequeue                       convergenceOutcome = "requeue"
	convergenceDropConvergedDirtyWork        convergenceOutcome = "drop_converged_dirty_work"
	convergenceFailoverExhaustedOrNoCapacity convergenceOutcome = "failover_exhausted_or_no_capacity"
)

type convergenceInput struct {
	IsExecutionInterrupted bool
	IsTaskCancelled        bool
	ProviderFailure        convergenceProviderFailure
	Failover               convergenceFailover
	Task                   convergenceTaskContext
}

type convergenceProviderFailure struct {
	IsFailoverable bool
	Reason         string
	Stage          string
	Category       string
}

type convergenceFailover struct {
	RemainingAccountCandidates int
	HasFailoverCapacity        bool
}

type convergenceTaskContext struct {
	IsDirtyConverged bool
}

type convergenceDecision struct {
	Outcome         convergenceOutcome
	Reason          string
	FailureStage    string
	FailureCategory string
}

func (convergencePolicy) DecideConvergence(input convergenceInput) convergenceDecision {
	decision := convergenceDecision{
		Reason:          input.ProviderFailure.Reason,
		FailureStage:    input.ProviderFailure.Stage,
		FailureCategory: input.ProviderFailure.Category,
	}

	if input.IsExecutionInterrupted {
		if input.IsTaskCancelled {
			decision.Outcome = convergenceCancel
			return decision
		}
		decision.Outcome = convergenceInterruptRequeue
		return decision
	}

	if input.Task.IsDirtyConverged {
		decision.Outcome = convergenceDropConvergedDirtyWork
		return decision
	}

	if !input.ProviderFailure.IsFailoverable {
		decision.Outcome = convergenceFinalFail
		return decision
	}

	if input.Failover.RemainingAccountCandidates <= 0 || !input.Failover.HasFailoverCapacity {
		decision.Outcome = convergenceFailoverExhaustedOrNoCapacity
		return decision
	}

	decision.Outcome = convergenceRequeue
	return decision
}
