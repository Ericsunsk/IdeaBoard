package queue

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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
