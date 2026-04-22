package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api/internal/platform/redis"
	"api/internal/tasks"
)

const defaultTaskRetryLimit = 3

type TaskRuntimeService struct {
	accountRepo *AccountRepository
	taskRepo    *TaskRepository
	redisMgr    *redis.RedisManager
}

func NewTaskRuntimeService(
	accountRepo *AccountRepository,
	taskRepo *TaskRepository,
	redisMgr *redis.RedisManager,
) *TaskRuntimeService {
	return &TaskRuntimeService{
		accountRepo: accountRepo,
		taskRepo:    taskRepo,
		redisMgr:    redisMgr,
	}
}

func (s *TaskRuntimeService) RefreshTaskAccounts(task *redis.GlobalQueueTask) error {
	accounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return err
	}

	ctx := context.Background()
	task.AccountIDs = task.AccountIDs[:0]
	for _, account := range accounts {
		maxConcurrent := normalizeMaxConcurrent(account.MaxConcurrent)
		if err := s.redisMgr.EnsureAccountConcurrent(ctx, account.ID, maxConcurrent); err != nil {
			return err
		}
		task.AccountIDs = append(task.AccountIDs, account.ID)
	}
	return nil
}

func (s *TaskRuntimeService) ReleaseTask(accountID uint64, taskID string) error {
	if accountID == 0 {
		return nil
	}
	return s.redisMgr.ReleaseTask(context.Background(), accountID, taskID)
}

func (s *TaskRuntimeService) IsTaskCancelled(taskID string) (bool, error) {
	return s.redisMgr.IsTaskCancelled(context.Background(), taskID)
}

func (s *TaskRuntimeService) ClearTaskCancelled(taskID string) error {
	return s.redisMgr.ClearTaskCancelled(context.Background(), taskID)
}

func (s *TaskRuntimeService) WatchTaskCancellation(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cancelled, err := s.IsTaskCancelled(taskID)
				if err != nil || !cancelled {
					continue
				}
				cancel()
				return
			}
		}
	}()

	return ctx, cancel
}

func (s *TaskRuntimeService) MarkTaskRunning(taskID string, accountID uint64) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	task.Status = "running"
	task.AccountID = accountID
	return s.taskRepo.Update(task)
}

func (s *TaskRuntimeService) MarkTaskCompleted(taskID string, result string) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	now := time.Now()
	task.Status = "completed"
	task.Result = result
	task.CompletedAt = &now
	return s.taskRepo.Update(task)
}

func (s *TaskRuntimeService) MarkTaskFailed(taskID string, reason string) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}

	now := time.Now()
	task.Status = "failed"
	task.ErrorMsg = reason
	task.Result = formatTaskFailureResult(taskID, reason)
	task.CompletedAt = &now
	return s.taskRepo.Update(task)
}

func (s *TaskRuntimeService) MarkTaskFailedWithRetry(taskID string, accountID uint64, errMsg string) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}

	task.RetryCount++
	task.ErrorMsg = errMsg
	if task.RetryCount >= defaultTaskRetryLimit {
		task.Status = tasks.StatusFailed
		now := time.Now()
		task.CompletedAt = &now
	} else {
		task.Status = tasks.StatusPending
	}

	if accountID > 0 {
		if _, _, err := s.RecordAccountFailure(accountID); err != nil {
			return err
		}
	}

	return s.taskRepo.Update(task)
}

func (s *TaskRuntimeService) RecordAccountFailure(accountID uint64) (disabled bool, failCount int, err error) {
	if accountID == 0 {
		return false, 0, nil
	}

	if err := s.accountRepo.IncrementFailCount(accountID); err != nil {
		return false, 0, err
	}

	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return false, 0, err
	}

	if account.FailCount >= MaxFailCount && account.Status != 0 {
		if err := s.accountRepo.UpdateStatus(accountID, 0); err != nil {
			return false, account.FailCount, err
		}
		return true, account.FailCount, nil
	}

	return false, account.FailCount, nil
}

func (s *TaskRuntimeService) ResetAccountFailure(accountID uint64) error {
	if accountID == 0 {
		return nil
	}
	return s.accountRepo.ResetFailCount(accountID)
}

func formatTaskFailureResult(taskID string, reason string) string {
	result := map[string]string{
		"task_id": taskID,
		"status":  "failed",
		"error":   reason,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"task_id":"%s","status":"failed","error":"任务失败"}`, taskID)
	}
	return string(resultJSON)
}
