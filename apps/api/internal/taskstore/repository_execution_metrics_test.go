package taskstore

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
)

func TestListAttachesExecutionMetrics(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-5 * time.Minute)
	task := &tasks.Task{
		TaskID:    "metrics-list-task",
		UserID:    10,
		ChannelID: 1,
		Status:    tasks.StatusRunning,
		CreatedAt: createdAt,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	stage := tasks.ExecutionStageProvider
	bottleneck := tasks.BottleneckSourceProvider
	providerStartedAt := time.Now().Add(-3 * time.Second)
	queueWaitMS := int64(1200)
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:            task.TaskID,
		CurrentStage:      &stage,
		BottleneckSource:  &bottleneck,
		ProviderStartedAt: &providerStartedAt,
		QueueWaitMS:       &queueWaitMS,
	}); err != nil {
		t.Fatalf("patch execution metrics failed: %v", err)
	}

	nextStage := tasks.ExecutionStageFinalizing
	totalLatencyMS := int64(4500)
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:         task.TaskID,
		CurrentStage:   &nextStage,
		TotalLatencyMS: &totalLatencyMS,
	}); err != nil {
		t.Fatalf("patch execution metrics update failed: %v", err)
	}

	items, total, err := taskRepo.List(ctx, 0, 0, "", "", 1, 20)
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("list total=%d len=%d, want 1/1", total, len(items))
	}
	metrics := items[0].ExecutionMetrics
	if metrics == nil {
		t.Fatal("execution_metrics missing from task list item")
	}
	if metrics.CurrentStage != nextStage {
		t.Fatalf("current_stage=%q, want %q", metrics.CurrentStage, nextStage)
	}
	if metrics.BottleneckSource != bottleneck {
		t.Fatalf("bottleneck_source=%q, want %q", metrics.BottleneckSource, bottleneck)
	}
	if metrics.ProviderStartedAt == nil {
		t.Fatal("provider_started_at was cleared by partial patch")
	}
	if metrics.QueueWaitMS != queueWaitMS || metrics.TotalLatencyMS != totalLatencyMS {
		t.Fatalf("latencies=%d/%d, want %d/%d", metrics.QueueWaitMS, metrics.TotalLatencyMS, queueWaitMS, totalLatencyMS)
	}
}

func TestListExecutionBottleneckStatsByCreatedRangeOnlyUsesTerminalTasks(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	seedTasks(t, taskRepo, ctx,
		&tasks.Task{TaskID: "metrics-running", UserID: 10, ChannelID: 1, Status: tasks.StatusRunning, CreatedAt: now.Add(-5 * time.Minute)},
		&tasks.Task{TaskID: "metrics-completed", UserID: 10, ChannelID: 1, Status: tasks.StatusCompleted, CreatedAt: now.Add(-4 * time.Minute)},
		&tasks.Task{TaskID: "metrics-failed", UserID: 10, ChannelID: 1, Status: tasks.StatusFailed, CreatedAt: now.Add(-3 * time.Minute)},
	)

	queue := tasks.BottleneckSourceQueue
	provider := tasks.BottleneckSourceProvider
	storage := tasks.BottleneckSourceStorage
	totalLatencyMS := int64(3000)
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:           "metrics-running",
		BottleneckSource: &queue,
		TotalLatencyMS:   &totalLatencyMS,
	}); err != nil {
		t.Fatalf("patch running metrics failed: %v", err)
	}
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:           "metrics-completed",
		BottleneckSource: &provider,
		TotalLatencyMS:   &totalLatencyMS,
	}); err != nil {
		t.Fatalf("patch completed metrics failed: %v", err)
	}
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:           "metrics-failed",
		BottleneckSource: &storage,
		TotalLatencyMS:   &totalLatencyMS,
	}); err != nil {
		t.Fatalf("patch failed metrics failed: %v", err)
	}

	rows, err := taskRepo.ListExecutionBottleneckStatsByCreatedRange(ctx, now.Add(-10*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("list bottleneck stats failed: %v", err)
	}

	bySource := map[string]ExecutionBottleneckAggregate{}
	for _, row := range rows {
		bySource[row.Source] = row
	}
	if _, ok := bySource[tasks.BottleneckSourceQueue]; ok {
		t.Fatalf("running task source %q leaked into terminal bottleneck stats: %+v", tasks.BottleneckSourceQueue, rows)
	}
	if bySource[tasks.BottleneckSourceProvider].Total != 1 || bySource[tasks.BottleneckSourceProvider].Failed != 0 {
		t.Fatalf("provider aggregate=%+v, want total=1 failed=0", bySource[tasks.BottleneckSourceProvider])
	}
	if bySource[tasks.BottleneckSourceStorage].Total != 1 || bySource[tasks.BottleneckSourceStorage].Failed != 1 {
		t.Fatalf("storage aggregate=%+v, want total=1 failed=1", bySource[tasks.BottleneckSourceStorage])
	}
}

func TestListExecutionFailureCategoryStatsByCreatedRangeOnlyUsesFailedTasks(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	seedTasks(t, taskRepo, ctx,
		&tasks.Task{TaskID: "failure-category-completed", UserID: 10, ChannelID: 1, Status: tasks.StatusCompleted, CreatedAt: now.Add(-4 * time.Minute)},
		&tasks.Task{TaskID: "failure-category-provider", UserID: 10, ChannelID: 1, Status: tasks.StatusFailed, CreatedAt: now.Add(-3 * time.Minute)},
		&tasks.Task{TaskID: "failure-category-timeout", UserID: 10, ChannelID: 1, Status: tasks.StatusFailed, CreatedAt: now.Add(-2 * time.Minute)},
	)

	providerCategory := tasks.FailureCategoryProviderError
	timeoutCategory := tasks.FailureCategoryTimeout
	totalLatencyMS := int64(3000)
	for _, item := range []struct {
		taskID   string
		category string
	}{
		{taskID: "failure-category-completed", category: providerCategory},
		{taskID: "failure-category-provider", category: providerCategory},
		{taskID: "failure-category-timeout", category: timeoutCategory},
	} {
		if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
			TaskID:          item.taskID,
			FailureCategory: &item.category,
			TotalLatencyMS:  &totalLatencyMS,
		}); err != nil {
			t.Fatalf("patch %s metrics failed: %v", item.taskID, err)
		}
	}

	rows, err := taskRepo.ListExecutionFailureCategoryStatsByCreatedRange(ctx, now.Add(-10*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("list failure category stats failed: %v", err)
	}

	byCategory := map[string]ExecutionFailureCategoryAggregate{}
	for _, row := range rows {
		byCategory[row.Category] = row
	}
	if byCategory[tasks.FailureCategoryProviderError].Total != 1 {
		t.Fatalf("provider category should only count failed tasks, got %+v", byCategory[tasks.FailureCategoryProviderError])
	}
	if byCategory[tasks.FailureCategoryTimeout].Total != 1 {
		t.Fatalf("timeout category aggregate mismatch: %+v", byCategory[tasks.FailureCategoryTimeout])
	}
}
