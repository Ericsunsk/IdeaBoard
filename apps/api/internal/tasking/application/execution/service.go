package execution

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

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
)

const (
	taskCancellationPollInterval = 2 * time.Second
	// taskFailureStatus is the failure word embedded in the task result
	// JSON payload (formatTaskFailureResult). The wire word is unchanged
	// ("failed"); the constant only kills the raw literal duplicated in
	// the marshal-fallback branch.
	taskFailureStatus = "failed"
)

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskTerminal      = errors.New("task already in terminal state")
	ErrTaskStateConflict = errors.New("task state conflict")
)

type TaskStore interface {
	ConvergeTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error)
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetActiveExecutionAttemptByTaskID(ctx context.Context, taskID string) (*runtimecontracts.ActiveTaskExecution, error)
	ClaimAndStartExecutionAttempt(ctx context.Context, start tasks.ExecutionAttemptStart) (int32, error)
	PatchExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error
	PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error)
	FinalizeExecutionAttempt(ctx context.Context, finish tasks.ExecutionAttemptFinish) (int64, error)
	MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
}

type Service struct {
	taskRepo    TaskStore
	cancelStore taskqueueports.TaskCancellationStore
	taskEvents  events.TaskStatusPublisher
}

type FailTaskInput struct {
	TaskID         string
	ExecutionToken string
	Reason         string
	Result         string
	ResolvedModel  string
	GatewayStep    string
	ReceiptLogID   string
	ReceiptEventID string
	CacheHit       bool
	AttemptFinish  *tasks.TaskAttemptFinish
}

type ResetRunningTaskInput struct {
	TaskID         string
	ExecutionToken string
	Reason         string
	AttemptFinish  *tasks.TaskAttemptFinish
}

func NewService(
	taskRepo TaskStore,
	cancelStore taskqueueports.TaskCancellationStore,
) *Service {
	service := &Service{
		taskRepo:    taskRepo,
		cancelStore: cancelStore,
	}
	return service
}

func (s *Service) SetTaskStatusPublisher(publisher events.TaskStatusPublisher) {
	s.taskEvents = publisher
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
		Kind:           transition.convergenceKind,
		TaskID:         taskID,
		ExecutionToken: transition.executionToken,
		ErrorMessage:   reason,
		Result:         resolveTaskFailureResult(taskID, reason, ""),
		OccurredAt:     time.Now(),
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	if rows == 0 {
		if err := s.resolveFinalFailureNoop(ctx, taskID); err != nil {
			return err
		}
	}
	applied, err := s.resolveTaskTransitionResult(ctx, transition.transition, taskID, rows)
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
	// executionToken fences the write when the failed River job still owns
	// the live attempt; empty for pending or already-reaped tasks.
	executionToken string
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
	activeAttempt, err := s.taskRepo.GetActiveExecutionAttemptByTaskID(ctx, taskID)
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
	if riverJobID > 0 && !taskqueueports.LeaseBelongsToRiverJob(activeAttempt.ExecutionToken, riverJobID) {
		return finalFailureTransition{}, ErrTaskStateConflict
	}
	result.transition = taskTransitionFinalFail
	result.convergenceKind = runtimecontracts.TaskConvergenceFinalFailed
	result.executionToken = activeAttempt.ExecutionToken
	return result, nil
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
	if strings.TrimSpace(input.ExecutionToken) != "" {
		return s.failRunningTaskWithExecutionToken(ctx, input)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return ErrTaskStateConflict
	}
	return s.failPendingTask(ctx, input)
}

func (s *Service) failPendingTask(ctx context.Context, input FailTaskInput) error {
	taskID := strings.TrimSpace(input.TaskID)
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
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFailPending, taskID, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
	}
	return nil
}

func (s *Service) failRunningTaskWithExecutionToken(ctx context.Context, input FailTaskInput) error {
	if err := requireTaskTransitionToken(taskTransitionFailRunning, input.ExecutionToken); err != nil {
		return err
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return ErrTaskStateConflict
	}
	now := taskTerminalTime(input.AttemptFinish)
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:           runtimecontracts.TaskConvergenceFailed,
		TaskID:         input.TaskID,
		ExecutionToken: strings.TrimSpace(input.ExecutionToken),
		ErrorMessage:   input.Reason,
		Result:         resolveTaskFailureResult(input.TaskID, input.Reason, input.Result),
		OccurredAt:     now,
		AttemptFinish:  input.AttemptFinish,
		ResolvedModel:  strings.TrimSpace(input.ResolvedModel),
		GatewayStep:    strings.TrimSpace(input.GatewayStep),
		ReceiptLogID:   strings.TrimSpace(input.ReceiptLogID),
		ReceiptEventID: strings.TrimSpace(input.ReceiptEventID),
		CacheHit:       input.CacheHit,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionFailRunning, input.TaskID, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, input.TaskID, tasks.StatusFailed)
	}
	return nil
}

func (s *Service) ResetRunningTask(ctx context.Context, input ResetRunningTaskInput) error {
	token := strings.TrimSpace(input.ExecutionToken)
	if token == "" {
		// Nothing was claimed by this worker; resetting is a no-op so the
		// task keeps whatever state convergence left behind.
		return nil
	}
	return s.convergeRunningTask(ctx, input, runtimecontracts.TaskConvergenceReset, taskTransitionReset)
}

