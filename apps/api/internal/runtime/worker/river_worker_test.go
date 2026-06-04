package worker

import (
	"context"
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
