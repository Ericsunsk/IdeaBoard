package taskruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/events"
	"api/internal/platform/database"
	"api/internal/runtime/queuetypes"
	"api/internal/runtime/taskqueue"
	"api/internal/tasks"
)

const (
	defaultTaskRetryLimit = 3
)

var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrTaskTerminal       = errors.New("task already in terminal state")
	ErrTaskStateConflict  = errors.New("task state conflict")
	ErrTaskAlreadyRunning = errors.New("task already running on same account")
)

type Service struct {
	accountRepo *accounts.AccountRepository
	taskRepo    *accounts.TaskRepository
	queue       taskqueue.TaskAssigner
	cancelStore taskqueue.TaskCancellationStore
	taskEvents  events.TaskStatusPublisher
}

func NewService(
	accountRepo *accounts.AccountRepository,
	taskRepo *accounts.TaskRepository,
	assigner taskqueue.TaskAssigner,
	cancelStore taskqueue.TaskCancellationStore,
) *Service {
	service := &Service{
		accountRepo: accountRepo,
		taskRepo:    taskRepo,
		queue:       assigner,
		cancelStore: cancelStore,
	}
	return service
}

func (s *Service) SetTaskStatusPublisher(publisher events.TaskStatusPublisher) {
	s.taskEvents = publisher
}

func (s *Service) RefreshTaskAccounts(ctx context.Context, task *queuetypes.TaskPayload) error {
	enabledAccounts, err := s.accountRepo.GetAllEnabled(ctx)
	if err != nil {
		return err
	}
	featureSlug := accounts.ParseTaskFeatureSlug(task.Params)
	enabledAccounts = accounts.FilterAccountsByFeature(enabledAccounts, featureSlug)

	hasScopedCandidates := task.AccountIDs != nil
	scoped := make(map[int64]struct{}, len(task.AccountIDs))
	for _, id := range task.AccountIDs {
		scoped[id] = struct{}{}
	}

	next := make([]int64, 0, len(enabledAccounts))
	for _, account := range enabledAccounts {
		if hasScopedCandidates {
			if _, ok := scoped[account.ID]; !ok {
				continue
			}
		}
		next = append(next, account.ID)
	}
	task.AccountIDs = next
	return nil
}

func (s *Service) ConfirmTaskRunning(ctx context.Context, assignment taskqueue.TaskAssignment) error {
	return s.queue.ConfirmTaskRunning(ctx, assignment)
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

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	if err := events.PublishTaskStatusBestEffort(ctx, s.taskEvents, events.NewTaskStatusEvent(taskID, status)); err != nil {
		slog.Warn("publish task status event failed", "task_id", taskID, "status", status, "err", err)
	}
}

func (s *Service) MarkTaskRunning(ctx context.Context, taskID string, accountID int64, leaseToken string) error {
	if accountID == 0 {
		return ErrTaskStateConflict
	}
	if strings.TrimSpace(leaseToken) == "" {
		return ErrTaskStateConflict
	}
	rows, err := s.taskRepo.MarkTaskRunning(ctx, taskID, []string{tasks.StatusPending}, accountID, leaseToken)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusRunning)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrTaskNotFound
		}
		return err
	}
	if task.Status == tasks.StatusRunning && task.AccountID == accountID {
		if task.LeaseToken == leaseToken {
			return nil
		}
		return ErrTaskAlreadyRunning
	}
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	return ErrTaskStateConflict
}

func (s *Service) MarkTaskCompleted(ctx context.Context, taskID string, leaseToken string, result string) error {
	if strings.TrimSpace(leaseToken) == "" {
		return ErrTaskStateConflict
	}
	now := time.Now()
	outputs, err := taskOutputsFromResult(taskID, result)
	if err != nil {
		return err
	}
	rows, err := s.taskRepo.CompleteWithOutputsAndLeaseToken(ctx, taskID, []string{tasks.StatusRunning}, leaseToken, result, now, outputs)
	if err != nil {
		return err
	}
	if rows > 0 {
		if task, getErr := s.taskRepo.GetByTaskID(ctx, taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(ctx, task); statErr != nil {
				slog.Warn("upsert daily stat failed after complete", "task_id", taskID, "err", statErr)
			}
		} else {
			slog.Warn("load task for daily stat failed after complete", "task_id", taskID, "err", getErr)
		}
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusCompleted)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
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
	return s.markTaskFailed(ctx, taskID, "", false, reason)
}

