package taskqueue

import (
	"api/internal/runtime/queuetypes"
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

type fakeAsynqClient struct {
	lastTask *asynq.Task
}

func (f *fakeAsynqClient) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.lastTask = task
	return &asynq.TaskInfo{}, nil
}

func (f *fakeAsynqClient) Close() error { return nil }

type fakeAsynqInspector struct {
	queueInfo *asynq.QueueInfo
	err       error
}

func (f *fakeAsynqInspector) GetQueueInfo(_ string) (*asynq.QueueInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.queueInfo, nil
}

func (f *fakeAsynqInspector) Close() error { return nil }

func TestAsynqQueueEnqueueUsesDrawingTaskType(t *testing.T) {
	client := &fakeAsynqClient{}
	inspector := &fakeAsynqInspector{queueInfo: &asynq.QueueInfo{}}
	q := NewAsynqQueueWithClients(client, inspector, nil, AsynqQueueDefault, 5, 30*time.Second, time.Hour)

	task := &queuetypes.TaskPayload{TaskID: "t1", UserID: 1, Prompt: "p", Params: "{}", AccountIDs: []uint64{1}, CreatedAt: 1}
	if err := q.Enqueue(context.Background(), task); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if client.lastTask == nil {
		t.Fatal("expected enqueue to submit task")
	}
	if client.lastTask.Type() != AsynqTaskTypeDrawingGenerate {
		t.Fatalf("task type=%q, want %q", client.lastTask.Type(), AsynqTaskTypeDrawingGenerate)
	}
	decoded, err := DecodeAsynqPayload(client.lastTask.Payload())
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if decoded.TaskID != "t1" {
		t.Fatalf("decoded task id=%q, want t1", decoded.TaskID)
	}
}

func TestAsynqQueueQueueLengthsUsesInspector(t *testing.T) {
	client := &fakeAsynqClient{}
	inspector := &fakeAsynqInspector{queueInfo: &asynq.QueueInfo{Pending: 3, Scheduled: 2, Retry: 1, Active: 4}}
	q := NewAsynqQueueWithClients(client, inspector, nil, AsynqQueueDefault, 5, 30*time.Second, time.Hour)

	global, wait, err := q.QueueLengths(context.Background())
	if err != nil {
		t.Fatalf("QueueLengths failed: %v", err)
	}
	if global != 6 || wait != 4 {
		t.Fatalf("unexpected queue lengths global=%d wait=%d", global, wait)
	}
}

func TestAsynqQueueNoCapacityPayloadValidationError(t *testing.T) {
	client := &fakeAsynqClient{}
	inspector := &fakeAsynqInspector{queueInfo: &asynq.QueueInfo{}}
	q := NewAsynqQueueWithClients(client, inspector, nil, AsynqQueueDefault, 5, 30*time.Second, time.Hour)

	err := q.Enqueue(context.Background(), &queuetypes.TaskPayload{TaskID: ""})
	if err == nil {
		t.Fatal("expected enqueue error for empty task id")
	}
	if client.lastTask != nil {
		t.Fatal("enqueue should fail before calling client")
	}
}
