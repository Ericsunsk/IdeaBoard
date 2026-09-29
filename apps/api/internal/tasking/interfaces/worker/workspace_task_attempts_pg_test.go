package worker

// Postgres attempt-fact round trip over the fixture schema (#848).
// SKIPs gracefully without API_TEST_DATABASE_DSN, like every other
// fixture-DB test in this package.

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/workspacetasks"
)

func TestWorkspaceAttemptRecorderPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	recorder := workspacetasks.AttemptRecorder(env.tasks)

	start := workspacetasks.AttemptStart{
		TaskID:         "task_pgattempt1",
		ExecutionToken: "exec_pgattempt_gateway",
		Service:        "render",
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     workspacetasks.AttemptRetryLayerGateway,
		StartedAt:      time.Now(),
	}
	attemptNo, err := recorder.StartAttempt(ctx, start)
	if err != nil {
		t.Fatalf("StartAttempt: %v", err)
	}
	if attemptNo != 1 {
		t.Fatalf("attempt_no = %d, want 1 (server-assigned)", attemptNo)
	}
	if consumed, err := recorder.HasModelFallbackAttempt(ctx, start.TaskID); err != nil || consumed {
		t.Fatalf("hasFallback = %v/%v, want false/nil before any fallback", consumed, err)
	}

	fallbackNo, err := recorder.StartAttempt(ctx, workspacetasks.AttemptStart{
		TaskID:         start.TaskID,
		ExecutionToken: "exec_pgattempt_fallback",
		Service:        "render",
		RequestedModel: "openrouter/model-b",
		ResolvedModel:  "openrouter/model-b",
		Provider:       "openrouter",
		RetryLayer:     workspacetasks.AttemptRetryLayerModelFallback,
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartAttempt fallback: %v", err)
	}
	if fallbackNo != 2 {
		t.Fatalf("fallback attempt_no = %d, want 2", fallbackNo)
	}
	if consumed, err := recorder.HasModelFallbackAttempt(ctx, start.TaskID); err != nil || !consumed {
		t.Fatalf("hasFallback = %v/%v, want true/nil after the fallback row", consumed, err)
	}

	// Duplicate execution tokens violate the uniqueness fence.
	if _, err := recorder.StartAttempt(ctx, start); err == nil {
		t.Fatal("duplicate execution token started, want uniqueness error")
	}

	rows, err := recorder.FinishAttempt(ctx, workspacetasks.AttemptFinish{
		ExecutionToken:  start.ExecutionToken,
		TaskID:          start.TaskID,
		Status:          workspacetasks.AttemptStatusFailed,
		ResolvedModel:   start.ResolvedModel,
		FailureStage:    "provider",
		FailureCategory: "provider_error",
		ErrorMsg:        "upstream says no",
		FinishedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("FinishAttempt: %v", err)
	}
	if rows != 1 {
		t.Fatalf("finished rows = %d, want 1", rows)
	}
	// Closing twice matches nothing: idempotent, never an error.
	rows, err = recorder.FinishAttempt(ctx, workspacetasks.AttemptFinish{
		ExecutionToken: start.ExecutionToken,
		TaskID:         start.TaskID,
		Status:         workspacetasks.AttemptStatusFailed,
		FinishedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("second FinishAttempt: %v", err)
	}
	if rows != 0 {
		t.Fatalf("second finish rows = %d, want 0", rows)
	}
	// Unknown tokens match nothing either.
	rows, err = recorder.FinishAttempt(ctx, workspacetasks.AttemptFinish{
		ExecutionToken: "exec_missing",
		TaskID:         start.TaskID,
		Status:         workspacetasks.AttemptStatusFailed,
		FinishedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("unknown FinishAttempt: %v", err)
	}
	if rows != 0 {
		t.Fatalf("unknown finish rows = %d, want 0", rows)
	}
	// Invalid layers and statuses fail at the CHECK, never silently.
	if _, err := recorder.StartAttempt(ctx, workspacetasks.AttemptStart{
		TaskID: "task_pgattempt1", ExecutionToken: "exec_bad_layer",
		RetryLayer: "anything", Service: "render",
	}); err == nil {
		t.Fatal("invalid retry_layer started, want CHECK error")
	}
	if _, err := recorder.FinishAttempt(ctx, workspacetasks.AttemptFinish{
		ExecutionToken: "exec_pgattempt_fallback", TaskID: start.TaskID,
		Status: "bogus", FinishedAt: time.Now(),
	}); err == nil {
		t.Fatal("bogus status finished, want validation error")
	}
}
