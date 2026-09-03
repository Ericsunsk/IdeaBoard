package runtime

import (
	"context"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"time"

	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
)

func (s *Service) RecordExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime metrics store not configured")
	}
	return s.taskRepo.PatchExecutionMetrics(ctx, patch)
}

func (s *Service) RecordExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime snapshot store not configured")
	}
	_, err := s.taskRepo.PatchExecutionSnapshot(ctx, snapshot)
	return err
}

func (s *Service) StartTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	if s == nil || s.taskRepo == nil {
		return tasks.TaskAttemptRef{}, fmt.Errorf("task runtime attempt store not configured")
	}
	ref, err := s.taskRepo.CreateTaskAttempt(ctx, start)
	if database.IsNotFound(err) {
		return tasks.TaskAttemptRef{}, ErrTaskStateConflict
	}
	return ref, err
}

func (s *Service) FinishTaskAttempt(ctx context.Context, finish tasks.TaskAttemptFinish) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime attempt store not configured")
	}
	updated, err := s.taskRepo.FinishTaskAttempt(ctx, finish)
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrTaskStateConflict
	}
	if updated != 1 {
		return fmt.Errorf("finish task attempt affected %d rows, want 1", updated)
	}
	return nil
}

func (s *Service) RecordTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot) error {
	return s.recordTaskCancelled(ctx, snapshot, nil)
}

func (s *Service) RecordTaskCancelledWithAttempt(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, finish tasks.TaskAttemptFinish) error {
	return s.recordTaskCancelled(ctx, snapshot, &finish)
}

func (s *Service) recordTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime cancellation store not configured")
	}
	rows, err := s.taskRepo.MarkTaskCancelled(ctx, snapshot, attemptFinish)
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionCancel, snapshot.TaskID, taskqueueports.TaskAssignment{}, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, snapshot.TaskID, tasks.StatusCancelled)
	}
	return nil
}

func (s *Service) RecordExecutionTerminal(ctx context.Context, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime metrics store not configured")
	}
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	metrics := task.ExecutionMetrics
	totalLatencyMS := finishedAt.Sub(task.CreatedAt).Milliseconds()
	if totalLatencyMS < 0 {
		totalLatencyMS = 0
	}

	bottleneckSource := tasks.ResolveExecutionBottleneck(metrics, failureStage, failureCategory)
	patch := tasks.TaskExecutionMetricsPatch{
		TaskID:           taskID,
		CurrentStage:     tasks.StringPtr(currentStage),
		BottleneckSource: tasks.StringPtr(bottleneckSource),
		TotalLatencyMS:   int64Ptr(totalLatencyMS),
	}
	if currentStage == tasks.ExecutionStageFailed {
		patch.FailureStage = tasks.StringPtr(failureStage)
		patch.FailureCategory = tasks.StringPtr(failureCategory)
	} else {
		empty := ""
		patch.FailureStage = &empty
		patch.FailureCategory = &empty
	}
	return s.taskRepo.PatchExecutionMetrics(ctx, patch)
}

func int64Ptr(value int64) *int64 {
	return &value
}
