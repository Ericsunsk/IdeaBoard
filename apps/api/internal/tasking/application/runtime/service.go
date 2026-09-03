package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
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
	ConvergeTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error)
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetActiveTaskLeaseByTaskID(ctx context.Context, taskID string) (*runtimecontracts.ActiveTaskLease, error)
	PatchExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error
	PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error)
	CreateTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error)
	FinishTaskAttempt(ctx context.Context, finish tasks.TaskAttemptFinish) (int64, error)
	MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
}

type Service struct {
	accountRepo AccountStore
	taskRepo    TaskStore
	queue       taskqueueports.TaskAssigner
	cancelStore taskqueueports.TaskCancellationStore
	taskEvents  events.TaskStatusPublisher
}

type FailTaskInput struct {
	TaskID        string
	Assignment    taskqueueports.TaskAssignment
	Reason        string
	Result        string
	AttemptFinish *tasks.TaskAttemptFinish
}

type ResetRunningTaskInput struct {
	Assignment    taskqueueports.TaskAssignment
	Reason        string
	AttemptFinish *tasks.TaskAttemptFinish
}

type RequeueForFailoverInput struct {
	Assignment             taskqueueports.TaskAssignment
	Reason                 string
	Result                 string
	RequeuedAttemptFinish  *tasks.TaskAttemptFinish
	FinalFailAttemptFinish *tasks.TaskAttemptFinish
}

func NewService(
	accountRepo AccountStore,
	taskRepo TaskStore,
	assigner taskqueueports.TaskAssigner,
	cancelStore taskqueueports.TaskCancellationStore,
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

func (s *Service) ConfirmTaskLease(ctx context.Context, assignment taskqueueports.TaskAssignment) error {
	if s.queue == nil {
		return ErrTaskStateConflict
	}
	if err := s.queue.ConfirmTaskLease(ctx, assignment); err != nil {
		if errors.Is(err, taskqueueports.ErrTaskLeaseConflict) {
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
	task, err := s.loadTaskForFinalFailure(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == tasks.StatusFailed {
		return nil
	}
	if tasks.IsTerminalStatus(task.Status) {
		return ErrTaskTerminal
	}
	transition, err := s.prepareFinalFailureTransition(ctx, task, taskID, riverJobID)
	if err != nil {
		return err
	}
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:         transition.convergenceKind,
		TaskID:       taskID,
		Lease:        transition.leaseIdentity,
		ErrorMessage: reason,
		Result:       resolveTaskFailureResult(taskID, reason, ""),
		OccurredAt:   time.Now(),
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	if rows == 0 {
		if err := s.resolveFinalFailureNoop(ctx, taskID); err != nil {
			return err
		}
	}
	applied, err := s.resolveTaskTransitionResult(ctx, transition.transition, taskID, transition.assignment, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
	}
	return nil
}

type finalFailureTransition struct {
	transition      taskTransition
	convergenceKind runtimecontracts.TaskConvergenceKind
	assignment      taskqueueports.TaskAssignment
	leaseIdentity   *runtimecontracts.TaskLeaseIdentity
}

func (s *Service) loadTaskForFinalFailure(ctx context.Context, taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return task, nil
}

func (s *Service) prepareFinalFailureTransition(ctx context.Context, task *tasks.Task, taskID string, riverJobID int64) (finalFailureTransition, error) {
	result := finalFailureTransition{
		transition:      taskTransitionFailPending,
		convergenceKind: runtimecontracts.TaskConvergenceFinalFailed,
	}
	switch task.Status {
	case tasks.StatusPending:
		// Pending tasks have no lease to release, but use the same explicit
		// final-failure kind as exhausted running attempts.
		return result, nil
	case tasks.StatusRunning:
		return s.prepareRunningFinalFailureTransition(ctx, result, taskID, riverJobID)
	default:
		return finalFailureTransition{}, ErrTaskStateConflict
	}
}

func (s *Service) prepareRunningFinalFailureTransition(ctx context.Context, result finalFailureTransition, taskID string, riverJobID int64) (finalFailureTransition, error) {
	activeLease, err := s.taskRepo.GetActiveTaskLeaseByTaskID(ctx, taskID)
	if err != nil {
		if !database.IsNotFound(err) {
			return finalFailureTransition{}, err
		}
		if riverJobID != 0 {
			return finalFailureTransition{}, ErrTaskStateConflict
		}
		result.transition = taskTransitionRecoveryFail
		result.convergenceKind = runtimecontracts.TaskConvergenceRecoveredFailed
		return result, nil
	}
	assignment := finalFailureAssignment(activeLease)
	if !assignment.HasLeaseIdentity() || (riverJobID > 0 && !taskqueueports.LeaseBelongsToRiverJob(activeLease.LeaseToken, riverJobID)) {
		return finalFailureTransition{}, ErrTaskStateConflict
	}
	result.transition = taskTransitionFinalFail
	result.assignment = assignment
	result.leaseIdentity = taskConvergenceLease(assignment)
	return result, nil
}

func finalFailureAssignment(activeLease *runtimecontracts.ActiveTaskLease) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{
		TaskID:                  activeLease.TaskID,
		AccountID:               activeLease.AccountID,
		ProviderEndpointID:      activeLease.ProviderEndpointID,
		CredentialID:            activeLease.CredentialID,
		ResolvedModel:           activeLease.ResolvedModel,
		APIType:                 activeLease.APIType,
		ProviderEndpointName:    activeLease.ProviderEndpointName,
		ProviderEndpointBaseURL: activeLease.ProviderEndpointBaseURL,
		CredentialFingerprint:   activeLease.CredentialFingerprint,
		CredentialKeyVersion:    activeLease.CredentialKeyVersion,
		LeaseToken:              activeLease.LeaseToken,
	}
}

func (s *Service) resolveFinalFailureNoop(ctx context.Context, taskID string) error {
	latest, err := s.loadTaskForFinalFailure(ctx, taskID)
	if err != nil {
		return err
	}
	if latest.Status == tasks.StatusFailed {
		return nil
	}
	if tasks.IsTerminalStatus(latest.Status) {
		return ErrTaskTerminal
	}
	return nil
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
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceFailed,
		TaskID:        taskID,
		ErrorMessage:  input.Reason,
		Result:        resolveTaskFailureResult(taskID, input.Reason, input.Result),
		OccurredAt:    now,
		AttemptFinish: input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFailPending, taskID, taskqueueports.TaskAssignment{}, rows)
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
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceFailed,
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

	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceReset,
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
		rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
			Kind:          runtimecontracts.TaskConvergenceRequeued,
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
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceFinalFailed,
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

func taskConvergenceLease(assignment taskqueueports.TaskAssignment) *runtimecontracts.TaskLeaseIdentity {
	return &runtimecontracts.TaskLeaseIdentity{
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
