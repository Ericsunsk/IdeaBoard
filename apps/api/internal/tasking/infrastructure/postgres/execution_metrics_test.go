package postgres

import (
	"context"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

func TestListAttachesExecutionMetrics(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 92)
	createdAt := time.Now().Add(-5 * time.Minute)
	task := &tasks.Task{
		TaskID:                     "metrics-list-task",
		UserID:                     10,
		AssignedProviderEndpointID: testPtr(accountID + 10000),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		CreatedAt:                  createdAt,
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
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
		TaskID:           task.TaskID,
		CurrentStage:     &nextStage,
		BottleneckSource: &bottleneck,
		TotalLatencyMS:   &totalLatencyMS,
	}); err != nil {
		t.Fatalf("patch execution metrics update failed: %v", err)
	}

	items, counts, err := taskRepo.List(ctx, 0, 0, "", "", 1, 20)
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	if counts.Total != 1 || len(items) != 1 {
		t.Fatalf("list total=%d len=%d, want 1/1", counts.Total, len(items))
	}
	metrics := items[0].ExecutionMetrics
	if metrics == nil {
		t.Fatal("execution_metrics missing from task list item")
		return
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

func TestListCountsCancelledTasksSeparately(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	cancelledAt := now.Add(-2 * time.Minute)

	tasksToSeed := []*tasks.Task{
		{
			TaskID:    "list-count-pending",
			UserID:    10,
			Status:    tasks.StatusPending,
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			TaskID:       "list-count-cancelled",
			UserID:       10,
			Status:       tasks.StatusCancelled,
			CreatedAt:    now.Add(-4 * time.Minute),
			CompletedAt:  &cancelledAt,
			CancelledAt:  &cancelledAt,
			CancelReason: "user cancelled",
		},
	}
	for _, task := range tasksToSeed {
		if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
			t.Fatalf("seed task %s failed: %v", task.TaskID, err)
		}
	}

	_, counts, err := taskRepo.List(ctx, 0, 0, "", "", 1, 20)
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	if counts.Total != 2 {
		t.Fatalf("total=%d, want 2", counts.Total)
	}
	if counts.PendingCount != 1 || counts.CancelledCount != 1 {
		t.Fatalf("pending/cancelled=%d/%d, want 1/1", counts.PendingCount, counts.CancelledCount)
	}
	if counts.OtherCount != 0 {
		t.Fatalf("other_count=%d, want 0", counts.OtherCount)
	}
}

func TestListCountsIgnoreStatusFilter(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	completedAt := now.Add(-2 * time.Minute)

	tasksToSeed := []*tasks.Task{
		{
			TaskID:    "list-count-filter-pending",
			UserID:    10,
			Status:    tasks.StatusPending,
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			TaskID:      "list-count-filter-failed",
			UserID:      10,
			Status:      tasks.StatusFailed,
			CreatedAt:   now.Add(-4 * time.Minute),
			CompletedAt: &completedAt,
			ErrorMsg:    "boom",
		},
	}
	for _, task := range tasksToSeed {
		if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
			t.Fatalf("seed task %s failed: %v", task.TaskID, err)
		}
	}

	items, counts, err := taskRepo.List(ctx, 0, 0, string(tasks.StatusFailed), "", 1, 20)
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	if len(items) != 1 || items[0].TaskID != "list-count-filter-failed" {
		t.Fatalf("filtered rows = %d, want only the failed task", len(items))
	}
	if counts.Total != 2 || counts.PendingCount != 1 || counts.FailedCount != 1 {
		t.Fatalf("counts total/pending/failed=%d/%d/%d, want 2/1/1", counts.Total, counts.PendingCount, counts.FailedCount)
	}
}

