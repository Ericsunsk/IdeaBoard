package taskruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/runtime/queuetypes"
	"api/internal/runtime/taskqueue"
	"api/internal/tasks"
	"gorm.io/gorm"
)

const defaultTaskRetryLimit = 3

var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrTaskTerminal       = errors.New("task already in terminal state")
	ErrTaskStateConflict  = errors.New("task state conflict")
	ErrTaskAlreadyRunning = errors.New("task already running on same account")
)

type EventHub interface {
	Publish(taskID string, event accounts.TaskEvent)
}

type Service struct {
	accountRepo *accounts.AccountRepository
	taskRepo    *accounts.TaskRepository
	queue       taskqueue.TaskAssigner
	cancelStore taskqueue.TaskCancellationStore
	hub         EventHub
}

func NewService(
	accountRepo *accounts.AccountRepository,
	taskRepo *accounts.TaskRepository,
	assigner taskqueue.TaskAssigner,
	cancelStore taskqueue.TaskCancellationStore,
) *Service {
	return &Service{
		accountRepo: accountRepo,
		taskRepo:    taskRepo,
		queue:       assigner,
		cancelStore: cancelStore,
	}
}

// SetEventHub injects the event hub for SSE task event streaming.
func (s *Service) SetEventHub(hub EventHub) {
	s.hub = hub
}

func (s *Service) RefreshTaskAccounts(ctx context.Context, task *queuetypes.TaskPayload) error {
	enabledAccounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return err
	}
	featureSlug := accounts.ParseTaskFeatureSlug(task.Params)
	enabledAccounts = accounts.FilterAccountsByFeature(enabledAccounts, featureSlug)

	hasScopedCandidates := task.AccountIDs != nil
	scoped := make(map[uint64]struct{}, len(task.AccountIDs))
	for _, id := range task.AccountIDs {
		scoped[id] = struct{}{}
	}

	next := make([]uint64, 0, len(enabledAccounts))
	for _, account := range enabledAccounts {
		if hasScopedCandidates {
			if _, ok := scoped[account.ID]; !ok {
				continue
			}
		}
		maxConcurrent := accounts.NormalizeMaxConcurrent(account.MaxConcurrent)
		if err := s.queue.EnsureAccountConcurrent(ctx, account.ID, maxConcurrent); err != nil {
			return err
		}
		next = append(next, account.ID)
	}
	task.AccountIDs = next
	return nil
}

func (s *Service) ReleaseTask(ctx context.Context, accountID uint64, taskID string) error {
	if accountID == 0 {
		return nil
	}
	return s.queue.Release(ctx, accountID, taskID)
}

func (s *Service) IsTaskCancelled(ctx context.Context, taskID string) (bool, error) {
	return s.cancelStore.IsCancelled(ctx, taskID)
}

func (s *Service) ClearTaskCancelled(ctx context.Context, taskID string) error {
	return s.cancelStore.ClearCancelled(ctx, taskID)
}

func (s *Service) WatchTaskCancellation(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cancelled, err := s.IsTaskCancelled(ctx, taskID)
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

func (s *Service) publishTaskStatusEvent(taskID, status string) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(taskID, accounts.TaskEvent{TaskID: taskID, Status: status})
}

