package outbox

import (
	eventoutbox "ideaboard/api/internal/events/outbox"

	"github.com/riverqueue/river"
)

type boardAccessOutboxArgs struct{}

func (boardAccessOutboxArgs) Kind() string { return "board_access_event_outbox_replay" }

// BoardAccessWorker replays committed directed access signals whose live
// emit never completed (crash between commit and emit, NATS publish
// failure). Rows keep their deterministic evt_ id, so a replay converges
// with the live emit instead of duplicating; webhooks are never a sink.
// Shared template: see worker.go (same queue/budget/period as the
// task-status sweeper; only kind/sinks/unavailable differ).
type BoardAccessWorker = OutboxWorker[boardAccessOutboxArgs]

func NewBoardAccessWorker(replayer *eventoutbox.BoardAccessSignals) *BoardAccessWorker {
	var r OutboxReplayer
	if replayer != nil {
		r = replayer
	}
	return NewOutboxWorker[boardAccessOutboxArgs](r, eventoutbox.ErrBoardAccessOutboxUnavailable)
}

// BoardAccessPeriodicJobs sweeps pending directed signals on the shared
// maintenance queue (same precedent as the task-status outbox sweeper:
// serial, low priority; the outbox row is the retry fact, never the task
// queue).
func BoardAccessPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{newOutboxPeriodicJob(boardAccessOutboxArgs{}, "board-access-event-outbox-replay")}
}
