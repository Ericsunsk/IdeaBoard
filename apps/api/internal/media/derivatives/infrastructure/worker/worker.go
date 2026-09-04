package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"ideaboard/api/internal/media/derivatives/application/processing"
	"ideaboard/api/internal/media/derivatives/domain"
)

const QueueName = "media_derivatives"

type JobArgs struct {
	DerivativeID int64 `json:"derivative_id"`
}

func (JobArgs) Kind() string { return "media_derivative" }

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *processing.Service
}

func NewWorker(service *processing.Service) *Worker {
	return &Worker{service: service}
}

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.DerivativeID <= 0 {
		return context.Canceled
	}
	err := w.service.Process(ctx, job.Args.DerivativeID)
	var recorded *domain.FailureRecordedError
	if errors.As(err, &recorded) {
		slog.Warn("media derivative processing failed", "derivative_id", job.Args.DerivativeID, "err", err)
		return nil
	}
	return err
}
