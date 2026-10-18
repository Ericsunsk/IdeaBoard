package outbox

import (
	"context"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/riverpolicy"

	"github.com/riverqueue/river"
)

const boardAccessOutboxPeriod = 30 * time.Second

type boardAccessOutboxArgs struct{}

func (boardAccessOutboxArgs) Kind() string { return "board_access_event_outbox_replay" }

// BoardAccessWorker replays committed directed access signals whose live
// emit never completed (crash between commit and emit, NATS publish
// failure). Rows keep their deterministic evt_ id, so a replay converges
// with the live emit instead of duplicating; webhooks are never a sink.
type BoardAccessWorker struct {
	river.WorkerDefaults[boardAccessOutboxArgs]
	Replayer *events.BoardAccessSignals
}

func NewBoardAccessWorker(replayer *events.BoardAccessSignals) *BoardAccessWorker {
	return &BoardAccessWorker{Replayer: replayer}
}

func (*BoardAccessWorker) Timeout(*river.Job[boardAccessOutboxArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutTaskSettlementSweep
}

func (*BoardAccessWorker) MaxAttempts(*river.Job[boardAccessOutboxArgs]) int {
	return riverpolicy.RiverMaxAttemptsTaskSettlementSweep
}

func (w *BoardAccessWorker) Work(ctx context.Context, _ *river.Job[boardAccessOutboxArgs]) error {
	if w == nil || w.Replayer == nil {
		return events.ErrBoardAccessOutboxUnavailable
	}
	return w.Replayer.Replay(ctx, 100)
}

type boardAccessSchedule struct{}

func (boardAccessSchedule) Next(current time.Time) time.Time {
	return current.Add(boardAccessOutboxPeriod)
}

func boardAccessInsert() (river.JobArgs, *river.InsertOpts) {
	return boardAccessOutboxArgs{}, &river.InsertOpts{Queue: riverpolicy.RiverQueueTaskSettlementSweep, MaxAttempts: riverpolicy.RiverMaxAttemptsTaskSettlementSweep, Priority: 4}
}

// BoardAccessPeriodicJobs sweeps pending directed signals on the shared
// maintenance queue (same precedent as the task-status outbox sweeper:
// serial, low priority; the outbox row is the retry fact, never the task
// queue).
func BoardAccessPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(boardAccessSchedule{}, boardAccessInsert, &river.PeriodicJobOpts{ID: "board-access-event-outbox-replay", RunOnStart: true})}
}
