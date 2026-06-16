package taskqueue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/queuetypes"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type fakeRiverClient struct {
	args river.JobArgs
	opts *river.InsertOpts
}

func (f *fakeRiverClient) Insert(_ context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.args = args
	f.opts = opts
	return &rivertype.JobInsertResult{}, nil
}

type concurrentRiverClient struct {
	inserts atomic.Int64
}

func (c *concurrentRiverClient) Insert(_ context.Context, _ river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	c.inserts.Add(1)
	return &rivertype.JobInsertResult{}, nil
}

func TestRiverPayloadEncodeDecodeRoundTrip(t *testing.T) {
	task := &queuetypes.TaskPayload{
		TaskID:     "task_123",
		UserID:     42,
		Prompt:     "draw",
		Params:     `{"size":"1024x1024"}`,
		AccountIDs: []int64{1, 2},
		RetryCount: 1,
		CreatedAt:  time.Now().Unix(),
	}

	data, err := EncodeRiverPayload(task)
	if err != nil {
		t.Fatalf("EncodeRiverPayload failed: %v", err)
	}
	decoded, err := DecodeRiverPayload(data)
	if err != nil {
		t.Fatalf("DecodeRiverPayload failed: %v", err)
	}
	if decoded.TaskID != task.TaskID || decoded.UserID != task.UserID || len(decoded.AccountIDs) != 2 {
		t.Fatalf("decoded=%+v, want %+v", decoded, task)
	}
}

func TestRiverQueueEnqueueUsesDrawingArgs(t *testing.T) {
	client := &fakeRiverClient{}
	queue := NewRiverQueueWithClient(client, nil, nil, RiverQueueDefault, 7)
	task := &queuetypes.TaskPayload{TaskID: "task_123", UserID: 42}

	if err := queue.Enqueue(context.Background(), task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	args, ok := client.args.(DrawingTaskArgs)
	if !ok {
		t.Fatalf("args type=%T, want DrawingTaskArgs", client.args)
	}
	if args.Kind() != RiverJobKindDrawingGenerate {
		t.Fatalf("kind=%q, want %q", args.Kind(), RiverJobKindDrawingGenerate)
	}
	if args.Task.TaskID != task.TaskID {
		t.Fatalf("task id=%q, want %q", args.Task.TaskID, task.TaskID)
	}
	if client.opts == nil || client.opts.Queue != RiverQueueDefault || client.opts.MaxAttempts != 7 {
		t.Fatalf("opts=%+v", client.opts)
	}
}

func TestRiverQueueRejectsMissingTaskID(t *testing.T) {
	client := &fakeRiverClient{}
	queue := NewRiverQueueWithClient(client, nil, nil, RiverQueueDefault, 7)
	if err := queue.Enqueue(context.Background(), &queuetypes.TaskPayload{}); err == nil {
		t.Fatal("expected error for missing task id")
	}
}

func TestRiverQueueSetClientConcurrentWithEnqueue(t *testing.T) {
	clientA := &concurrentRiverClient{}
	clientB := &concurrentRiverClient{}
	queue := NewRiverQueueWithClient(clientA, nil, nil, RiverQueueDefault, 7)
	ctx := context.Background()

	const (
		enqueueWorkers = 4
		iterations     = 200
	)
	errs := make(chan error, enqueueWorkers)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				queue.SetClient(clientA)
				continue
			}
			queue.SetClient(clientB)
		}
	}()

	for worker := 0; worker < enqueueWorkers; worker++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				err := queue.Enqueue(ctx, &queuetypes.TaskPayload{TaskID: "task_concurrent", UserID: userID})
				if err != nil {
					errs <- err
					return
				}
			}
		}(int64(worker + 1))
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Enqueue failed during concurrent SetClient: %v", err)
		}
	}
	if got := clientA.inserts.Load() + clientB.inserts.Load(); got != enqueueWorkers*iterations {
		t.Fatalf("insert count=%d, want %d", got, enqueueWorkers*iterations)
	}
}

func TestQueuedRiverJobStatesExcludeRunning(t *testing.T) {
	states := queuedRiverJobStates()
	seen := make(map[rivertype.JobState]bool, len(states))
	for _, state := range states {
		seen[state] = true
	}
	for _, state := range []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStateScheduled,
		rivertype.JobStateRetryable,
	} {
		if !seen[state] {
			t.Fatalf("queued states missing %q: %v", state, states)
		}
	}
	if seen[rivertype.JobStateRunning] {
		t.Fatalf("queued states should not include running: %v", states)
	}
}
