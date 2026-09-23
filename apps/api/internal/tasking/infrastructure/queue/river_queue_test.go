package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// openLazyPostgresForStats returns a *pgxpool.Pool without pinging, so stats
// validation paths stay unit-testable without a live database.
func openLazyPostgresForStats() (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig("postgres://ideaboard:secret@127.0.0.1:5432/ideaboard?sslmode=disable")
	if err != nil {
		return nil, err
	}
	cfg.MinConns = 0
	cfg.MaxConns = 1
	return pgxpool.NewWithConfig(context.Background(), cfg)
}

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

func TestRiverQueueEnqueueUsesDrawingArgs(t *testing.T) {
	client := &fakeRiverClient{}
	queue := NewRiverQueueWithClient(client, nil, RiverQueueDefault, 7)
	task := &tasks.TaskPayload{TaskID: "task_123", UserID: 42, Priority: tasks.TaskPriorityInteractive}

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
	if client.opts.Priority != tasks.TaskPriorityInteractive {
		t.Fatalf("priority=%d, want %d", client.opts.Priority, tasks.TaskPriorityInteractive)
	}
}

func TestDrawingTaskInsertOptsUsesRiverDefaults(t *testing.T) {
	opts := drawingTaskInsertOpts(&tasks.TaskPayload{TaskID: "task_default"}, "", 0)

	if opts.Queue != RiverQueueDefault {
		t.Fatalf("queue=%q, want %q", opts.Queue, RiverQueueDefault)
	}
	if opts.MaxAttempts != RiverMaxAttemptsDefault {
		t.Fatalf("max attempts=%d, want %d", opts.MaxAttempts, RiverMaxAttemptsDefault)
	}
	if opts.Priority != tasks.TaskPriorityDefault {
		t.Fatalf("priority=%d, want %d", opts.Priority, tasks.TaskPriorityDefault)
	}
}

func TestRiverQueueRejectsMissingTaskID(t *testing.T) {
	client := &fakeRiverClient{}
	queue := NewRiverQueueWithClient(client, nil, RiverQueueDefault, 7)
	if err := queue.Enqueue(context.Background(), &tasks.TaskPayload{}); err == nil {
		t.Fatal("expected error for missing task id")
	}
}

func TestRiverQueueSetClientConcurrentWithEnqueue(t *testing.T) {
	clientA := &concurrentRiverClient{}
	clientB := &concurrentRiverClient{}
	queue := NewRiverQueueWithClient(clientA, nil, RiverQueueDefault, 7)
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
				err := queue.Enqueue(ctx, &tasks.TaskPayload{TaskID: "task_concurrent", UserID: userID})
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

func TestQueueStatsForRejectsEmptyQueueName(t *testing.T) {
	db, err := openLazyPostgresForStats()
	if err != nil {
		t.Skipf("lazy pool unavailable: %v", err)
	}
	defer db.Close()
	queue := NewRiverQueueWithClient(&fakeRiverClient{}, db, RiverQueueDefault, RiverMaxAttemptsDefault)
	if _, _, err := queue.QueueStatsFor(context.Background(), "   "); err == nil {
		t.Fatal("QueueStatsFor with empty queue succeeded, want error")
	}
}

func TestQueueStatsForNilMonitorStaysSilent(t *testing.T) {
	var queue *RiverQueue
	depth, lag, err := queue.QueueStatsFor(context.Background(), RiverQueueDefault)
	if err != nil || depth != 0 || lag != 0 {
		t.Fatalf("nil QueueStatsFor = (%d, %v, %v), want zeros", depth, lag, err)
	}
	if _, _, err := queue.QueueStats(context.Background()); err != nil {
		t.Fatalf("nil QueueStats() error = %v", err)
	}
}
