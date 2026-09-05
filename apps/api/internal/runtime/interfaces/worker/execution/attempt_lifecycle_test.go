package execution

import (
	"context"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"testing"
	"time"
)

func TestTaskAttemptLifecycleContractBeginsWithExactLeaseIdentity(t *testing.T) {
	startedAt := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	providerStartedAt := startedAt.Add(40 * time.Millisecond)
	assignment := taskqueueports.TaskAssignment{
		TaskID: "attempt-lifecycle-task", AccountID: 12, ProviderEndpointID: 34,
		CredentialID: 56, LeaseToken: "lease-attempt-lifecycle",
	}
	runtime := &recordingTaskAttemptLifecycleRuntime{
		startRef: tasks.TaskAttemptRef{
			ID: 99, TaskID: assignment.TaskID, AccountID: assignment.AccountID,
			ProviderEndpointID: assignment.ProviderEndpointID, CredentialID: assignment.CredentialID,
			LeaseToken: assignment.LeaseToken,
		},
	}
	lifecycle := newAttemptLifecycle(runtime)

	attempt, err := lifecycle.begin(context.Background(), &tasks.TaskPayload{
		TaskID: assignment.TaskID, RiverJobID: 78,
	}, assignment, startedAt, providerStartedAt)
	if err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if runtime.start.TaskID != assignment.TaskID || runtime.start.RiverJobID != 78 {
		t.Fatalf("start task identity = %+v", runtime.start)
	}
	if runtime.start.AccountID != assignment.AccountID ||
		runtime.start.ProviderEndpointID != assignment.ProviderEndpointID ||
		runtime.start.CredentialID != assignment.CredentialID ||
		runtime.start.LeaseToken != assignment.LeaseToken {
		t.Fatalf("start lease identity = %+v, want assignment %+v", runtime.start, assignment)
	}
	if !runtime.start.ProviderStartedAt.Equal(providerStartedAt) {
		t.Fatalf("provider_started_at = %s, want %s", runtime.start.ProviderStartedAt, providerStartedAt)
	}
	if attempt.attemptRef.ID != runtime.startRef.ID || !attempt.startedAt.Equal(startedAt) {
		t.Fatalf("attempt context = %+v", attempt)
	}
}

func TestTaskAttemptLifecycleContractFallbackFinishPreservesTimelineAndLease(t *testing.T) {
	startedAt := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	providerStartedAt := startedAt.Add(50 * time.Millisecond)
	providerFinishedAt := providerStartedAt.Add(150 * time.Millisecond)
	completedAt := startedAt.Add(500 * time.Millisecond)
	runtime := &recordingTaskAttemptLifecycleRuntime{}
	lifecycle := newAttemptLifecycle(runtime)
	attempt := newAttemptContext(
		&tasks.TaskPayload{TaskID: "attempt-fallback-task"},
		taskqueueports.TaskAssignment{TaskID: "attempt-fallback-task", AccountID: 12, ProviderEndpointID: 34, CredentialID: 56, LeaseToken: "lease-fallback"},
		12,
		startedAt,
		tasks.TaskAttemptRef{ID: 99, TaskID: "attempt-fallback-task", AccountID: 12, ProviderEndpointID: 34, CredentialID: 56, LeaseToken: "lease-fallback"},
	).withProviderOutput(runtimeports.ProviderCallOutput{Timeline: runtimeports.ProviderCallTimeline{
		ProviderStartedAt: &providerStartedAt, ProviderFinishedAt: &providerFinishedAt,
	}})

	lifecycle.finishFallback(
		context.Background(), attempt, tasks.TaskAttemptStatusFailed, "writeback failed",
		tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, completedAt,
	)
	if len(runtime.finishes) != 1 {
		t.Fatalf("finish calls = %d, want 1", len(runtime.finishes))
	}
	finish := runtime.finishes[0]
	if finish.ID != 99 || finish.TaskID != attempt.task.TaskID || finish.LeaseToken != "lease-fallback" {
		t.Fatalf("finish identity = %+v", finish)
	}
	if finish.Status != tasks.TaskAttemptStatusFailed || finish.FailureStage != tasks.ExecutionStageFinalizing || finish.FailureCategory != tasks.FailureCategoryFinalizeError {
		t.Fatalf("finish outcome = %+v", finish)
	}
	if finish.ProviderLatencyMS != 150 || finish.TotalLatencyMS != 500 || !finish.CompletedAt.Equal(completedAt) {
		t.Fatalf("finish timing = %+v", finish)
	}
}

func TestTaskAttemptLifecycleContractDoesNotFallbackFinishWithoutAttempt(t *testing.T) {
	runtime := &recordingTaskAttemptLifecycleRuntime{}
	lifecycle := newAttemptLifecycle(runtime)

	lifecycle.finishFallback(context.Background(), attemptContext{}, tasks.TaskAttemptStatusFailed, "ignored", "", "", time.Now())
	if len(runtime.finishes) != 0 {
		t.Fatalf("finish calls = %d, want 0", len(runtime.finishes))
	}
}

type recordingTaskAttemptLifecycleRuntime struct {
	start     tasks.TaskAttemptStart
	startRef  tasks.TaskAttemptRef
	startErr  error
	finishes  []tasks.TaskAttemptFinish
	finishErr error
}

func (r *recordingTaskAttemptLifecycleRuntime) StartTaskAttempt(_ context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	r.start = start
	return r.startRef, r.startErr
}

func (r *recordingTaskAttemptLifecycleRuntime) FinishTaskAttempt(_ context.Context, finish tasks.TaskAttemptFinish) error {
	r.finishes = append(r.finishes, finish)
	return r.finishErr
}

func TestAttemptContextBuildsFinishFromProviderTimeline(t *testing.T) {
	startedAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	providerStartedAt := startedAt.Add(50 * time.Millisecond)
	providerFinishedAt := providerStartedAt.Add(150 * time.Millisecond)
	finalizeStartedAt := providerFinishedAt.Add(25 * time.Millisecond)
	finalizeFinishedAt := finalizeStartedAt.Add(75 * time.Millisecond)
	completedAt := startedAt.Add(500 * time.Millisecond)
	attempt := newAttemptContext(
		&tasks.TaskPayload{TaskID: "task-attempt-context"},
		taskqueueports.TaskAssignment{TaskID: "task-attempt-context", AccountID: 12},
		12,
		startedAt,
		tasks.TaskAttemptRef{ID: 99},
	).withProviderOutput(runtimeports.ProviderCallOutput{
		Timeline: runtimeports.ProviderCallTimeline{
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