func TestPatchExecutionMetricsDoesNotOverwriteTerminalStage(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-5 * time.Minute)
	task := &tasks.Task{
		TaskID:    "metrics-terminal-guard",
		UserID:    10,
		Status:    tasks.StatusCancelled,
		CreatedAt: createdAt,
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	terminalStage := tasks.ExecutionStageCancelled
	terminalCategory := tasks.FailureCategoryCancelled
	terminalSource := tasks.BottleneckSourceUnknown
	terminalTotalLatencyMS := int64(1000)
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:           task.TaskID,
		CurrentStage:     &terminalStage,
		BottleneckSource: &terminalSource,
		FailureCategory:  &terminalCategory,
		TotalLatencyMS:   &terminalTotalLatencyMS,
	}); err != nil {
		t.Fatalf("patch terminal metrics failed: %v", err)
	}

	lateStage := tasks.ExecutionStageProvider
	lateCategory := tasks.FailureCategoryProviderError
	lateSource := tasks.BottleneckSourceProvider
	lateTotalLatencyMS := int64(9000)
	providerLatencyMS := int64(500)
	if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
		TaskID:            task.TaskID,
		CurrentStage:      &lateStage,
		BottleneckSource:  &lateSource,
		FailureCategory:   &lateCategory,
		ProviderLatencyMS: &providerLatencyMS,
		TotalLatencyMS:    &lateTotalLatencyMS,
	}); err != nil {
		t.Fatalf("patch late progress metrics failed: %v", err)
	}

	loaded, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	metrics := loaded.ExecutionMetrics
	if metrics == nil {
		t.Fatal("execution metrics missing")
		return
	}
	if metrics.CurrentStage != terminalStage {
		t.Fatalf("current_stage=%q, want terminal %q", metrics.CurrentStage, terminalStage)
	}
	if metrics.FailureCategory != terminalCategory {
		t.Fatalf("failure_category=%q, want terminal %q", metrics.FailureCategory, terminalCategory)
	}
	if metrics.BottleneckSource != terminalSource {
		t.Fatalf("bottleneck_source=%q, want terminal %q", metrics.BottleneckSource, terminalSource)
	}
	if metrics.TotalLatencyMS != terminalTotalLatencyMS {
		t.Fatalf("total_latency_ms=%d, want terminal %d", metrics.TotalLatencyMS, terminalTotalLatencyMS)
	}
	if metrics.ProviderLatencyMS != providerLatencyMS {
		t.Fatalf("provider_latency_ms=%d, want late progress value %d", metrics.ProviderLatencyMS, providerLatencyMS)
	}
}

func TestMarkTaskFailedWithLeaseWritesTerminalExecutionMetrics(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 94)
	channelID := accountID + 10000
	completedAt := time.Now()
	task := &tasks.Task{
		TaskID:                     "metrics-failed-with-lease",
		UserID:                     10,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-metrics-failed",
		CreatedAt:                  completedAt.Add(-2 * time.Second),
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	rows, err := taskRepo.convergeFailedWithLeaseForTest(ctx, task.TaskID, task.LeaseToken, "provider timeout", "{}", completedAt, &tasks.TaskAttemptFinish{
		FailureStage:    tasks.ExecutionStageProvider,
		FailureCategory: tasks.FailureCategoryTimeout,
		CompletedAt:     completedAt,
	})
	if err != nil || rows != 1 {
		t.Fatalf("mark failed rows=%d err=%v, want rows=1 nil err", rows, err)
	}

	loaded, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.ExecutionMetrics == nil {
		t.Fatal("execution metrics missing after failed terminal write")
	}
	if loaded.ExecutionMetrics.CurrentStage != tasks.ExecutionStageFailed {
		t.Fatalf("current_stage=%q, want %q", loaded.ExecutionMetrics.CurrentStage, tasks.ExecutionStageFailed)
	}
	if loaded.ExecutionMetrics.FailureStage != tasks.ExecutionStageProvider {
		t.Fatalf("failure_stage=%q, want %q", loaded.ExecutionMetrics.FailureStage, tasks.ExecutionStageProvider)
	}
	if loaded.ExecutionMetrics.FailureCategory != tasks.FailureCategoryTimeout {
		t.Fatalf("failure_category=%q, want %q", loaded.ExecutionMetrics.FailureCategory, tasks.FailureCategoryTimeout)
	}
	if loaded.ExecutionMetrics.BottleneckSource != tasks.BottleneckSourceProvider {
		t.Fatalf("bottleneck_source=%q, want %q", loaded.ExecutionMetrics.BottleneckSource, tasks.BottleneckSourceProvider)
	}
}

