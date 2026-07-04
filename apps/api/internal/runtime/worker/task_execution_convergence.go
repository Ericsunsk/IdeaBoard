package worker

type taskExecutionConvergenceDecider interface {
	DecideTaskExecutionConvergence(input taskExecutionConvergenceInput) taskExecutionConvergenceDecision
}

type taskExecutionConvergencePolicy struct{}

var _ taskExecutionConvergenceDecider = taskExecutionConvergencePolicy{}

type taskExecutionConvergenceOutcome string

const (
	taskExecutionConvergenceCancel                        taskExecutionConvergenceOutcome = "cancel"
	taskExecutionConvergenceInterruptRequeue              taskExecutionConvergenceOutcome = "interrupt_requeue"
	taskExecutionConvergenceFinalFail                     taskExecutionConvergenceOutcome = "final_fail"
	taskExecutionConvergenceRequeue                       taskExecutionConvergenceOutcome = "requeue"
	taskExecutionConvergenceDropConvergedDirtyWork        taskExecutionConvergenceOutcome = "drop_converged_dirty_work"
	taskExecutionConvergenceFailoverExhaustedOrNoCapacity taskExecutionConvergenceOutcome = "failover_exhausted_or_no_capacity"
)

type taskExecutionConvergenceInput struct {
	IsExecutionInterrupted bool
	IsTaskCancelled        bool
	ProviderFailure        taskExecutionConvergenceProviderFailure
	Failover               taskExecutionConvergenceFailover
	Task                   taskExecutionConvergenceTaskContext
}

type taskExecutionConvergenceProviderFailure struct {
	IsFailoverable bool
	Reason         string
	Stage          string
	Category       string
}

type taskExecutionConvergenceFailover struct {
	RemainingAccountCandidates int
	HasFailoverCapacity        bool
}

type taskExecutionConvergenceTaskContext struct {
	IsDirtyConverged bool
}

type taskExecutionConvergenceDecision struct {
	Outcome         taskExecutionConvergenceOutcome
	Reason          string
	FailureStage    string
	FailureCategory string
}

func (taskExecutionConvergencePolicy) DecideTaskExecutionConvergence(input taskExecutionConvergenceInput) taskExecutionConvergenceDecision {
	decision := taskExecutionConvergenceDecision{
		Reason:          input.ProviderFailure.Reason,
		FailureStage:    input.ProviderFailure.Stage,
		FailureCategory: input.ProviderFailure.Category,
	}

	if input.IsExecutionInterrupted {
		if input.IsTaskCancelled {
			decision.Outcome = taskExecutionConvergenceCancel
			return decision
		}
		decision.Outcome = taskExecutionConvergenceInterruptRequeue
		return decision
	}

	if input.Task.IsDirtyConverged {
		decision.Outcome = taskExecutionConvergenceDropConvergedDirtyWork
		return decision
	}

	if !input.ProviderFailure.IsFailoverable {
		decision.Outcome = taskExecutionConvergenceFinalFail
		return decision
	}

	if input.Failover.RemainingAccountCandidates <= 0 || !input.Failover.HasFailoverCapacity {
		decision.Outcome = taskExecutionConvergenceFailoverExhaustedOrNoCapacity
		return decision
	}

	decision.Outcome = taskExecutionConvergenceRequeue
	return decision
}
