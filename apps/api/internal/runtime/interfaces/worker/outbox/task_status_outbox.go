package outbox

import (
	"context"
	"time"

	"ideaboard/api/internal/events"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"

	"github.com/riverqueue/river"
)

const taskStatusOutboxPeriod = 30 * time.Second

type taskStatusOutboxArgs struct{}

func (taskStatusOutboxArgs) Kind() string { return "task_status_event_outbox_replay" }

// Worker replays committed task transition facts. A row remains pending when
// any downstream sink fails; the next run retries with the same evt_ id.
type Worker struct {
	river.WorkerDefaults[taskStatusOutboxArgs]
	Replayer *events.TaskStatusOutboxReplayer
}

func NewWorker(replayer *events.TaskStatusOutboxReplayer) *Worker { return &Worker{Replayer: replayer} }

func (*Worker) Timeout(*river.Job[taskStatusOutboxArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutTaskSettlementSweep
}

func (*Worker) MaxAttempts(*river.Job[taskStatusOutboxArgs]) int {
	return taskqueue.RiverMaxAttemptsTaskSettlementSweep
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[taskStatusOutboxArgs]) error {
	if w == nil || w.Replayer == nil {
		return events.ErrTaskStatusOutboxUnavailable
	}
	return w.Replayer.Replay(ctx, 100)
}

type schedule struct{}

func (schedule) Next(current time.Time) time.Time { return current.Add(taskStatusOutboxPeriod) }

func insert() (river.JobArgs, *river.InsertOpts) {
	return taskStatusOutboxArgs{}, &river.InsertOpts{Queue: taskqueue.RiverQueueTaskSettlementSweep, MaxAttempts: taskqueue.RiverMaxAttemptsTaskSettlementSweep, Priority: 4}
}

func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(schedule{}, insert, &river.PeriodicJobOpts{ID: "task-status-event-outbox-replay", RunOnStart: true})}
}
