package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	taskbilling "ideaboard/api/internal/tasking/application/billing"
	tasks "ideaboard/api/internal/tasking/domain"
)

const taskCancelTTL = 24 * time.Hour
const userTaskCancelReason = "用户取消任务"
const adminTaskCancelReason = "管理员取消任务"

var (
	errTaskIntakeUnavailable          = errors.New("task intake not configured")
	errTaskSubmissionStoreUnavailable = errors.New("task submission store not configured")
	errTaskBillingUnavailable         = errors.New("task billing service not configured")
)

// CommandService is the application boundary for task mutations and billing
// readiness checks consumed by HTTP adapters.
type CommandService interface {
	SubmitTask(context.Context, int64, *tasks.SubmitTaskRequest) (string, error)
	CheckSubmitTaskReadiness(context.Context, int64, *tasks.SubmitTaskRequest) (SubmitTaskReadiness, error)
	CancelUserTask(context.Context, string, int64) error
	CancelAdminTask(context.Context, string, int64) error
}

func (s *Service) SubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if s == nil || s.taskIntake == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskIntakeUnavailable)
	}
	canonical, err := s.taskIntake.Compile(ctx, userID, req)
	if err != nil {
		return "", err
	}
	if s.submissionStore == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskSubmissionStoreUnavailable)
	}
	canonical, err = s.taskIntake.PrepareMedia(ctx, userID, canonical)
	if err != nil {
		return "", err
	}
	result, err := s.submissionStore.SubmitTask(ctx, canonical)
	if err != nil {
		slog.Warn("submit task transaction failed", "user_id", userID, "feature", canonical.Feature(), "err", err)
		if errors.Is(err, taskbilling.ErrInsufficientCredits) {
			return "", taskbilling.ErrInsufficientCredits
		}
		if errors.Is(err, taskbilling.ErrPricingUnavailable) {
			return "", taskbilling.ErrPricingUnavailable
		}
		if errors.Is(err, taskbilling.ErrWalletRestricted) {
			return "", taskbilling.ErrWalletRestricted
		}
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", err)
	}

	if result.Created {
		s.publishTaskStatusEvent(ctx, result.TaskID, tasks.StatusPending)
	}
	return result.TaskID, nil
}

type SubmitTaskReadiness struct {
	Feature string
	Billing *taskbilling.Readiness
}

func (s *Service) CheckSubmitTaskReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (SubmitTaskReadiness, error) {
	if s == nil || s.taskIntake == nil {
		return SubmitTaskReadiness{}, newSubmissionError(
			"submission_failed", "任务提交检查暂不可用，请稍后重试", errTaskIntakeUnavailable,
		)
	}
	compilation, err := s.taskIntake.CompileReadiness(ctx, userID, req)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result := SubmitTaskReadiness{Feature: compilation.Feature}
	billingRequest := compilation.BillingRequest
	if billingRequest == nil {
		return result, nil
	}
	if s.taskBilling == nil {
		return SubmitTaskReadiness{}, newSubmissionError(
			"submission_failed",
			"任务提交检查暂不可用，请稍后重试",
			errTaskBillingUnavailable,
		)
	}
	readiness, err := s.taskBilling.Readiness(ctx, userID, *billingRequest)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result.Billing = &readiness
	return result, nil
}

func (s *Service) CancelUserTask(ctx context.Context, taskID string, userID int64) error {
	taskID = strings.TrimSpace(taskID)
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, task, userID, userTaskCancelReason, func() (*tasks.Task, error) {
		return s.taskRepo.GetByTaskIDAndUserID(ctx, task.TaskID, userID)
	})
}

func (s *Service) CancelAdminTask(ctx context.Context, taskID string, adminID int64) error {
	taskID = strings.TrimSpace(taskID)
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, task, adminID, adminTaskCancelReason, func() (*tasks.Task, error) {
		return s.taskRepo.GetByTaskID(ctx, task.TaskID)
	})
}

func (s *Service) cancelTask(
	ctx context.Context,
	task *tasks.Task,
	cancelledBy int64,
	reason string,
	reload func() (*tasks.Task, error),
) error {
	if tasks.IsTerminalStatus(task.Status) {
		return nil
	}
	if s.cancelStore == nil {
		return fmt.Errorf("task cancellation store not configured")
	}
	if err := s.cancelStore.MarkCancelled(ctx, task.TaskID, taskCancelTTL); err != nil {
		return err
	}

	now := time.Now()
	rows, err := s.taskRepo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: now,
		CancelledBy: &cancelledBy,
		Reason:      reason,
	}, nil)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, task.TaskID, tasks.StatusCancelled)
		return nil
	}

	latest, err := reload()
	if err != nil {
		return err
	}
	if tasks.IsTerminalStatus(latest.Status) {
		return nil
	}
	return fmt.Errorf("task cancellation did not apply")
}
