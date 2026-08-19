package derivatives

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

const QueueName = "media_derivatives"

type JobArgs struct {
	DerivativeID int64 `json:"derivative_id"`
}

func (JobArgs) Kind() string { return "media_derivative" }

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *Service
}

type OutboxDispatcher struct {
	river.WorkerDefaults[DispatchScheduleArgs]
	q *dbsqlc.Queries
}

func NewOutboxDispatcher(db *sql.DB) *OutboxDispatcher {
	return &OutboxDispatcher{q: dbsqlc.New(db)}
}

type DispatchScheduleArgs struct{}

func (DispatchScheduleArgs) Kind() string { return "media_derivative_outbox_schedule" }

func (d *OutboxDispatcher) Work(ctx context.Context, job *river.Job[DispatchScheduleArgs]) error {
	if d == nil || d.q == nil || job == nil {
		return fmt.Errorf("invalid media derivative outbox job")
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("get River client for media derivative outbox: %w", err)
	}
	ids, err := d.q.ListPendingMediaDerivativeIDs(ctx, 100)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := client.Insert(ctx, JobArgs{DerivativeID: id}, &river.InsertOpts{Queue: QueueName, MaxAttempts: 5, Priority: 4, UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
			return err
		}
	}
	return nil
}

func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(30*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			return DispatchScheduleArgs{}, &river.InsertOpts{Queue: QueueName, MaxAttempts: 5, Priority: 4}
		},
		&river.PeriodicJobOpts{ID: "media-derivative-outbox-schedule", RunOnStart: true},
	)}
}

func NewWorker(service *Service) *Worker { return &Worker{service: service} }

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 5 * time.Minute }

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.DerivativeID <= 0 {
		return context.Canceled
	}
	return w.service.Process(ctx, job.Args.DerivativeID)
}
