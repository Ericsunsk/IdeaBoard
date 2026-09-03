package worker

import (
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

func TestTaskExecutionAttemptContextBuildsFinishFromProviderTimeline(t *testing.T) {
	startedAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	providerStartedAt := startedAt.Add(50 * time.Millisecond)
	providerFinishedAt := providerStartedAt.Add(150 * time.Millisecond)
	finalizeStartedAt := providerFinishedAt.Add(25 * time.Millisecond)
	finalizeFinishedAt := finalizeStartedAt.Add(75 * time.Millisecond)
	completedAt := startedAt.Add(500 * time.Millisecond)
	attempt := newTaskExecutionAttemptContext(
		&tasks.TaskPayload{TaskID: "task-attempt-context"},
		taskqueueports.TaskAssignment{TaskID: "task-attempt-context", AccountID: 12},
		12,
		startedAt,
		tasks.TaskAttemptRef{ID: 99},
	).withProviderOutput(providerCallOutput{
		Timeline: providerCallTimeline{
			ProviderStartedAt:  &providerStartedAt,
			ProviderFinishedAt: &providerFinishedAt,
			FinalizeStartedAt:  &finalizeStartedAt,
			FinalizeFinishedAt: &finalizeFinishedAt,
		},
	})

	finish, ok := attempt.buildAttemptFinish(
		tasks.TaskAttemptStatusFailed,
		"provider failed",
		tasks.ExecutionStageProvider,
		tasks.FailureCategoryProviderError,
		completedAt,
	)
	if !ok {
		t.Fatal("expected finish to be built")
	}
	if finish.ID != 99 {
		t.Fatalf("finish ID = %d, want 99", finish.ID)
	}
	if finish.ProviderLatencyMS != 150 {
		t.Fatalf("provider latency = %d, want 150", finish.ProviderLatencyMS)
	}
	if finish.FinalizeLatencyMS != 75 {
		t.Fatalf("finalize latency = %d, want 75", finish.FinalizeLatencyMS)
	}
	if finish.TotalLatencyMS != 500 {
		t.Fatalf("total latency = %d, want 500", finish.TotalLatencyMS)
	}
	if finish.FailureStage != tasks.ExecutionStageProvider {
		t.Fatalf("failure stage = %q", finish.FailureStage)
	}
	if finish.FailureCategory != tasks.FailureCategoryProviderError {
		t.Fatalf("failure category = %q", finish.FailureCategory)
	}
}
