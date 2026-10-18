package outbox

// Shared outbox sweeper template (P0-3).
//
// Verification (do not split): both sweepers run on the same maintenance
// queue (riverpolicy.RiverQueueTaskSettlementSweep), the same budget
// (RiverJobTimeoutTaskSettlementSweep / RiverMaxAttemptsTaskSettlementSweep),
// the same 30s period, the same replay limit (100), and the same priority
// (4). Only the River kind, the replayer sink set, and the unavailable
// error differ, so the implementation is one parameterized worker.
//
// River constraint (why kinds stay distinct Go types): river.AddWorker
// registers by zero-value Kind() (Workers.add calls jobArgs.Kind() on a
// fresh T), so a single string-parameterized Args type cannot serve two
// kinds. The kind parameter therefore lives in the type parameter A; the
// queue needs no parameter because both sweepers share it.

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/platform/riverpolicy"

	"github.com/riverqueue/river"
)

// OutboxReplayer is the drain seam both outbox replayers satisfy
// (*eventoutbox.TaskStatusOutboxReplayer and *eventoutbox.BoardAccessSignals).
type OutboxReplayer interface {
	Replay(context.Context, int) error
}

const (
	// OutboxPeriod is the shared sweep interval both workers ran before
	// the merge.
	OutboxPeriod = 30 * time.Second
	// OutboxReplayLimit is the shared per-run row budget.
	OutboxReplayLimit = 100
	// outboxPriority is the shared maintenance-queue priority.
	outboxPriority = 4
)

// OutboxWorker is the single sweeper implementation, parameterized by the
// River job kind A. Use NewOutboxWorker to build one.
type OutboxWorker[A river.JobArgs] struct {
	river.WorkerDefaults[A]
	replayer    OutboxReplayer
	unavailable error
}

// NewOutboxWorker builds a sweeper for kind A sharing the outbox
// queue/budget/period.
func NewOutboxWorker[A river.JobArgs](replayer OutboxReplayer, unavailable error) *OutboxWorker[A] {
	return &OutboxWorker[A]{replayer: replayer, unavailable: unavailable}
}

func (w *OutboxWorker[A]) Timeout(*river.Job[A]) time.Duration {
	return riverpolicy.RiverJobTimeoutTaskSettlementSweep
}

func (w *OutboxWorker[A]) MaxAttempts(*river.Job[A]) int {
	return riverpolicy.RiverMaxAttemptsTaskSettlementSweep
}

func (w *OutboxWorker[A]) Work(ctx context.Context, _ *river.Job[A]) error {
	if w == nil {
		// Unreachable via River (it never calls Work on a nil worker);
		// previously each sweeper returned its own unavailable error
		// here, now generalized.
		return fmt.Errorf("events: outbox worker unavailable")
	}
	if w.replayer == nil {
		return w.unavailable
	}
	return w.replayer.Replay(ctx, OutboxReplayLimit)
}

// outboxSchedule is the single shared sweep ticker both periodic jobs use.
type outboxSchedule struct{}

func (outboxSchedule) Next(current time.Time) time.Time { return current.Add(OutboxPeriod) }

// newOutboxPeriodicJob builds the shared maintenance-queue sweep job: same
// queue, same attempts, same priority, RunOnStart. Only the args value
// (kind) and the periodic ID differ per sweeper.
func newOutboxPeriodicJob(args river.JobArgs, id string) *river.PeriodicJob {
	insert := func() (river.JobArgs, *river.InsertOpts) {
		return args, &river.InsertOpts{Queue: riverpolicy.RiverQueueTaskSettlementSweep, MaxAttempts: riverpolicy.RiverMaxAttemptsTaskSettlementSweep, Priority: outboxPriority}
	}
	return river.NewPeriodicJob(outboxSchedule{}, insert, &river.PeriodicJobOpts{ID: id, RunOnStart: true})
}
