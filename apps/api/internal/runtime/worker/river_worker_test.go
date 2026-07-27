package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestRiverWorkerNoCapacitySnoozesJob(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusNoCapacity}
	runtime := &fakeRuntime{}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		t.Fatal("execute should not be called without capacity")
		return nil
	}))

	err := worker.Work(context.Background(), &river.Job[taskqueue.DrawingTaskArgs]{
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{TaskID: "cap-1"}},
	})
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) {
		t.Fatalf("expected JobSnoozeError, got %v", err)
	}
	if snoozeErr.Duration <= 0 {
		t.Fatalf("expected positive snooze duration, got %s", snoozeErr.Duration)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("no-capacity should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestRiverWorkerRejectsNilJob(t *testing.T) {
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(&fakeAssigner{}, &fakeRuntime{}, nil))

	if err := worker.Work(context.Background(), nil); err == nil {
		t.Fatal("expected nil job error")
	}
}

func TestRiverWorkerPropagatesCorrelationIDToProcessor(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	var gotCorrelationID string
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(_ context.Context, task *tasks.TaskPayload, _ taskqueue.TaskAssignment) error {
		gotCorrelationID = task.EffectiveCorrelationID()
		return nil
	}))

	err := worker.Work(context.Background(), &river.Job[taskqueue.DrawingTaskArgs]{
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID:        "trace-task",
			CorrelationID: "corr-custom",
		}},
	})
	if err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if gotCorrelationID != "corr-custom" {
		t.Fatalf("correlation_id=%q, want corr-custom", gotCorrelationID)
	}
}

func TestRiverFinalFailureHandlerConvergesOnlyFinalDrawingAttempts(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := newRiverFinalFailureHandler(runtime)
	if handler == nil {
		t.Fatal("expected final failure handler")
	}
	encodedArgs, err := json.Marshal(taskqueue.DrawingTaskArgs{
		Task: tasks.TaskPayload{TaskID: "final-task"},
	})
	if err != nil {
		t.Fatalf("marshal args failed: %v", err)
	}

	handler.HandleError(context.Background(), &rivertype.JobRow{
		ID:          9001,
		Kind:        taskqueue.RiverJobKindDrawingGenerate,
		EncodedArgs: encodedArgs,
		Attempt:     1,
		MaxAttempts: 2,
	}, errors.New("retryable"))
	if len(runtime.converged) != 0 {
		t.Fatalf("non-final attempt converged unexpectedly: %#v", runtime.converged)
	}

	handler.HandleError(context.Background(), &rivertype.JobRow{
		ID:          9001,
		Kind:        taskqueue.RiverJobKindDrawingGenerate,
		EncodedArgs: encodedArgs,
		Attempt:     2,
		MaxAttempts: 2,
	}, errors.New("final"))
	if len(runtime.converged) != 1 {
		t.Fatalf("converged calls=%d, want 1", len(runtime.converged))
	}
	call := runtime.converged[0]
	if call.taskID != "final-task" || call.riverJobID != 9001 {
		t.Fatalf("converged call=%#v, want task final-task river 9001", call)
	}
	if call.reason != "任务队列执行失败，请稍后重试" {
		t.Fatalf("converged reason = %q, want safe queue failure message", call.reason)
	}
}

func TestRiverFinalFailureHandlerConvergesPanicWithSafeReason(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := newRiverFinalFailureHandler(runtime)
	if handler == nil {
		t.Fatal("expected final failure handler")
	}
	encodedArgs, err := json.Marshal(taskqueue.DrawingTaskArgs{
		Task: tasks.TaskPayload{TaskID: "panic-task"},
	})
	if err != nil {
		t.Fatalf("marshal args failed: %v", err)
	}

	handler.HandlePanic(context.Background(), &rivertype.JobRow{
		ID:          9003,
		Kind:        taskqueue.RiverJobKindDrawingGenerate,
		EncodedArgs: encodedArgs,
		Attempt:     2,
		MaxAttempts: 2,
	}, "database detail", "stack detail")
	if len(runtime.converged) != 1 {
		t.Fatalf("converged calls=%d, want 1", len(runtime.converged))
	}
	call := runtime.converged[0]
	if call.taskID != "panic-task" || call.riverJobID != 9003 {
		t.Fatalf("converged call=%#v, want task panic-task river 9003", call)
	}
	if call.reason != "任务队列执行异常，请稍后重试" {
		t.Fatalf("converged reason = %q, want safe queue panic message", call.reason)
	}
}

