package execution

import (
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
)

func TestConvergencePolicyDecidesOutcomes(t *testing.T) {
	tests := []struct {
		name  string
		input convergenceInput
		want  convergenceOutcome
	}{
		{
			name: "cancellation wins interrupted provider work",
			input: convergenceInput{
				IsExecutionInterrupted: true,
				IsTaskCancelled:        true,
				ProviderFailure:        providerFailureInput(true),
				Failover:               failoverInput(1, true),
			},
			want: convergenceCancel,
		},
		{
			name: "interrupted non-cancelled work requeues through interrupt path",
			input: convergenceInput{
				IsExecutionInterrupted: true,
				ProviderFailure:        providerFailureInput(true),
				Failover:               failoverInput(1, true),
			},
			want: convergenceInterruptRequeue,
		},
		{
			name: "non-failover provider failure final fails",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(false),
			},
			want: convergenceFinalFail,
		},
		{
			name: "failover provider failure with retry capacity requeues",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(2, true),
			},
			want: convergenceRequeue,
		},
		{
			name: "failover provider failure without remaining account candidates stops requeue",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(0, true),
			},
			want: convergenceFailoverExhaustedOrNoCapacity,
		},
		{
			name: "dirty converged task drops queued work",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(1, true),
				Task: convergenceTaskContext{
					IsDirtyConverged: true,
				},
			},
			want: convergenceDropConvergedDirtyWork,
		},
		{
			name: "failover exhausted or no capacity stops requeue",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(true),
				Failover:        failoverInput(1, false),
			},
			want: convergenceFailoverExhaustedOrNoCapacity,
		},
	}

	var decider convergenceDecider = convergencePolicy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decider.DecideConvergence(tt.input)
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

func providerFailureInput(isFailoverable bool) convergenceProviderFailure {
	return convergenceProviderFailure{
		IsFailoverable: isFailoverable,
		Reason:         "上游图像服务调用失败",
		Stage:          tasks.ExecutionStageProvider,
		Category:       tasks.FailureCategoryProviderError,
	}
}

func failoverInput(remainingAccountCandidates int, hasFailoverCapacity bool) convergenceFailover {
	return convergenceFailover{
		RemainingAccountCandidates: remainingAccountCandidates,
		HasFailoverCapacity:        hasFailoverCapacity,
	}
}
