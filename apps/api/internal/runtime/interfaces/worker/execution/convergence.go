package execution

type convergencePolicy struct{}

type convergenceOutcome string

const (
	convergenceCancel                 convergenceOutcome = "cancel"
	convergenceInterruptRequeue       convergenceOutcome = "interrupt_requeue"
	convergenceFinalFail              convergenceOutcome = "final_fail"
	convergenceRequeue                convergenceOutcome = "requeue"
	convergenceDropConvergedDirtyWork convergenceOutcome = "drop_converged_dirty_work"
)

// maxProviderFailureRequeues bounds worker-level retries for retryable
// provider failures (429/5xx/timeout/network): the gateway already retried
// internally, so the worker only buys a few delayed whole-job redeliveries
// via River backoff before converging terminally. Deterministic failures
// (4xx/auth/quota/input) never requeue. See ADR-0010.
const maxProviderFailureRequeues = 2

type convergenceInput struct {
	IsExecutionInterrupted bool
	IsTaskCancelled        bool
	ProviderFailure        convergenceProviderFailure
	Task                   convergenceTaskContext
}

type convergenceProviderFailure struct {
	Reason    string
	Stage     string
	Category  string
	Retryable bool
}

type convergenceTaskContext struct {
	IsDirtyConverged bool
	// RetryCount is the fresh tasks.retry_count at failure time: every
	// requeue and final failure bumps it, so it bounds total gateway sends.
	RetryCount int
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

	// Retryable provider failures below the requeue bound ride River backoff
	// as whole-job redeliveries (fresh token per attempt); everything else
	// converges terminally here.
	if input.ProviderFailure.Retryable && input.Task.RetryCount < maxProviderFailureRequeues {
		decision.Outcome = convergenceRequeue
		return decision
	}
	decision.Outcome = convergenceFinalFail
	return decision
}