func TestRiverFinalFailureHandlerIgnoresNonDrawingJobs(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := newRiverFinalFailureHandler(runtime)

	handler.HandleError(context.Background(), &rivertype.JobRow{
		ID:          9002,
		Kind:        "other",
		EncodedArgs: []byte(`{"task":{"task_id":"ignored"}}`),
		Attempt:     2,
		MaxAttempts: 2,
	}, errors.New("final"))
	if len(runtime.converged) != 0 {
		t.Fatalf("non-drawing job converged unexpectedly: %#v", runtime.converged)
	}
}

func TestTaskProcessorProcessesSynchronously(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	executed := false
	processor := newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		time.Sleep(80 * time.Millisecond)
		executed = true
		return nil
	})

	start := time.Now()
	err := processor.Process(context.Background(), &tasks.TaskPayload{TaskID: "sync-1"})
	if err != nil {
		t.Fatalf("process task failed: %v", err)
	}
	if !executed {
		t.Fatal("expected execute function to run")
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatalf("processor returned before sync execution completed")
	}
}

func TestTaskProcessorDiscardDoesNotCallProvider(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusDiscard}
	runtime := &fakeRuntime{}
	processor := newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		t.Fatal("execute should not be called for discarded assignment")
		return nil
	})

	if err := processor.Process(context.Background(), &tasks.TaskPayload{TaskID: "discard-1"}); err != nil {
		t.Fatalf("process discard task failed: %v", err)
	}
}

func TestRiverWorkerPassesRiverJobIDToProcessor(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		return nil
	}))

	err := worker.Work(context.Background(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 12345},
		Args:   taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{TaskID: "job-id-1"}},
	})
	if err != nil {
		t.Fatalf("Work failed: %v", err)
	}
	if assigner.lastRiverJobID != 12345 {
		t.Fatalf("RiverJobID=%d, want 12345", assigner.lastRiverJobID)
	}
}

func TestRiverWorkerFinalFailureSnoozesWithDurableConvergenceMarker(t *testing.T) {
	runtime := &fakeRuntime{convergeErr: errors.New("database unavailable")}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			return errors.New("provider failed")
		},
	))
	var recorded any
	worker.recordOutput = func(ctx context.Context, output any) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("marker recorder inherited work cancellation: %v", err)
		}
		recorded = output
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := worker.Work(ctx, &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9004, Attempt: 2, MaxAttempts: 2},
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID: "settlement-retry-task",
		}},
	})
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || snoozeErr.Duration != defaultFinalFailureRetryDelay {
		t.Fatalf("error=%v, want %s settlement snooze", err, defaultFinalFailureRetryDelay)
	}
	retry, ok := recorded.(finalFailureRetryOutput)
	if !ok || retry.Kind != finalFailureRetryOutputKind || retry.TaskID != "settlement-retry-task" || retry.Reason != finalFailureErrorReason {
		t.Fatalf("recorded=%#v, want durable settlement marker", recorded)
	}
}

func TestRiverWorkerFallsBackToDedicatedSettlementJobWhenMarkerRecordingFails(t *testing.T) {
	runtime := &fakeRuntime{convergeErr: errors.New("database unavailable")}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			return errors.New("provider failed")
		},
	))
	recordCalls := 0
	worker.recordOutput = func(context.Context, any) error {
		recordCalls++
		if recordCalls == 1 {
			return nil
		}
		return errors.New("forced marker recording failure")
	}
	var enqueued taskqueue.DrawingFinalFailureArgs
	worker.enqueueFinalFailure = func(_ context.Context, args taskqueue.DrawingFinalFailureArgs) error {
		enqueued = args
		return nil
	}

	err := worker.Work(t.Context(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9010, Attempt: 2, MaxAttempts: 2},
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID: "dedicated-settlement-fallback",
		}},
	})
	if err == nil || err.Error() != "provider failed" {
		t.Fatalf("error=%v, want original provider failure after fallback enqueue", err)
	}
	if enqueued.TaskID != "dedicated-settlement-fallback" || enqueued.RiverJobID != 9010 || enqueued.Reason != finalFailureErrorReason {
		t.Fatalf("enqueued=%+v, want settlement-only fallback", enqueued)
	}
}

