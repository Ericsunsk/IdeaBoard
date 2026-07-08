package taskruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

const (
	defaultTaskRetryLimit        = 3
	taskCancellationPollInterval = 2 * time.Second
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
	CompleteWithOutputsAndLeaseToken(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, result string, completedAt time.Time, outputs []tasks.TaskOutput, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	MarkTaskFailed(ctx context.Context, taskID string, statuses []string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	MarkTaskFailedWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	ResetTaskRunningToPendingWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	RequeueTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	FinalFailTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetActiveTaskLeaseByTaskID(ctx context.Context, taskID string) (*taskstore.ActiveTaskLease, error)
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

type CompleteTaskInput struct {
	Assignment    taskqueue.TaskAssignment
	Result        string
	Outputs       []tasks.TaskOutput
	AttemptFinish *tasks.TaskAttemptFinish
}

type FailTaskInput struct {
	TaskID        string
	Assignment    taskqueue.TaskAssignment
	Reason        string
	AttemptFinish *tasks.TaskAttemptFinish
}

type ResetRunningTaskInput struct {
	Assignment    taskqueue.TaskAssignment
	Reason        string
	AttemptFinish *tasks.TaskAttemptFinish
}

type RequeueForFailoverInput struct {
	Assignment             taskqueue.TaskAssignment
	Reason                 string
	RequeuedAttemptFinish  *tasks.TaskAttemptFinish
	FinalFailAttemptFinish *tasks.TaskAttemptFinish
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

func (s *Service) RefreshTaskAccounts(ctx context.Context, task *tasks.TaskPayload) error {
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
		if errors.Is(err, taskqueue.ErrTaskLeaseConflict) {
			return fmt.Errorf("%w: %v", ErrTaskStateConflict, err)
		}
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("WatchTaskCancellation goroutine panicked", "task_id", taskID, "panic", r)
			}
		}()
		ticker := time.NewTicker(taskCancellationPollInterval)
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
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}

func (s *Service) CompleteTask(ctx context.Context, input CompleteTaskInput) error {
	assignment := input.Assignment
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(input.AttemptFinish)
	if err := validateTaskOutputs(assignment.TaskID, input.Outputs); err != nil {
		return err
	}
	rows, err := s.taskRepo.CompleteWithOutputsAndLeaseToken(ctx, assignment.TaskID, runningOnly(), assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID, assignment.LeaseToken, input.Result, now, input.Outputs, input.AttemptFinish)
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
		return s.FailTask(ctx, FailTaskInput{TaskID: taskID, Reason: reason})
	}
	if task.Status != tasks.StatusRunning {
		return ErrTaskStateConflict
	}

	lease, err := s.taskRepo.GetActiveTaskLeaseByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrTaskStateConflict
		}
		return err
	}
	assignment := taskqueue.TaskAssignment{
		TaskID:                  lease.TaskID,
		AccountID:               lease.AccountID,
		ProviderEndpointID:      lease.ProviderEndpointID,
		CredentialID:            lease.CredentialID,
		ResolvedModel:           lease.ResolvedModel,
		APIType:                 lease.APIType,
		ProviderEndpointName:    lease.ProviderEndpointName,
		ProviderEndpointBaseURL: lease.ProviderEndpointBaseURL,
		CredentialFingerprint:   lease.CredentialFingerprint,
		CredentialKeyVersion:    lease.CredentialKeyVersion,
		LeaseToken:              lease.LeaseToken,
	}
	if !assignment.HasLeaseIdentity() || !taskqueue.LeaseBelongsToRiverJob(lease.LeaseToken, riverJobID) {
		return ErrTaskStateConflict
	}
	return s.FailTask(ctx, FailTaskInput{Assignment: assignment, Reason: reason})
}

func (s *Service) FailTask(ctx context.Context, input FailTaskInput) error {
	if input.Assignment.HasLeaseIdentity() {
		return s.failRunningTaskWithLease(ctx, input)
	}
	if input.Assignment.TaskID != "" {
		return ErrTaskStateConflict
	}
	return s.failPendingTask(ctx, input)
}