func TestCompleteWithOutputsWritesTerminalExecutionMetrics(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 95)
	channelID := accountID + 10000
	completedAt := time.Now()
	task := &tasks.Task{
		TaskID:                     "metrics-completed-with-outputs",
		UserID:                     10,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusPending,
		CreatedAt:                  completedAt.Add(-3 * time.Second),
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	const claimToken = "lease-metrics-completed"
	if _, err := taskRepo.ClaimAndStartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID:         task.TaskID,
		RiverJobID:     1,
		ExecutionToken: claimToken,
		FeatureSlug:    "generation",
		RequestedModel: "openai/gpt-image-1",
		StartedAt:      completedAt.Add(-2 * time.Second),
	}); err != nil {
		t.Fatalf("claim and start execution attempt failed: %v", err)
	}
	rows, err := taskRepo.ConvergeTask(ctx, completedTaskConvergenceIntent(task.TaskID, claimToken, `{"status":"success"}`, completedAt, nil, nil))
	if err != nil || rows != 1 {
		t.Fatalf("complete task rows=%d err=%v, want rows=1 nil err", rows, err)
	}

	loaded, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.ExecutionMetrics == nil {
		t.Fatal("execution metrics missing after completed terminal write")
	}
	if loaded.ExecutionMetrics.CurrentStage != tasks.ExecutionStageCompleted {
		t.Fatalf("current_stage=%q, want %q", loaded.ExecutionMetrics.CurrentStage, tasks.ExecutionStageCompleted)
	}
	if loaded.ExecutionMetrics.FailureStage != "" {
		t.Fatalf("failure_stage=%q, want empty", loaded.ExecutionMetrics.FailureStage)
	}
	if loaded.ExecutionMetrics.FailureCategory != "" {
		t.Fatalf("failure_category=%q, want empty", loaded.ExecutionMetrics.FailureCategory)
	}
	if loaded.ExecutionMetrics.TotalLatencyMS <= 0 {
		t.Fatalf("total_latency_ms=%d, want positive", loaded.ExecutionMetrics.TotalLatencyMS)
	}
	history, err := taskRepo.ListAdminTaskAttempts(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListAdminTaskAttempts failed: %v", err)
	}
	if len(history[task.TaskID]) != 1 ||
		history[task.TaskID][0].Status != tasks.TaskAttemptStatusCompleted ||
		history[task.TaskID][0].Stage != tasks.ExecutionStageCompleted {
		t.Fatalf("single-success execution trace=%+v", history[task.TaskID])
	}
}

func TestListExecutionBottleneckStatsByCreatedRangeOnlyUsesTerminalTasks(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 93)
	now := time.Now()
	seedTasks(t, taskRepo, ctx,
		&tasks.Task{TaskID: "metrics-running", UserID: 10, AccountID: testPtr(accountID), Status: tasks.StatusRunning, CreatedAt: now.Add(-5 * time.Minute)},
		&tasks.Task{TaskID: "metrics-completed", UserID: 10, Status: tasks.StatusCompleted, CreatedAt: now.Add(-4 * time.Minute)},
		&tasks.Task{TaskID: "metrics-failed", UserID: 10, Status: tasks.StatusFailed, CreatedAt: now.Add(-3 * time.Minute)},
		&tasks.Task{TaskID: "metrics-completed-missing", UserID: 10, Status: tasks.StatusCompleted, CreatedAt: now.Add(-2 * time.Minute)},
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

	bySource := map[string]statisticscontracts.ExecutionBottleneckAggregate{}
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
	if bySource[tasks.BottleneckSourceUnknown].Total != 1 || bySource[tasks.BottleneckSourceUnknown].Failed != 0 || bySource[tasks.BottleneckSourceUnknown].AvgLatencyMS != 0 {
		t.Fatalf("missing metrics should be grouped as unknown with zero latency, got %+v", bySource[tasks.BottleneckSourceUnknown])
	}
}