func TestRiverWorkerRetriesPreProviderMarkerFailureWithoutCallingProvider(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	providerCalls := 0
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		providerCalls++
		return nil
	}))
	recordCalls := 0
	worker.recordOutput = func(context.Context, any) error {
		recordCalls++
		if recordCalls == 1 {
			return errors.New("forced pre-provider marker failure")
		}
		return nil
	}
	job := &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9013, Attempt: 2, MaxAttempts: 2},
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID: "pre-provider-marker-retry",
		}},
	}

	err := worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || providerCalls != 0 {
		t.Fatalf("first attempt error=%v provider_calls=%d, want snooze/0", err, providerCalls)
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("retry after marker recovery failed: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls=%d, want 1", providerCalls)
	}
}

func TestRiverWorkerConvergesTerminallyWhenMarkerResetFails(t *testing.T) {
	runtime := &fakeRuntime{resetErr: errors.New("forced reset failure")}
	providerCalls := 0
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			providerCalls++
			return nil
		},
	))
	worker.recordOutput = func(context.Context, any) error {
		return errors.New("forced pre-provider marker failure")
	}

	err := worker.Work(t.Context(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9015, Attempt: 2, MaxAttempts: 2},
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID: "marker-reset-terminal-convergence",
		}},
	})
	if err != nil {
		t.Fatalf("terminal convergence after reset failure failed: %v", err)
	}
	if providerCalls != 0 || runtime.resetCalls != 1 || len(runtime.converged) != 1 {
		t.Fatalf("provider/reset/converge=%d/%d/%d, want 0/1/1", providerCalls, runtime.resetCalls, len(runtime.converged))
	}
}

func TestRiverWorkerNoCapacityDoesNotCreateSettlementOnlyMarker(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusNoCapacity}
	runtime := &fakeRuntime{}
	providerCalls := 0
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
		providerCalls++
		return nil
	}))
	recordCalls := 0
	worker.recordOutput = func(context.Context, any) error {
		recordCalls++
		return nil
	}
	job := &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9014, Attempt: 2, MaxAttempts: 2},
		Args: taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{
			TaskID: "no-capacity-final-attempt",
		}},
	}

	err := worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || recordCalls != 0 || providerCalls != 0 || len(runtime.converged) != 0 {
		t.Fatalf("no-capacity error=%v record/provider/converge=%d/%d/%d", err, recordCalls, providerCalls, len(runtime.converged))
	}
	assigner.assignStatus = taskqueue.AssignStatusAssigned
	assigner.assignAccount = 9
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("capacity recovery failed: %v", err)
	}
	if recordCalls != 1 || providerCalls != 1 || len(runtime.converged) != 0 {
		t.Fatalf("after recovery record/provider/converge=%d/%d/%d, want 1/1/0", recordCalls, providerCalls, len(runtime.converged))
	}
}

func TestRiverFinalFailureWorkerOnlyRetriesTerminalConvergence(t *testing.T) {
	runtime := &fakeRuntime{convergeErr: errors.New("database unavailable")}
	worker := &RiverFinalFailureWorker{taskRuntime: runtime, retryDelay: time.Millisecond}
	job := &river.Job[taskqueue.DrawingFinalFailureArgs]{Args: taskqueue.DrawingFinalFailureArgs{
		TaskID: "dedicated-settlement-task", RiverJobID: 9011, Reason: finalFailureErrorReason,
	}}

	err := worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) || snoozeErr.Duration != time.Millisecond {
		t.Fatalf("first error=%v, want settlement snooze", err)
	}
	runtime.convergeErr = nil
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("settlement retry failed: %v", err)
	}
	if len(runtime.converged) != 2 {
		t.Fatalf("convergence calls=%d, want 2", len(runtime.converged))
	}
}

