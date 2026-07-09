package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

func TestRecoverRunningTasksAfterStartupCancelsAndPublishesEvents(t *testing.T) {
	ctx := context.Background()
	recoveredAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	store := &fakeStartupRecoveryTaskStore{tasks: []taskstore.StartupRecoveredTask{
		{TaskID: "task-a", RiverJobID: 101},
		{TaskID: "task-b", RiverJobID: 202},
	}}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{}
	publisher := &recordingStartupRecoveryPublisher{}

	count, err := recoverRunningTasksAfterStartup(ctx, store, riverJobs, publisher, recoveredAt)
	if err != nil {
		t.Fatalf("recoverRunningTasksAfterStartup failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
	if store.reason != startupRecoveryCancelReason {
		t.Fatalf("reason=%q, want %q", store.reason, startupRecoveryCancelReason)
	}
	if !store.cancelledAt.Equal(recoveredAt) {
		t.Fatalf("cancelledAt=%v, want %v", store.cancelledAt, recoveredAt)
	}
	if len(riverJobs.jobIDs) != 2 || riverJobs.jobIDs[0] != 101 || riverJobs.jobIDs[1] != 202 {
		t.Fatalf("cancelled river jobs=%v, want [101 202]", riverJobs.jobIDs)
	}
	if len(publisher.events) != 2 {
		t.Fatalf("published events=%d, want 2", len(publisher.events))
	}
	for i, event := range publisher.events {
		if event.TaskID != store.tasks[i].TaskID || event.Status != tasks.StatusCancelled {
			t.Fatalf("event[%d]=%+v, want task_id=%q status=%q", i, event, store.tasks[i].TaskID, tasks.StatusCancelled)
		}
	}
}

func TestRecoverRunningTasksAfterStartupReturnsStoreError(t *testing.T) {
	ctx := context.Background()
	store := &fakeStartupRecoveryTaskStore{err: errors.New("database unavailable")}
	publisher := &recordingStartupRecoveryPublisher{}

	count, err := recoverRunningTasksAfterStartup(ctx, store, nil, publisher, time.Now())
	if err == nil {
		t.Fatal("recoverRunningTasksAfterStartup succeeded, want error")
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0", count)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events=%d, want 0", len(publisher.events))
	}
}

type fakeStartupRecoveryTaskStore struct {
	tasks       []taskstore.StartupRecoveredTask
	reason      string
	cancelledAt time.Time
	err         error
}

func (f *fakeStartupRecoveryTaskStore) CancelRunningTasksForStartupRecovery(_ context.Context, reason string, cancelledAt time.Time) ([]taskstore.StartupRecoveredTask, error) {
	f.reason = reason
	f.cancelledAt = cancelledAt
	if f.err != nil {
		return nil, f.err
	}
	return append([]taskstore.StartupRecoveredTask(nil), f.tasks...), nil
}

type recordingStartupRecoveryRiverJobCanceller struct {
	jobIDs []int64
}

func (c *recordingStartupRecoveryRiverJobCanceller) CancelJob(_ context.Context, jobID int64) error {
	c.jobIDs = append(c.jobIDs, jobID)
	return nil
}

type recordingStartupRecoveryPublisher struct {
	events []events.TaskStatusEvent
}

func (p *recordingStartupRecoveryPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, events.NormalizeTaskStatusEvent(event))
	return nil
}
