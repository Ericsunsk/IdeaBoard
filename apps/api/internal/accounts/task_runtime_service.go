package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"api/internal/platform/redis"
	"api/internal/tasks"
	"gorm.io/gorm"
)

const defaultTaskRetryLimit = 3

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskTerminal      = errors.New("task already in terminal state")
	ErrTaskStateConflict = errors.New("task state conflict")
)

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
	featureSlug := parseTaskFeatureSlug(task.Params)
	accounts = filterAccountsByFeature(accounts, featureSlug)

	hasScopedCandidates := task.AccountIDs != nil
	scoped := make(map[uint64]struct{}, len(task.AccountIDs))
	for _, id := range task.AccountIDs {
		scoped[id] = struct{}{}
	}

	ctx := context.Background()
	next := make([]uint64, 0, len(accounts))
	for _, account := range accounts {
		if hasScopedCandidates {
			if _, ok := scoped[account.ID]; !ok {
				continue
			}
		}
		maxConcurrent := normalizeMaxConcurrent(account.MaxConcurrent)
		if err := s.redisMgr.EnsureAccountConcurrent(ctx, account.ID, maxConcurrent); err != nil {
			return err
		}
		next = append(next, account.ID)
	}
	task.AccountIDs = next
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
	if accountID == 0 {
		return ErrTaskStateConflict
	}
	rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusPending}, map[string]interface{}{
		"status":     tasks.StatusRunning,
		"account_id": accountID,
	})
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if task.Status == tasks.StatusRunning && task.AccountID == accountID {
		return nil
	}
	if isTaskTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *TaskRuntimeService) MarkTaskCompleted(taskID string, result string) error {
	now := time.Now()
	rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusRunning}, map[string]interface{}{
		"status":       tasks.StatusCompleted,
		"result":       result,
		"error_msg":    "",
		"completed_at": now,
	})
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if isTaskTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *TaskRuntimeService) MarkTaskFailed(taskID string, reason string) error {
	now := time.Now()
	rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusPending, tasks.StatusRunning}, map[string]interface{}{
		"status":       tasks.StatusFailed,
		"error_msg":    reason,
		"result":       formatTaskFailureResult(taskID, reason),
		"completed_at": now,
	})
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if task.Status == tasks.StatusFailed {
		return nil
	}
	if task.Status == tasks.StatusCompleted {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
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

func (s *TaskRuntimeService) RequeueTaskForFailover(taskID string, accountID uint64, errMsg string) (bool, error) {
	if accountID == 0 {
		return false, ErrTaskStateConflict
	}
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrTaskNotFound
		}
		return false, err
	}
	if isTaskTerminalStatus(task.Status) {
		return false, ErrTaskTerminal
	}
	if task.Status != tasks.StatusRunning {
		if task.Status == tasks.StatusPending {
			return true, nil
		}
		return false, ErrTaskStateConflict
	}

	if task.RetryCount < defaultTaskRetryLimit-1 {
		rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusRunning}, map[string]interface{}{
			"status":      tasks.StatusPending,
			"account_id":  0,
			"error_msg":   errMsg,
			"retry_count": gorm.Expr("retry_count + 1"),
		})
		if err != nil {
			return false, err
		}
		if rows > 0 {
			return true, nil
		}
		return false, ErrTaskStateConflict
	}

	now := time.Now()
	rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusRunning}, map[string]interface{}{
		"status":       tasks.StatusFailed,
		"account_id":   0,
		"error_msg":    errMsg,
		"result":       formatTaskFailureResult(taskID, errMsg),
		"retry_count":  gorm.Expr("retry_count + 1"),
		"completed_at": now,
	})
	if err != nil {
		return false, err
	}
	if rows > 0 {
		return false, nil
	}

	latest, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrTaskNotFound
		}
		return false, err
	}
	if latest.Status == tasks.StatusPending {
		return true, nil
	}
	if isTaskTerminalStatus(latest.Status) {
		return false, ErrTaskTerminal
	}
	return false, ErrTaskStateConflict
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

func isTaskTerminalStatus(status string) bool {
	return status == tasks.StatusCompleted || status == tasks.StatusFailed
}
