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

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/tasks"
)

const (
	defaultTaskRetryLimit = 3
)

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskTerminal      = errors.New("task already in terminal state")
	ErrTaskStateConflict = errors.New("task state conflict")
)

type AccountStore interface {
	GetAllEnabled(ctx context.Context) ([]*accounts.ApiAccount, error)
	GetByID(ctx context.Context, id int64) (*accounts.ApiAccount, error)
	IncrementFailCount(ctx context.Context, id int64) error
	UpdateStatus(ctx context.Context, id int64, status int16) error
	ResetFailCount(ctx context.Context, id int64) error
}

type TaskStore interface {
	CompleteWithOutputsAndLeaseToken(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, result string, completedAt time.Time, outputs []tasks.TaskOutput, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	MarkTaskFailed(ctx context.Context, taskID string, statuses []string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	MarkTaskFailedWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	ResetTaskRunningToPendingWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	RequeueTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	FinalFailTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	PatchExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error
	PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error)
	CreateTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error)
	FinishTaskAttempt(ctx context.Context, finish tasks.TaskAttemptFinish) (int64, error)
	MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
}

type Service struct {
	accountRepo AccountStore
	taskRepo    TaskStore
	queue       taskqueue.TaskAssigner
	cancelStore taskqueue.TaskCancellationStore
	taskEvents  events.TaskStatusPublisher
}

func NewService(
	accountRepo AccountStore,
	taskRepo TaskStore,
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
	featureSlug := accounts.ParseFeatureSlugFromParamsJSON(task.Params)
	enabledAccounts = accounts.FilterAccountsForRequestJSON(enabledAccounts, featureSlug, task.Params)

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

func (s *Service) ConfirmTaskLease(ctx context.Context, assignment taskqueue.TaskAssignment) error {
	if s.queue == nil {
		return ErrTaskStateConflict
	}
	if err := s.queue.ConfirmTaskLease(ctx, assignment); err != nil {
		return err
	}
	s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusRunning)
	return nil
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

func (s *Service) MarkTaskCompleted(ctx context.Context, assignment taskqueue.TaskAssignment, result string) error {
	return s.markTaskCompleted(ctx, assignment, result, nil)
}

func (s *Service) MarkTaskCompletedWithAttempt(ctx context.Context, assignment taskqueue.TaskAssignment, result string, finish tasks.TaskAttemptFinish) error {
	return s.markTaskCompleted(ctx, assignment, result, &finish)
}

func (s *Service) markTaskCompleted(ctx context.Context, assignment taskqueue.TaskAssignment, result string, attemptFinish *tasks.TaskAttemptFinish) error {
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(attemptFinish)
	outputs, err := taskOutputsFromResult(assignment.TaskID, result)
	if err != nil {
		return err
	}
	rows, err := s.taskRepo.CompleteWithOutputsAndLeaseToken(ctx, assignment.TaskID, []string{tasks.StatusRunning}, assignment.AccountID, assignment.ChannelID, assignment.LeaseToken, result, now, outputs, attemptFinish)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusCompleted)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
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
	return s.markTaskFailed(ctx, taskID, reason, nil)
}

func (s *Service) MarkTaskFailedWithAttempt(ctx context.Context, taskID string, reason string, finish tasks.TaskAttemptFinish) error {
	return s.markTaskFailed(ctx, taskID, reason, &finish)
}

func (s *Service) MarkTaskFailedWithLease(ctx context.Context, assignment taskqueue.TaskAssignment, reason string) error {
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	return s.markTaskFailedWithAssignment(ctx, assignment, reason, nil)
}

func (s *Service) MarkTaskFailedWithLeaseAndAttempt(ctx context.Context, assignment taskqueue.TaskAssignment, reason string, finish tasks.TaskAttemptFinish) error {
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	return s.markTaskFailedWithAssignment(ctx, assignment, reason, &finish)
}

func (s *Service) ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error {
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
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	if task.Status == tasks.StatusPending {
		return s.MarkTaskFailed(ctx, taskID, reason)
	}
	if task.Status != tasks.StatusRunning {
		return ErrTaskStateConflict
	}

	assignment := taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  tasks.IDValue(task.AccountID),
		ChannelID:  tasks.IDValue(task.AssignedChannelID),
		LeaseToken: task.LeaseToken,
	}
	if !assignment.HasLeaseIdentity() || !taskqueue.LeaseBelongsToRiverJob(task.LeaseToken, riverJobID) {
		return ErrTaskStateConflict
	}
	return s.MarkTaskFailedWithLease(ctx, assignment, reason)
}