func (s *Service) failPendingTask(ctx context.Context, input FailTaskInput) error {
	taskID := input.TaskID
	if taskID == "" {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(input.AttemptFinish)
	rows, err := s.taskRepo.MarkTaskFailed(ctx, taskID, pendingOnly(), input.Reason, formatTaskFailureResult(taskID, input.Reason), now, input.AttemptFinish)
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

func (s *Service) failRunningTaskWithLease(ctx context.Context, input FailTaskInput) error {
	assignment := input.Assignment
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(input.AttemptFinish)
	rows, err := s.taskRepo.MarkTaskFailedWithLease(ctx, assignment.TaskID, runningOnly(), assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID, assignment.LeaseToken, input.Reason, formatTaskFailureResult(assignment.TaskID, input.Reason), now, input.AttemptFinish)
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

func (s *Service) ResetRunningTask(ctx context.Context, input ResetRunningTaskInput) error {
	assignment := input.Assignment
	if !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}

	errMsg := ""
	if input.Reason != "" {
		errMsg = input.Reason
	}

	rows, err := s.taskRepo.ResetTaskRunningToPendingWithLease(ctx, assignment.TaskID, runningOnly(), assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID, assignment.LeaseToken, errMsg, input.AttemptFinish)
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

func (s *Service) RequeueTaskForFailover(ctx context.Context, input RequeueForFailoverInput) (bool, error) {
	assignment := input.Assignment
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
	if tasks.IDValue(task.AccountID) != assignment.AccountID || tasks.IDValue(task.AssignedProviderEndpointID) != assignment.ProviderEndpointID || task.LeaseToken != assignment.LeaseToken {
		return false, ErrTaskStateConflict
	}
	if assignment.CredentialID > 0 && tasks.IDValue(task.AssignedCredentialID) != assignment.CredentialID {
		return false, ErrTaskStateConflict
	}

	if task.RetryCount < defaultTaskRetryLimit-1 {
		rows, err := s.taskRepo.RequeueTaskRunningWithLease(ctx, assignment.TaskID, runningOnly(), assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID, assignment.LeaseToken, input.Reason, input.RequeuedAttemptFinish)
		if err != nil {
			return false, err
		}
		if rows > 0 {
			s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusPending)
			return true, nil
		}
		return false, ErrTaskStateConflict
	}

	now := taskTerminalTime(input.FinalFailAttemptFinish)
	rows, err := s.taskRepo.FinalFailTaskRunningWithLease(ctx, assignment.TaskID, runningOnly(), assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID, assignment.LeaseToken, input.Reason, formatTaskFailureResult(assignment.TaskID, input.Reason), now, input.FinalFailAttemptFinish)
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

func pendingOnly() []string {
	return []string{tasks.StatusPending}
}

func runningOnly() []string {
	return []string{tasks.StatusRunning}
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

func validateTaskOutputs(taskID string, outputs []tasks.TaskOutput) error {
	for _, output := range outputs {
		if strings.TrimSpace(output.TaskID) != "" && strings.TrimSpace(output.TaskID) != taskID {
			return fmt.Errorf("任务结果包含非本任务的输出")
		}
		outputKind := strings.TrimSpace(output.OutputKind)
		if outputKind == "" {
			outputKind = "asset"
		}
		if outputKind != "asset" && outputKind != "remote" {
			return fmt.Errorf("任务结果输出类型不支持")
		}
		if strings.TrimSpace(output.URL) == "" {
			return fmt.Errorf("任务结果输出缺少 URL")
		}
		if outputKind == "asset" && strings.TrimSpace(output.StorageKey) == "" {
			return fmt.Errorf("任务结果输出缺少 storage_key")
		}
		if outputKind == "remote" && strings.TrimSpace(output.StorageKey) != "" {
			return fmt.Errorf("remote 任务结果不能包含 storage_key")
		}
	}
	return nil
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
