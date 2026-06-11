package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/runtime/taskqueue"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestRiverWorkerNoCapacitySnoozesJob(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusNoCapacity}
	runtime := &fakeRuntime{}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, taskqueue.TaskAssignment) error {
		t.Fatal("execute should not be called without capacity")
		return nil
	}))

	err := worker.Work(context.Background(), &river.Job[taskqueue.DrawingTaskArgs]{
		Args: taskqueue.DrawingTaskArgs{Task: queuetypes.TaskPayload{TaskID: "cap-1", AccountIDs: []int64{1}}},
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

func TestRiverFinalFailureHandlerConvergesOnlyFinalDrawingAttempts(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := newRiverFinalFailureHandler(runtime)
	if handler == nil {
		t.Fatal("expected final failure handler")
	}
	encodedArgs, err := json.Marshal(taskqueue.DrawingTaskArgs{
		Task: queuetypes.TaskPayload{TaskID: "final-task"},
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
	if call.reason == "" {
		t.Fatal("converged reason should not be empty")
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
	processor := newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, taskqueue.TaskAssignment) error {
		time.Sleep(80 * time.Millisecond)
		executed = true
		return nil
	})

	start := time.Now()
	err := processor.Process(context.Background(), &queuetypes.TaskPayload{TaskID: "sync-1", AccountIDs: []int64{9}})
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
	processor := newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, taskqueue.TaskAssignment) error {
		t.Fatal("execute should not be called for discarded assignment")
		return nil
	})

	if err := processor.Process(context.Background(), &queuetypes.TaskPayload{TaskID: "discard-1", AccountIDs: []int64{1}}); err != nil {
		t.Fatalf("process discard task failed: %v", err)
	}
}

func TestRiverWorkerPassesRiverJobIDToProcessor(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	worker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, taskqueue.TaskAssignment) error {
		return nil
	}))

	err := worker.Work(context.Background(), &river.Job[taskqueue.DrawingTaskArgs]{
		JobRow: &rivertype.JobRow{ID: 12345},
		Args:   taskqueue.DrawingTaskArgs{Task: queuetypes.TaskPayload{TaskID: "job-id-1", AccountIDs: []int64{9}}},
	})
	if err != nil {
		t.Fatalf("Work failed: %v", err)
	}
	if assigner.lastRiverJobID != 12345 {
		t.Fatalf("RiverJobID=%d, want 12345", assigner.lastRiverJobID)
	}
}
