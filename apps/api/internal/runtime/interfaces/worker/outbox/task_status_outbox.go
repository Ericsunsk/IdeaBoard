package outbox

import (
	eventoutbox "ideaboard/api/internal/events/outbox"

	"github.com/riverqueue/river"
)

type taskStatusOutboxArgs struct{}

func (taskStatusOutboxArgs) Kind() string { return "task_status_event_outbox_replay" }

// Worker replays committed task transition facts. A row remains pending when
// any downstream sink fails; the next run retries with the same evt_ id.
// Shared template: see worker.go (same queue/budget/period as the
// board-access sweeper; only kind/sinks/unavailable differ).
type Worker = OutboxWorker[taskStatusOutboxArgs]

func NewWorker(replayer *eventoutbox.TaskStatusOutboxReplayer) *Worker {
	var r OutboxReplayer
	if replayer != nil {
		r = replayer
	}
	return NewOutboxWorker[taskStatusOutboxArgs](r, eventoutbox.ErrTaskStatusOutboxUnavailable)
}

func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{newOutboxPeriodicJob(taskStatusOutboxArgs{}, "task-status-event-outbox-replay")}
}