func (s *Service) markTaskFailed(ctx context.Context, taskID string, reason string, attemptFinish *tasks.TaskAttemptFinish) error {
	now := taskTerminalTime(attemptFinish)
	rows, err := s.taskRepo.MarkTaskFailed(ctx, taskID, []string{tasks.StatusPending}, reason, formatTaskFailureResult(taskID, reason), now, attemptFinish)
	if err != nil {
		return err
	}
	if rows > 0 {
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

func (s *Service) markTaskFailedWithAssignment(ctx context.Context, assignment taskqueue.TaskAssignment, reason string, attemptFinish *tasks.TaskAttemptFinish) error {
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(attemptFinish)
	rows, err := s.taskRepo.MarkTaskFailedWithLease(ctx, assignment.TaskID, []string{tasks.StatusRunning}, assignment.AccountID, assignment.ChannelID, assignment.LeaseToken, reason, formatTaskFailureResult(assignment.TaskID, reason), now, attemptFinish)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusFailed)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
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

func (s *Service) ResetTaskRunningToPending(ctx context.Context, assignment taskqueue.TaskAssignment, reason string) error {
	return s.resetTaskRunningToPending(ctx, assignment, reason, nil)
}

func (s *Service) ResetTaskRunningToPendingWithAttempt(ctx context.Context, assignment taskqueue.TaskAssignment, reason string, finish tasks.TaskAttemptFinish) error {
	return s.resetTaskRunningToPending(ctx, assignment, reason, &finish)
}

func (s *Service) resetTaskRunningToPending(ctx context.Context, assignment taskqueue.TaskAssignment, reason string, attemptFinish *tasks.TaskAttemptFinish) error {
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}

	errMsg := ""
	if reason != "" {
		errMsg = reason
	}

	rows, err := s.taskRepo.ResetTaskRunningToPendingWithLease(ctx, assignment.TaskID, []string{tasks.StatusRunning}, assignment.AccountID, assignment.ChannelID, assignment.LeaseToken, errMsg, attemptFinish)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusPending)
		return nil
	}

	task, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
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

func (s *Service) RequeueTaskForFailover(ctx context.Context, assignment taskqueue.TaskAssignment, errMsg string) (bool, error) {
	return s.requeueTaskForFailover(ctx, assignment, errMsg, nil, nil)
}

func (s *Service) RequeueTaskForFailoverWithFinalAttempt(ctx context.Context, assignment taskqueue.TaskAssignment, errMsg string, finish tasks.TaskAttemptFinish) (bool, error) {
	return s.requeueTaskForFailover(ctx, assignment, errMsg, nil, &finish)
}

func (s *Service) RequeueTaskForFailoverWithAttempts(ctx context.Context, assignment taskqueue.TaskAssignment, errMsg string, requeuedFinish tasks.TaskAttemptFinish, finalFinish tasks.TaskAttemptFinish) (bool, error) {
	return s.requeueTaskForFailover(ctx, assignment, errMsg, &requeuedFinish, &finalFinish)
}

func (s *Service) requeueTaskForFailover(ctx context.Context, assignment taskqueue.TaskAssignment, errMsg string, requeuedAttemptFinish *tasks.TaskAttemptFinish, finalAttemptFinish *tasks.TaskAttemptFinish) (bool, error) {
	if !assignment.HasLeaseIdentity() {
		return false, ErrTaskStateConflict
	}
	task, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
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
	if tasks.IDValue(task.AccountID) != assignment.AccountID || tasks.IDValue(task.AssignedChannelID) != assignment.ChannelID || task.LeaseToken != assignment.LeaseToken {
		return false, ErrTaskStateConflict
	}

	if task.RetryCount < defaultTaskRetryLimit-1 {
		rows, err := s.taskRepo.RequeueTaskRunningWithLease(ctx, assignment.TaskID, []string{tasks.StatusRunning}, assignment.AccountID, assignment.ChannelID, assignment.LeaseToken, errMsg, requeuedAttemptFinish)
		if err != nil {
			return false, err
		}
		if rows > 0 {
			s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusPending)
			return true, nil
		}
		return false, ErrTaskStateConflict
	}

	now := taskTerminalTime(finalAttemptFinish)
	rows, err := s.taskRepo.FinalFailTaskRunningWithLease(ctx, assignment.TaskID, []string{tasks.StatusRunning}, assignment.AccountID, assignment.ChannelID, assignment.LeaseToken, errMsg, formatTaskFailureResult(assignment.TaskID, errMsg), now, finalAttemptFinish)
	if err != nil {
		return false, err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusFailed)
		return false, nil
	}

	latest, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
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

func taskTerminalTime(attemptFinish *tasks.TaskAttemptFinish) time.Time {
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		return attemptFinish.CompletedAt
	}
	return time.Now()
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
