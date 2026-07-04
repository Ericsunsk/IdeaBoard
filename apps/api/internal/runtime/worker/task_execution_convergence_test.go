package worker

import (
	"testing"

	"ideaboard/api/internal/tasking/tasks"
)

func TestTaskExecutionConvergencePolicyDecidesOutcomes(t *testing.T) {
	tests := []struct {
		name  string
		input taskExecutionConvergenceInput
		want  taskExecutionConvergenceOutcome
	}{
		{
			name: "cancellation wins interrupted provider work",
			input: taskExecutionConvergenceInput{
				IsExecutionInterrupted: true,
				IsTaskCancelled:        true,
				ProviderFailure:        providerFailureInput(true),
				Failover:               failoverInput(1, true),
			},
			want: taskExecutionConvergenceCancel,
		},
		{
			name: "interrupted non-cancelled work requeues through interrupt path",
			input: taskExecutionConvergenceInput{
				IsExecutionInterrupted: true,
				ProviderFailure:        providerFailureInput(true),
				Failover:               failoverInput(1, true),
			},
			want: taskExecutionConvergenceInterruptRequeue,
		},
		{
			name: "non-failover provider failure final fails",
			input: taskExecutionConvergenceInput{
				ProviderFailure: providerFailureInput(false),
			},
			want: taskExecutionConvergenceFinalFail,
		},
		{
			name: "failover provider failure with retry capacity requeues",
			input: taskExecutionConvergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(2, true),
			},
			want: taskExecutionConvergenceRequeue,
		},
		{
			name: "failover provider failure without remaining account candidates stops requeue",
			input: taskExecutionConvergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(0, true),
			},
			want: taskExecutionConvergenceFailoverExhaustedOrNoCapacity,
		},
		{
			name: "dirty converged task drops queued work",
			input: taskExecutionConvergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(1, true),
				Task: taskExecutionConvergenceTaskContext{
					IsDirtyConverged: true,
				},
			},
			want: taskExecutionConvergenceDropConvergedDirtyWork,
		},
		{
			name: "failover exhausted or no capacity stops requeue",
			input: taskExecutionConvergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(1, false),
			},
			want: taskExecutionConvergenceFailoverExhaustedOrNoCapacity,
		},
	}

	var decider taskExecutionConvergenceDecider = taskExecutionConvergencePolicy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decider.DecideTaskExecutionConvergence(tt.input)
			if got.Outcome != tt.want {
				t.Fatalf("outcome = %s, want %s", got.Outcome, tt.want)
			}
			if got.Reason != tt.input.ProviderFailure.Reason {
				t.Fatalf("reason = %q, want %q", got.Reason, tt.input.ProviderFailure.Reason)
			}
			if got.FailureStage != tt.input.ProviderFailure.Stage {
				t.Fatalf("failure stage = %q, want %q", got.FailureStage, tt.input.ProviderFailure.Stage)
			}
			if got.FailureCategory != tt.input.ProviderFailure.Category {
				t.Fatalf("failure category = %q, want %q", got.FailureCategory, tt.input.ProviderFailure.Category)
			}
		})
	}
}

func providerFailureInput(isFailoverable bool) taskExecutionConvergenceProviderFailure {
	return taskExecutionConvergenceProviderFailure{
		IsFailoverable: isFailoverable,
		Reason:         "上游图像服务调用失败",
		Stage:          tasks.ExecutionStageProvider,
		Category:       tasks.FailureCategoryProviderError,
	}
}

func failoverInput(remainingAccountCandidates int, hasFailoverCapacity bool) taskExecutionConvergenceFailover {
	return taskExecutionConvergenceFailover{
		RemainingAccountCandidates: remainingAccountCandidates,
		HasFailoverCapacity:        hasFailoverCapacity,
	}
}
