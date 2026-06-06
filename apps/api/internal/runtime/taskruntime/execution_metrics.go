package taskruntime

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/tasks"
)

func (s *Service) RecordExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error {
	if s == nil || s.taskRepo == nil {
		return fmt.Errorf("task runtime metrics store not configured")
	}
	return s.taskRepo.PatchExecutionMetrics(ctx, patch)
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

	bottleneckSource := resolveExecutionBottleneck(metrics, failureStage, failureCategory)
	patch := tasks.TaskExecutionMetricsPatch{
		TaskID:           taskID,
		CurrentStage:     stringPtr(currentStage),
		BottleneckSource: stringPtr(bottleneckSource),
		TotalLatencyMS:   int64Ptr(totalLatencyMS),
	}
	if currentStage == tasks.ExecutionStageFailed {
		patch.FailureStage = stringPtr(failureStage)
		patch.FailureCategory = stringPtr(failureCategory)
	} else {
		empty := ""
		patch.FailureStage = &empty
		patch.FailureCategory = &empty
	}
	return s.taskRepo.PatchExecutionMetrics(ctx, patch)
}

func resolveExecutionBottleneck(metrics *tasks.TaskExecutionMetrics, failureStage string, failureCategory string) string {
	switch failureCategory {
	case tasks.FailureCategoryStorageError:
		return tasks.BottleneckSourceStorage
	case tasks.FailureCategoryFinalizeError:
		return tasks.BottleneckSourceFinalize
	case tasks.FailureCategoryInputInvalid:
		if failureStage == tasks.ExecutionStageAssigned || failureStage == tasks.ExecutionStageQueued {
			return tasks.BottleneckSourceInput
		}
	case tasks.FailureCategoryAuth, tasks.FailureCategoryQuota, tasks.FailureCategoryTimeout, tasks.FailureCategoryProviderError:
		return tasks.BottleneckSourceProvider
	}
	if metrics == nil {
		return tasks.BottleneckSourceUnknown
	}
	type stageLatency struct {
		source string
		value  int64
	}
	latencies := []stageLatency{
		{source: tasks.BottleneckSourceQueue, value: metrics.QueueWaitMS},
		{source: tasks.BottleneckSourceWorker, value: metrics.WorkerOverheadMS},
		{source: tasks.BottleneckSourceProvider, value: metrics.ProviderLatencyMS},
		{source: tasks.BottleneckSourceFinalize, value: metrics.FinalizeLatencyMS},
	}
	best := stageLatency{source: tasks.BottleneckSourceUnknown}
	for _, latency := range latencies {
		if latency.value > best.value {
			best = latency
		}
	}
	if best.value <= 0 {
		return tasks.BottleneckSourceUnknown
	}
	return best.source
}

func stringPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