func (s *Service) MarkTaskRunning(ctx context.Context, taskID string, accountID uint64) error {
	_ = ctx
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
		s.publishTaskStatusEvent(taskID, tasks.StatusRunning)
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
		return ErrTaskAlreadyRunning
	}
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *Service) MarkTaskCompleted(ctx context.Context, taskID string, result string) error {
	_ = ctx
	now := time.Now()
	outputs, err := taskOutputsFromResult(taskID, result)
	if err != nil {
		return err
	}
	rows, err := s.taskRepo.CompleteWithOutputs(taskID, []string{tasks.StatusRunning}, map[string]interface{}{
		"status":       tasks.StatusCompleted,
		"result":       result,
		"error_msg":    "",
		"completed_at": now,
	}, outputs)
	if err != nil {
		return err
	}
	if rows > 0 {
		if task, getErr := s.taskRepo.GetByTaskID(taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(task); statErr != nil {
				log.Printf("[TaskRuntime] upsert daily stat failed after complete, task_id=%s err=%v", taskID, statErr)
			}
		} else {
			log.Printf("[TaskRuntime] load task for daily stat failed after complete, task_id=%s err=%v", taskID, getErr)
		}
		s.publishTaskStatusEvent(taskID, tasks.StatusCompleted)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *Service) MarkTaskFailed(ctx context.Context, taskID string, reason string) error {
	_ = ctx
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
		if task, getErr := s.taskRepo.GetByTaskID(taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(task); statErr != nil {
				log.Printf("[TaskRuntime] upsert daily stat failed after fail, task_id=%s err=%v", taskID, statErr)
			}
		} else {
			log.Printf("[TaskRuntime] load task for daily stat failed after fail, task_id=%s err=%v", taskID, getErr)
		}
		s.publishTaskStatusEvent(taskID, tasks.StatusFailed)
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

func (s *Service) ResetTaskRunningToPending(ctx context.Context, taskID string, accountID uint64, reason string) error {
	_ = ctx
	if accountID == 0 {
		return ErrTaskStateConflict
	}

	updates := map[string]interface{}{
		"status":     tasks.StatusPending,
		"account_id": 0,
	}
	if reason != "" {
		updates["error_msg"] = reason
	}

	rows, err := s.taskRepo.UpdateByTaskIDAndStatuses(taskID, []string{tasks.StatusRunning}, updates)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(taskID, tasks.StatusPending)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if task.Status == tasks.StatusPending {
		return nil
	}
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *Service) RequeueTaskForFailover(ctx context.Context, taskID string, accountID uint64, errMsg string) (bool, error) {
	_ = ctx
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
	if tasks.IsTerminalStatus(task.Status) {
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
			s.publishTaskStatusEvent(taskID, tasks.StatusPending)
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
		if task, getErr := s.taskRepo.GetByTaskID(taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(task); statErr != nil {
				log.Printf("[TaskRuntime] upsert daily stat failed after failover-final-fail, task_id=%s err=%v", taskID, statErr)
			}
		} else {
			log.Printf("[TaskRuntime] load task for daily stat failed after failover-final-fail, task_id=%s err=%v", taskID, getErr)
		}
		s.publishTaskStatusEvent(taskID, tasks.StatusFailed)
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
	if tasks.IsTerminalStatus(latest.Status) {
		return false, ErrTaskTerminal
	}
	return false, ErrTaskStateConflict
}

func (s *Service) RecordAccountFailure(ctx context.Context, accountID uint64) (disabled bool, failCount int, err error) {
	_ = ctx
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

	if account.FailCount >= accounts.MaxFailCount && account.Status != 0 {
		if err := s.accountRepo.UpdateStatus(accountID, 0); err != nil {
			return false, account.FailCount, err
		}
		return true, account.FailCount, nil
	}

	return false, account.FailCount, nil
}

func (s *Service) ResetAccountFailure(ctx context.Context, accountID uint64) error {
	_ = ctx
	if accountID == 0 {
		return nil
	}
	return s.accountRepo.ResetFailCount(accountID)
}

func taskOutputsFromResult(taskID string, rawResult string) ([]tasks.TaskOutput, error) {
	rawResult = strings.TrimSpace(rawResult)
	if rawResult == "" {
		return nil, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(rawResult), &payload); err != nil {
		return nil, nil
	}

	urls := resultImageURLs(payload)
	outputs := make([]tasks.TaskOutput, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, rawURL := range urls {
		imageURL := strings.TrimSpace(rawURL)
		if imageURL == "" {
			continue
		}
		if _, ok := seen[imageURL]; ok {
			continue
		}
		seen[imageURL] = struct{}{}
		if isInlineImageDataURL(imageURL) {
			return nil, fmt.Errorf("任务结果包含未持久化的内联图片")
		}
		outputs = append(outputs, tasks.TaskOutput{
			TaskID:     taskID,
			Role:       "image",
			URL:        imageURL,
			StorageKey: outputStorageKey(imageURL),
			SortIndex:  len(outputs),
		})
	}
	return outputs, nil
}

func resultImageURLs(payload map[string]interface{}) []string {
	urls := make([]string, 0, 4)
	if raw, ok := payload["image_urls"]; ok {
		switch values := raw.(type) {
		case []interface{}:
			for _, value := range values {
				if imageURL, ok := value.(string); ok {
					urls = append(urls, imageURL)
				}
			}
		case []string:
			urls = append(urls, values...)
		}
	}
	if raw, ok := payload["image_url"].(string); ok {
		urls = append(urls, raw)
	}
	return urls
}

func isInlineImageDataURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/")
}

func outputStorageKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "/uploads/") {
		return strings.TrimPrefix(trimmed, "/")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return ""
	}
	path := strings.TrimSpace(parsed.Path)
	index := strings.Index(path, "/uploads/")
	if index < 0 {
		return ""
	}
	return strings.TrimPrefix(path[index:], "/")
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
