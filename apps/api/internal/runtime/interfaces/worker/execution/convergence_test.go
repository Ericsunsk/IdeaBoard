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
				ProviderFailure:        providerFailureInput(),
			},
			want: convergenceCancel,
		},
		{
			name: "interrupted non-cancelled work requeues through interrupt path",
			input: convergenceInput{
				IsExecutionInterrupted: true,
				ProviderFailure:        providerFailureInput(),
			},
			want: convergenceInterruptRequeue,
		},
		{
			name: "non-retryable provider failure final fails",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(),
			},
			want: convergenceFinalFail,
		},
		{
			name: "retryable provider failure below bound requeues",
			input: convergenceInput{
				ProviderFailure: retryableFailureInput(),
				Task:            convergenceTaskContext{RetryCount: 0},
			},
			want: convergenceRequeue,
		},
		{
			name: "retryable provider failure at bound final fails",
			input: convergenceInput{
				ProviderFailure: retryableFailureInput(),
				Task:            convergenceTaskContext{RetryCount: maxProviderFailureRequeues},
			},
			want: convergenceFinalFail,
		},
		{
			name: "retryable provider failure above bound final fails",
			input: convergenceInput{
				ProviderFailure: retryableFailureInput(),
				Task:            convergenceTaskContext{RetryCount: maxProviderFailureRequeues + 1},
			},
			want: convergenceFinalFail,
		},
		{
			name: "dirty converged task drops queued work",
			input: convergenceInput{
				ProviderFailure: providerFailureInput(),
				Task: convergenceTaskContext{
					IsDirtyConverged: true,
				},
			},
			want: convergenceDropConvergedDirtyWork,
		},
	}

	decider := convergencePolicy{}
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

func providerFailureInput() convergenceProviderFailure {
	return convergenceProviderFailure{
		Reason:   "上游图像服务调用失败",
		Stage:    tasks.ExecutionStageProvider,
		Category: tasks.FailureCategoryProviderError,
	}
}

func retryableFailureInput() convergenceProviderFailure {
	failure := providerFailureInput()
	failure.Retryable = true
	return failure
}
