package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"

	"github.com/riverqueue/river/rivertype"
)

const startupRecoveryCancelReason = "服务重启，任务已停止"

type startupRecoveryTaskStore interface {
	CancelRunningTasksForStartupRecovery(ctx context.Context, reason string, cancelledAt time.Time) ([]taskstore.StartupRecoveredTask, error)
}

type startupRecoveryRiverJobCanceller interface {
	CancelJob(ctx context.Context, jobID int64) error
}

func recoverRunningTasksAfterStartup(
	ctx context.Context,
	taskStore startupRecoveryTaskStore,
	riverJobs startupRecoveryRiverJobCanceller,
	publisher events.TaskStatusPublisher,
	recoveredAt time.Time,
) (int, error) {
	if taskStore == nil {
		return 0, fmt.Errorf("startup recovery task store is nil")
	}
	if recoveredAt.IsZero() {
		recoveredAt = time.Now()
	}
	recoveredTasks, err := taskStore.CancelRunningTasksForStartupRecovery(ctx, startupRecoveryCancelReason, recoveredAt)
	if err != nil {
		return 0, fmt.Errorf("cancel running tasks for startup recovery: %w", err)
	}
	cancelledRiverJobs := cancelRecoveredRiverJobs(ctx, riverJobs, recoveredTasks)
	for _, task := range recoveredTasks {
		events.PublishTaskStatusLogWarn(ctx, publisher, task.TaskID, tasks.StatusCancelled)
	}
	slog.Info("task startup recovery completed", "cancelled_running", len(recoveredTasks), "cancelled_river_jobs", cancelledRiverJobs)
	return len(recoveredTasks), nil
}

func cancelRecoveredRiverJobs(ctx context.Context, canceller startupRecoveryRiverJobCanceller, recoveredTasks []taskstore.StartupRecoveredTask) int {
	if canceller == nil || len(recoveredTasks) == 0 {
		return 0
	}
	cancelled := 0
	for _, task := range recoveredTasks {
		if task.RiverJobID <= 0 {
			continue
		}
		if err := canceller.CancelJob(ctx, task.RiverJobID); err != nil {
			if errors.Is(err, rivertype.ErrNotFound) {
				continue
			}
			slog.Warn("cancel recovered River job failed", "task_id", task.TaskID, "river_job_id", task.RiverJobID, "err", err)
			continue
		}
		cancelled++
	}
	return cancelled
}
