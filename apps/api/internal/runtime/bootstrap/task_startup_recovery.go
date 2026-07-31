package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/tasking/taskstore"

	"github.com/riverqueue/river/rivertype"
)

const startupRecoveryFinalFailureReason = "服务重启，任务已停止"

type startupRecoveryTaskStore interface {
	ListRunningTasksForStartupRecovery(ctx context.Context) ([]taskstore.StartupRecoveredTask, error)
}

type startupRecoveryFinalFailures interface {
	ConvergeFinalFailureDurably(ctx context.Context, taskID string, riverJobID int64, reason string) error
}

type startupRecoveryRiverJobCanceller interface {
	CancelJob(ctx context.Context, jobID int64) error
}

func recoverRunningTasksAfterStartup(
	ctx context.Context,
	taskStore startupRecoveryTaskStore,
	finalFailures startupRecoveryFinalFailures,
	riverJobs startupRecoveryRiverJobCanceller,
) (int, error) {
	if taskStore == nil {
		return 0, fmt.Errorf("startup recovery task store is nil")
	}
	recoveredTasks, err := taskStore.ListRunningTasksForStartupRecovery(ctx)
	if err != nil {
		return 0, fmt.Errorf("list running tasks for startup recovery: %w", err)
	}
	if len(recoveredTasks) == 0 {
		return 0, nil
	}
	if finalFailures == nil {
		return 0, fmt.Errorf("startup recovery final failure orchestrator is nil")
	}

	durableTasks := make([]taskstore.StartupRecoveredTask, 0, len(recoveredTasks))
	var recoveryErrs []error
	for _, task := range recoveredTasks {
		if err := finalFailures.ConvergeFinalFailureDurably(ctx, task.TaskID, task.RiverJobID, startupRecoveryFinalFailureReason); err != nil {
			recoveryErrs = append(recoveryErrs, fmt.Errorf("converge startup task %s: %w", task.TaskID, err))
			continue
		}
		durableTasks = append(durableTasks, task)
	}
	cancelledRiverJobs, cancelErr := cancelRecoveredRiverJobs(ctx, riverJobs, durableTasks)
	if cancelErr != nil {
		slog.Warn("task startup River cleanup incomplete", "err", cancelErr)
	}
	slog.Info("task startup recovery completed",
		"running_found", len(recoveredTasks),
		"durably_converged", len(durableTasks),
		"cancelled_river_jobs", cancelledRiverJobs,
	)
	return len(durableTasks), errors.Join(recoveryErrs...)
}

func cancelRecoveredRiverJobs(ctx context.Context, canceller startupRecoveryRiverJobCanceller, recoveredTasks []taskstore.StartupRecoveredTask) (int, error) {
	if len(recoveredTasks) == 0 {
		return 0, nil
	}
	if canceller == nil {
		return 0, fmt.Errorf("startup recovery River job canceller is nil")
	}
	cancelled := 0
	var cancelErrs []error
	for _, task := range recoveredTasks {
		if task.RiverJobID <= 0 {
			cancelErrs = append(cancelErrs, fmt.Errorf("task %s has invalid River job ID", task.TaskID))
			continue
		}
		if err := canceller.CancelJob(ctx, task.RiverJobID); err != nil {
			if errors.Is(err, rivertype.ErrNotFound) {
				continue
			}
			cancelErrs = append(cancelErrs, fmt.Errorf("cancel recovered River job %d for task %s: %w", task.RiverJobID, task.TaskID, err))
			continue
		}
		cancelled++
	}
	return cancelled, errors.Join(cancelErrs...)
}
