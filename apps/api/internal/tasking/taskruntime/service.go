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
	ConvergeTask(ctx context.Context, intent taskstore.TaskConvergenceIntent) (int64, error)
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

type FailTaskInput struct {
	TaskID        string
	Assignment    taskqueue.TaskAssignment
	Reason        string
	Result        string
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
	Result                 string
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
	rows, err := s.taskRepo.ConvergeTask(ctx, taskstore.TaskConvergenceIntent{
		Kind:          taskstore.TaskConvergenceFailed,
		TaskID:        taskID,
		ErrorMessage:  input.Reason,
		Result:        resolveTaskFailureResult(taskID, input.Reason, input.Result),
		OccurredAt:    now,
		AttemptFinish: input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFailPending, taskID, taskqueue.TaskAssignment{}, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
	}
	return nil
}

func (s *Service) failRunningTaskWithLease(ctx context.Context, input FailTaskInput) error {
	assignment := input.Assignment
	if err := requireTaskTransitionLease(taskTransitionFailRunning, assignment); err != nil {
		return err
	}
	now := taskTerminalTime(input.AttemptFinish)
	rows, err := s.taskRepo.ConvergeTask(ctx, taskstore.TaskConvergenceIntent{
		Kind:          taskstore.TaskConvergenceFailed,
		TaskID:        assignment.TaskID,
		Lease:         taskConvergenceLease(assignment),
		ErrorMessage:  input.Reason,
		Result:        resolveTaskFailureResult(assignment.TaskID, input.Reason, input.Result),
		OccurredAt:    now,
		AttemptFinish: input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFailRunning, assignment.TaskID, assignment, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusFailed)
	}
	return nil
}

func (s *Service) ResetRunningTask(ctx context.Context, input ResetRunningTaskInput) error {
	assignment := input.Assignment
	if err := requireTaskTransitionLease(taskTransitionReset, assignment); err != nil {
		return err
	}

	errMsg := ""
	if input.Reason != "" {
		errMsg = input.Reason
	}

	rows, err := s.taskRepo.ConvergeTask(ctx, taskstore.TaskConvergenceIntent{
		Kind:          taskstore.TaskConvergenceReset,
		TaskID:        assignment.TaskID,
		Lease:         taskConvergenceLease(assignment),
		ErrorMessage:  errMsg,
		OccurredAt:    taskTerminalTime(input.AttemptFinish),
		AttemptFinish: input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionReset, assignment.TaskID, assignment, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusPending)
	}
	return nil
}

func (s *Service) RequeueTaskForFailover(ctx context.Context, input RequeueForFailoverInput) (bool, error) {
	assignment := input.Assignment
	if err := requireTaskTransitionLease(taskTransitionRequeue, assignment); err != nil {
		return false, err
	}
	task, err := s.taskRepo.GetByTaskID(ctx, assignment.TaskID)
	if err != nil {
		if database.IsNotFound(err) {
			return false, ErrTaskNotFound
		}
		return false, err
	}
	current, err := evaluateTaskTransitionCurrent(taskTransitionRequeue, task, assignment)
	if err != nil {
		return false, err
	}
	if current.idempotent {
		return true, nil
	}

	if task.RetryCount < defaultTaskRetryLimit-1 {
		rows, err := s.taskRepo.ConvergeTask(ctx, taskstore.TaskConvergenceIntent{
			Kind:          taskstore.TaskConvergenceRequeued,
			TaskID:        assignment.TaskID,
			Lease:         taskConvergenceLease(assignment),
			ErrorMessage:  input.Reason,
			Result:        strings.TrimSpace(input.Result),
			OccurredAt:    taskTerminalTime(input.RequeuedAttemptFinish),
			AttemptFinish: input.RequeuedAttemptFinish,
		})
		if err := normalizeTaskTransitionWriteError(err); err != nil {
			return false, err
		}
		applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionRequeue, assignment.TaskID, assignment, rows)
		if err != nil {
			return false, err
		}
		if applied {
			s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusPending)
		}
		return true, nil
	}

	now := taskTerminalTime(input.FinalFailAttemptFinish)
	rows, err := s.taskRepo.ConvergeTask(ctx, taskstore.TaskConvergenceIntent{
		Kind:          taskstore.TaskConvergenceFinalFailed,
		TaskID:        assignment.TaskID,
		Lease:         taskConvergenceLease(assignment),
		ErrorMessage:  input.Reason,
		Result:        resolveTaskFailureResult(assignment.TaskID, input.Reason, input.Result),
		OccurredAt:    now,
		AttemptFinish: input.FinalFailAttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return false, err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFinalFail, assignment.TaskID, assignment, rows)
	if err != nil {
		return false, err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusFailed)
		return false, nil
	}
	return true, nil
}

func taskTerminalTime(attemptFinish *tasks.TaskAttemptFinish) time.Time {
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		return attemptFinish.CompletedAt
	}
	return time.Now()
}

func taskConvergenceLease(assignment taskqueue.TaskAssignment) *taskstore.TaskLeaseIdentity {
	return &taskstore.TaskLeaseIdentity{
		AccountID:          assignment.AccountID,
		ProviderEndpointID: assignment.ProviderEndpointID,
		CredentialID:       assignment.CredentialID,
		LeaseToken:         assignment.LeaseToken,
	}
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

func resolveTaskFailureResult(taskID string, reason string, result string) string {
	if strings.TrimSpace(result) != "" {
		return strings.TrimSpace(result)
	}
	return formatTaskFailureResult(taskID, reason)
}