func TestListExecutionFailureCategoryStatsByCreatedRangeOnlyUsesFailedTasks(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	seedTasks(t, taskRepo, ctx,
		&tasks.Task{TaskID: "failure-category-completed", UserID: 10, Status: tasks.StatusCompleted, CreatedAt: now.Add(-4 * time.Minute)},
		&tasks.Task{TaskID: "failure-category-provider", UserID: 10, Status: tasks.StatusFailed, CreatedAt: now.Add(-3 * time.Minute)},
		&tasks.Task{TaskID: "failure-category-timeout", UserID: 10, Status: tasks.StatusFailed, CreatedAt: now.Add(-2 * time.Minute)},
		&tasks.Task{TaskID: "failure-category-missing", UserID: 10, Status: tasks.StatusFailed, CreatedAt: now.Add(-1 * time.Minute)},
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

	byCategory := map[string]statisticscontracts.ExecutionFailureCategoryAggregate{}
	for _, row := range rows {
		byCategory[row.Category] = row
	}
	if byCategory[tasks.FailureCategoryProviderError].Total != 1 {
		t.Fatalf("provider category should only count failed tasks, got %+v", byCategory[tasks.FailureCategoryProviderError])
	}
	if byCategory[tasks.FailureCategoryTimeout].Total != 1 {
		t.Fatalf("timeout category aggregate mismatch: %+v", byCategory[tasks.FailureCategoryTimeout])
	}
	if byCategory[tasks.FailureCategoryUnknown].Total != 1 || byCategory[tasks.FailureCategoryUnknown].AvgLatencyMS != 0 {
		t.Fatalf("missing metrics should be grouped as unknown with zero latency, got %+v", byCategory[tasks.FailureCategoryUnknown])
	}
}

func TestListMonitorGoldenSignalSamplesByRangeUsesExecutionMetrics(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	now := time.Now()
	completedAt := now.Add(-30 * time.Second)
	seedTasks(t, taskRepo, ctx,
		&tasks.Task{
			TaskID:      "golden-success",
			UserID:      10,
			Status:      tasks.StatusCompleted,
			CreatedAt:   now.Add(-2 * time.Minute),
			CompletedAt: &completedAt,
		},
		&tasks.Task{
			TaskID:      "golden-provider-failure",
			UserID:      10,
			Status:      tasks.StatusFailed,
			ErrorMsg:    "provider timeout",
			CreatedAt:   now.Add(-90 * time.Second),
			CompletedAt: &completedAt,
		},
	)

	providerStage := tasks.ExecutionStageProvider
	timeoutCategory := tasks.FailureCategoryTimeout
	providerStartedAt := now.Add(-80 * time.Second)
	for _, item := range []struct {
		taskID            string
		queueWaitMS       int64
		providerLatencyMS int64
		finalizeLatencyMS int64
		failureStage      *string
		failureCategory   *string
	}{
		{taskID: "golden-success", queueWaitMS: 120, providerLatencyMS: 900, finalizeLatencyMS: 40},
		{taskID: "golden-provider-failure", queueWaitMS: 300, providerLatencyMS: 1100, failureStage: &providerStage, failureCategory: &timeoutCategory},
	} {
		if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
			TaskID:            item.taskID,
			FailureStage:      item.failureStage,
			FailureCategory:   item.failureCategory,
			ProviderStartedAt: &providerStartedAt,
			QueueWaitMS:       &item.queueWaitMS,
			ProviderLatencyMS: &item.providerLatencyMS,
			FinalizeLatencyMS: &item.finalizeLatencyMS,
		}); err != nil {
			t.Fatalf("patch %s metrics failed: %v", item.taskID, err)
		}
	}

	rows, err := taskRepo.ListMonitorGoldenSignalSamplesByRange(ctx, now.Add(-10*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ListMonitorGoldenSignalSamplesByRange failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("samples=%d, want 2: %+v", len(rows), rows)
	}
	var providerErrors int
	var providerObserved int
	for _, row := range rows {
		if row.ProviderObserved {
			providerObserved++
		}
		if row.ProviderError {
			providerErrors++
		}
	}
	if providerObserved != 2 || providerErrors != 1 {
		t.Fatalf("provider observed/errors=%d/%d, want 2/1 rows=%+v", providerObserved, providerErrors, rows)
	}
}