func TestRiverFinalFailureWorkerTreatsAlreadyTerminalOrDeletedTaskAsConverged(t *testing.T) {
	for _, terminalErr := range []error{taskruntime.ErrTaskTerminal, taskruntime.ErrTaskNotFound} {
		runtime := &fakeRuntime{convergeErr: terminalErr}
		worker := &RiverFinalFailureWorker{taskRuntime: runtime}
		err := worker.Work(t.Context(), &river.Job[taskqueue.DrawingFinalFailureArgs]{Args: taskqueue.DrawingFinalFailureArgs{
			TaskID: "already-converged-task", RiverJobID: 9012, Reason: finalFailureErrorReason,
		}})
		if err != nil {
			t.Fatalf("terminal error %v was retried: %v", terminalErr, err)
		}
	}
}

func TestRiverWorkerFinalFailureRetryOnlyConvergesSettlement(t *testing.T) {
	runtime := &fakeRuntime{}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			t.Fatal("provider must not run during settlement-only retry")
			return nil
		},
	))
	metadata, err := json.Marshal(map[string]any{
		"output": finalFailureRetryOutput{
			Kind: finalFailureRetryOutputKind, TaskID: "settlement-only-task", Reason: finalFailureErrorReason,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = worker.Work(t.Context(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9005, Attempt: 2, MaxAttempts: 2, Metadata: metadata},
		Args:   taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{TaskID: "settlement-only-task"}},
	})
	if err != nil {
		t.Fatalf("settlement-only retry failed: %v", err)
	}
	if len(runtime.converged) != 1 || runtime.converged[0].taskID != "settlement-only-task" {
		t.Fatalf("converged=%#v, want settlement-only convergence", runtime.converged)
	}
}

func TestRiverFinalFailureHandlerLeavesSettlementSnoozeToDurableRetry(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := newRiverFinalFailureHandler(runtime)
	encodedArgs, err := json.Marshal(taskqueue.DrawingTaskArgs{
		Task: tasks.TaskPayload{TaskID: "snoozed-settlement-task"},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler.HandleError(t.Context(), &rivertype.JobRow{
		ID:          9008,
		Kind:        taskqueue.RiverJobKindDrawingGenerate,
		EncodedArgs: encodedArgs,
		Attempt:     2,
		MaxAttempts: 2,
	}, river.JobSnooze(time.Second))

	if len(runtime.converged) != 0 {
		t.Fatalf("snoozed settlement converged through ErrorHandler: %#v", runtime.converged)
	}
}

func TestRiverWorkerFinalFailureConvergesAfterWorkContextCancellation(t *testing.T) {
	runtime := &fakeRuntime{convergeFn: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("convergence context inherited cancellation: %v", err)
		}
		return nil
	}}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			return errors.New("job timed out")
		},
	))
	worker.recordOutput = func(context.Context, any) error { return nil }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := worker.Work(ctx, &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9006, Attempt: 2, MaxAttempts: 2},
		Args:   taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{TaskID: "timed-out-task"}},
	})
	if err == nil || err.Error() != "job timed out" {
		t.Fatalf("error=%v, want original terminal error", err)
	}
	if len(runtime.converged) != 1 {
		t.Fatalf("converged calls=%d, want 1", len(runtime.converged))
	}
}

func TestRiverWorkerFinalPanicSnoozesWhenConvergenceFails(t *testing.T) {
	runtime := &fakeRuntime{convergeErr: errors.New("database unavailable")}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(
		&fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9},
		runtime,
		func(context.Context, *tasks.TaskPayload, taskqueue.TaskAssignment) error {
			panic("provider panic")
		},
	))
	var recorded any
	worker.recordOutput = func(_ context.Context, output any) error {
		recorded = output
		return nil
	}

	err := worker.Work(t.Context(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 9007, Attempt: 2, MaxAttempts: 2},
		Args:   taskqueue.DrawingTaskArgs{Task: tasks.TaskPayload{TaskID: "panic-settlement-task"}},
	})
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) {
		t.Fatalf("error=%v, want settlement snooze", err)
	}
	retry, ok := recorded.(finalFailureRetryOutput)
	if !ok || retry.Reason != finalFailurePanicReason {
		t.Fatalf("recorded=%#v, want panic settlement marker", recorded)
	}
}
