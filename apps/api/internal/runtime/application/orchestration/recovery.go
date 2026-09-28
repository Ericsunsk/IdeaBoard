package orchestration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/runtime/ports"

	"github.com/riverqueue/river/rivertype"
)

const StartupRecoveryFinalFailureReason = msgPackage

// StartupRecoveredTask moved here from the deleted legacy taskstore
// (#816 Phase 3b-07): startup recovery coordinates over this row
// projection, and the workspace-scoped store (Phase 4a) will re-supply it.
type StartupRecoveredTask struct {
	TaskID     string
	RiverJobID int64
}

type StartupRecoveryTaskStore interface {
	ListRunningTasksForStartupRecovery(ctx context.Context) ([]StartupRecoveredTask, error)
}

type StartupRecoveryFinalFailures interface {
	ConvergeFinalFailureDurably(ctx context.Context, taskID string, riverJobID int64, reason string) error
}

type StartupRecoveryRiverJobCanceller interface {
	CancelJob(ctx context.Context, jobID int64) error
}

// TaskRecoveryCoordinator coordinates startup recovery of abandoned in-flight tasks.
type TaskRecoveryCoordinator struct {
	taskStore     StartupRecoveryTaskStore
	finalFailures StartupRecoveryFinalFailures
	riverJobs     StartupRecoveryRiverJobCanceller
}

// NewTaskRecoveryCoordinator creates a startup recovery coordinator satisfying ports.StartupRecoveryCoordinator.
func NewTaskRecoveryCoordinator(
	taskStore StartupRecoveryTaskStore,
	finalFailures StartupRecoveryFinalFailures,
	riverJobs StartupRecoveryRiverJobCanceller,
) *TaskRecoveryCoordinator {
	return &TaskRecoveryCoordinator{
		taskStore:     taskStore,
		finalFailures: finalFailures,
		riverJobs:     riverJobs,
	}
}

// RecoverRunningTasks scans for tasks stuck in running state during startup and durably converges them.
func (c *TaskRecoveryCoordinator) RecoverRunningTasks(ctx context.Context) (int, error) {
	if c == nil || c.taskStore == nil {
		return 0, fmt.Errorf("startup recovery task store is nil")
	}
	recoveredTasks, err := c.taskStore.ListRunningTasksForStartupRecovery(ctx)
	if err != nil {
		return 0, fmt.Errorf("list running tasks for startup recovery: %w", err)
	}
	if len(recoveredTasks) == 0 {
		return 0, nil
	}
	if c.finalFailures == nil {
		return 0, fmt.Errorf("startup recovery final failure orchestrator is nil")
	}

	durableTasks := make([]StartupRecoveredTask, 0, len(recoveredTasks))
	var recoveryErrs []error
	for _, task := range recoveredTasks {
		if err := c.finalFailures.ConvergeFinalFailureDurably(ctx, task.TaskID, task.RiverJobID, StartupRecoveryFinalFailureReason); err != nil {
			recoveryErrs = append(recoveryErrs, fmt.Errorf("converge startup task %s: %w", task.TaskID, err))
			continue
		}
		durableTasks = append(durableTasks, task)
	}
	cancelledRiverJobs, cancelErr := cancelRecoveredRiverJobs(ctx, c.riverJobs, durableTasks)
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

var _ ports.StartupRecoveryCoordinator = (*TaskRecoveryCoordinator)(nil)

func cancelRecoveredRiverJobs(ctx context.Context, canceller StartupRecoveryRiverJobCanceller, recoveredTasks []StartupRecoveredTask) (int, error) {
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