// RequeueRunningTask records one bounded retry for a running execution. Unlike
// ResetRunningTask, requeue convergence increments retry_count atomically with
// releasing the execution token.
func (s *Service) RequeueRunningTask(ctx context.Context, input ResetRunningTaskInput) error {
	return s.convergeRunningTask(ctx, input, runtimecontracts.TaskConvergenceRequeued, taskTransitionRequeue)
}

func (s *Service) convergeRunningTask(
	ctx context.Context,
	input ResetRunningTaskInput,
	kind runtimecontracts.TaskConvergenceKind,
	transition taskTransition,
) error {
	token := strings.TrimSpace(input.ExecutionToken)
	if err := requireTaskTransitionToken(transition, token); err != nil {
		return err
	}

	errMsg := ""
	if input.Reason != "" {
		errMsg = input.Reason
	}

	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:           kind,
		TaskID:         strings.TrimSpace(input.TaskID),
		ExecutionToken: token,
		ErrorMessage:   errMsg,
		OccurredAt:     taskTerminalTime(input.AttemptFinish),
		AttemptFinish:  input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, transition, strings.TrimSpace(input.TaskID), rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, strings.TrimSpace(input.TaskID), tasks.StatusPending)
	}
	return nil
}

func taskTerminalTime(attemptFinish *tasks.TaskAttemptFinish) time.Time {
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		return attemptFinish.CompletedAt
	}
	return time.Now()
}

// TaskAttemptFinishFromExecution adapts the gateway attempt outcome onto the
// legacy attempt-finish shape consumed by stats and cancellation paths.
func TaskAttemptFinishFromExecution(finish *tasks.ExecutionAttemptFinish) *tasks.TaskAttemptFinish {
	if finish == nil {
		return nil
	}
	return &tasks.TaskAttemptFinish{
		TaskID:          strings.TrimSpace(finish.TaskID),
		Status:          strings.TrimSpace(finish.Status),
		FailureStage:    strings.TrimSpace(finish.FailureStage),
		FailureCategory: strings.TrimSpace(finish.FailureCategory),
		ErrorMsg:        finish.ErrorMsg,
		CompletedAt:     finish.CompletedAt,
	}
}

// ClaimAndStartExecutionAttempt fences one pending task for the execution
// token and opens its attempt row atomically. It returns the server-assigned
// attempt number. A state-conflict error means another worker won the race
// or the task already converged; callers drop such tasks instead of retrying.
func (s *Service) ClaimAndStartExecutionAttempt(ctx context.Context, start tasks.ExecutionAttemptStart) (int32, error) {
	start.TaskID = strings.TrimSpace(start.TaskID)
	start.ExecutionToken = strings.TrimSpace(start.ExecutionToken)
	attemptNo, err := s.taskRepo.ClaimAndStartExecutionAttempt(ctx, start)
	if err != nil {
		if database.IsNotFound(err) {
			return 0, ErrTaskStateConflict
		}
		return 0, err
	}
	s.publishTaskStatusEvent(ctx, start.TaskID, tasks.StatusRunning)
	return attemptNo, nil
}

// GetTaskRetryCount reads the fresh tasks.retry_count for requeue bounding.
// Payload-carried counts go stale across redeliveries, so the orchestrator
// loads it at failure time; the token fence (not this read) owns correctness.
func (s *Service) GetTaskRetryCount(ctx context.Context, taskID string) (int, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		if database.IsNotFound(err) {
			return 0, ErrTaskNotFound
		}
		return 0, err
	}
	return task.RetryCount, nil
}

func validateTaskOutputs(taskID string, outputs []tasks.TaskOutput) error {
	for _, output := range outputs {
		if strings.TrimSpace(output.TaskID) != "" && strings.TrimSpace(output.TaskID) != taskID {
			return fmt.Errorf(msgValidateTaskOutputs)
		}
		outputKind := strings.TrimSpace(output.OutputKind)
		if outputKind == "" {
			outputKind = "asset"
		}
		if outputKind != "asset" && outputKind != "remote" {
			return fmt.Errorf(msgValidateTaskOutputs2)
		}
		if strings.TrimSpace(output.URL) == "" {
			return fmt.Errorf(msgValidateTaskOutputs3)
		}
		if outputKind == "asset" && strings.TrimSpace(output.StorageKey) == "" {
			return fmt.Errorf(msgValidateTaskOutputs4)
		}
		if outputKind == "remote" && strings.TrimSpace(output.StorageKey) != "" {
			return fmt.Errorf(msgValidateTaskOutputs5)
		}
	}
	return nil
}

func formatTaskFailureResult(taskID string, reason string) string {
	result := map[string]string{
		"task_id": taskID,
		"status":  taskFailureStatus,
		"error":   reason,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"task_id":"%s","status":"`+taskFailureStatus+msgFormatTaskFailureResult, taskID)
	}
	return string(resultJSON)
}

func resolveTaskFailureResult(taskID string, reason string, result string) string {
	if strings.TrimSpace(result) != "" {
		return strings.TrimSpace(result)
	}
	return formatTaskFailureResult(taskID, reason)
}