func (s *Service) MarkTaskFailedWithLease(ctx context.Context, taskID string, leaseToken string, reason string) error {
	if strings.TrimSpace(leaseToken) == "" {
		return ErrTaskStateConflict
	}
	return s.markTaskFailed(ctx, taskID, leaseToken, true, reason)
}

func (s *Service) markTaskFailed(ctx context.Context, taskID string, leaseToken string, requireLeaseToken bool, reason string) error {
	now := time.Now()
	var rows int64
	var err error
	if requireLeaseToken {
		rows, err = s.taskRepo.MarkTaskFailedWithLease(ctx, taskID, []string{tasks.StatusRunning}, leaseToken, reason, formatTaskFailureResult(taskID, reason), now)
	} else {
		rows, err = s.taskRepo.MarkTaskFailed(ctx, taskID, []string{tasks.StatusPending}, reason, formatTaskFailureResult(taskID, reason), now)
	}
	if err != nil {
		return err
	}
	if rows > 0 {
		if task, getErr := s.taskRepo.GetByTaskID(ctx, taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(ctx, task); statErr != nil {
				slog.Warn("upsert daily stat failed after fail", "task_id", taskID, "err", statErr)
			}
		} else {
			slog.Warn("load task for daily stat failed after fail", "task_id", taskID, "err", getErr)
		}
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
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

func (s *Service) ResetTaskRunningToPending(ctx context.Context, taskID string, accountID int64, leaseToken string, reason string) error {
	if accountID == 0 {
		return ErrTaskStateConflict
	}
	if strings.TrimSpace(leaseToken) == "" {
		return ErrTaskStateConflict
	}

	errMsg := ""
	if reason != "" {
		errMsg = reason
	}

	rows, err := s.taskRepo.ResetTaskRunningToPendingWithLease(ctx, taskID, []string{tasks.StatusRunning}, leaseToken, errMsg)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusPending)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
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

func (s *Service) RequeueTaskForFailover(ctx context.Context, taskID string, accountID int64, leaseToken string, errMsg string) (bool, error) {
	if accountID == 0 {
		return false, ErrTaskStateConflict
	}
	if strings.TrimSpace(leaseToken) == "" {
		return false, ErrTaskStateConflict
	}
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
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
	if task.LeaseToken != leaseToken {
		return false, ErrTaskStateConflict
	}

	if task.RetryCount < defaultTaskRetryLimit-1 {
		rows, err := s.taskRepo.RequeueTaskRunningWithLease(ctx, taskID, []string{tasks.StatusRunning}, leaseToken, errMsg)
		if err != nil {
			return false, err
		}
		if rows > 0 {
			s.publishTaskStatusEvent(ctx, taskID, tasks.StatusPending)
			return true, nil
		}
		return false, ErrTaskStateConflict
	}

	now := time.Now()
	rows, err := s.taskRepo.FinalFailTaskRunningWithLease(ctx, taskID, []string{tasks.StatusRunning}, leaseToken, errMsg, formatTaskFailureResult(taskID, errMsg), now)
	if err != nil {
		return false, err
	}
	if rows > 0 {
		if task, getErr := s.taskRepo.GetByTaskID(ctx, taskID); getErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(ctx, task); statErr != nil {
				slog.Warn("upsert daily stat failed after failover final fail", "task_id", taskID, "err", statErr)
			}
		} else {
			slog.Warn("load task for daily stat failed after failover final fail", "task_id", taskID, "err", getErr)
		}
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
		return false, nil
	}

	latest, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
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

func (s *Service) RecordAccountFailure(ctx context.Context, accountID int64) (disabled bool, failCount int, err error) {
	if accountID == 0 {
		return false, 0, nil
	}

	if err := s.accountRepo.IncrementFailCount(ctx, accountID); err != nil {
		return false, 0, err
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return false, 0, err
	}

	if account.FailCount >= accounts.MaxFailCount && account.Status != 0 {
		if err := s.accountRepo.UpdateStatus(ctx, accountID, 0); err != nil {
			return false, account.FailCount, err
		}
		return true, account.FailCount, nil
	}

	return false, account.FailCount, nil
}

func (s *Service) ResetAccountFailure(ctx context.Context, accountID int64) error {
	if accountID == 0 {
		return nil
	}
	return s.accountRepo.ResetFailCount(ctx, accountID)
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
