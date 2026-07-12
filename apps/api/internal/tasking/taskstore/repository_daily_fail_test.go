package taskstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
)

func newRepositoryTestDB(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	db := testdb.Open(t)
	return NewRepository(db), db
}

func TestFailRunningByAccountIDUpsertsDailyStats(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 66)
	createdAt := time.Now().Add(-10 * time.Minute)

	if err := createRepositoryTestTask(t, taskRepo, ctx, &tasks.Task{
		TaskID:                     "acc-fail-1",
		UserID:                     10,
		AssignedProviderEndpointID: tasks.IDPtr(accountID + 10000),
		AccountID:                  tasks.IDPtr(accountID),
		FeatureSlug:                "image-generate",
		APIType:                    "openai-compatible",
		Status:                     tasks.StatusRunning,
		Result:                     `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt:                  createdAt,
	}); err != nil {
		t.Fatalf("seed task1 failed: %v", err)
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, &tasks.Task{
		TaskID:                     "acc-fail-2",
		UserID:                     10,
		AssignedProviderEndpointID: tasks.IDPtr(accountID + 10000),
		AccountID:                  tasks.IDPtr(accountID),
		FeatureSlug:                "image-generate",
		APIType:                    "openai-compatible",
		Status:                     tasks.StatusRunning,
		Result:                     `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt:                  createdAt,
	}); err != nil {
		t.Fatalf("seed task2 failed: %v", err)
	}
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	insertRepositoryActiveTaskLease(t, db, "acc-fail-1", accountID, accountID+10000, credentialID, "lease-acc-fail-1")
	insertRepositoryActiveTaskLease(t, db, "acc-fail-2", accountID, accountID+10000, credentialID, "lease-acc-fail-2")

	failedTaskIDs, err := taskRepo.FailRunningByAccountID(ctx, accountID, "stopped")
	if err != nil {
		t.Fatalf("FailRunningByAccountID failed: %v", err)
	}
	if len(failedTaskIDs) != 2 {
		t.Fatalf("failedTaskIDs=%v, want 2 tasks", failedTaskIDs)
	}
	failedSet := map[string]bool{}
	for _, taskID := range failedTaskIDs {
		failedSet[taskID] = true
	}
	if !failedSet["acc-fail-1"] || !failedSet["acc-fail-2"] {
		t.Fatalf("failedTaskIDs=%v, want acc-fail-1 and acc-fail-2", failedTaskIDs)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily rows=%d, want 1", len(rows))
	}
	if rows[0].Failed != 2 || rows[0].Total != 2 || rows[0].Completed != 0 {
		t.Fatalf("unexpected daily row: %+v", rows[0])
	}

	events := int64(0)
	if err := taskRepo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_stat_events`).Scan(&events); err != nil {
		t.Fatalf("count stat events failed: %v", err)
	}
	if events != 2 {
		t.Fatalf("event count=%d, want 2", events)
	}

	var userID, eventAccountID, channelID int64
	var featureSlug, apiType string
	if err := taskRepo.db.QueryRowContext(ctx, `
SELECT user_id, account_id, provider_endpoint_id, feature_slug, api_type
FROM task_stat_events
WHERE task_id = 'acc-fail-1'`,
	).Scan(&userID, &eventAccountID, &channelID, &featureSlug, &apiType); err != nil {
		t.Fatalf("read stat event dimensions failed: %v", err)
	}
	if userID != 10 || eventAccountID != accountID || channelID != accountID+10000 ||
		featureSlug != "image-generate" || apiType != "openai-compatible" {
		t.Fatalf("unexpected stat event dimensions: user=%d account=%d channel=%d feature=%q api=%q",
			userID, eventAccountID, channelID, featureSlug, apiType)
	}
}

func TestMarkTaskFailedUpsertsDailyStats(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-3 * time.Minute)
	completedAt := createdAt.Add(30 * time.Second)
	task := &tasks.Task{
		TaskID:    "mark-failed-stat",
		UserID:    11,
		Status:    tasks.StatusPending,
		Result:    `{"model":"model-fail","username":"user-fail"}`,
		CreatedAt: createdAt,
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	rowsAffected, err := taskRepo.MarkTaskFailed(ctx, task.TaskID, []string{tasks.StatusPending}, "failed", task.Result, completedAt, nil)
	if err != nil {
		t.Fatalf("MarkTaskFailed failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("rows=%d, want 1", rowsAffected)
	}
	rowsAffected, err = taskRepo.MarkTaskFailed(ctx, task.TaskID, []string{tasks.StatusPending}, "failed again", task.Result, completedAt.Add(time.Second), nil)
	if err != nil {
		t.Fatalf("second MarkTaskFailed failed: %v", err)
	}
	if rowsAffected != 0 {
		t.Fatalf("second terminal transition rows=%d, want 0", rowsAffected)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Failed != 1 || rows[0].Total != 1 || rows[0].Completed != 0 {
		t.Fatalf("unexpected failed daily row: %+v", rows)
	}
	var eventCount int64
	if err := taskRepo.db.QueryRowContext(ctx, `SELECT count(*) FROM task_stat_events WHERE task_id = $1`, task.TaskID).Scan(&eventCount); err != nil {
		t.Fatalf("count task stat events failed: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("task stat events=%d, want 1", eventCount)
	}
}

func TestMarkTaskCancelledUpsertsDailyStats(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-5 * time.Minute)
	cancelledAt := createdAt.Add(45 * time.Second)
	task := &tasks.Task{
		TaskID:    "mark-cancelled-stat",
		UserID:    12,
		Status:    tasks.StatusPending,
		Result:    `{"model":"model-cancel","username":"user-cancel"}`,
		CreatedAt: createdAt,
	}
	if err := createRepositoryTestTask(t, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	rowsAffected, err := taskRepo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      "user cancelled",
	}, nil)
	if err != nil {
		t.Fatalf("MarkTaskCancelled failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("rows=%d, want 1", rowsAffected)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Cancelled != 1 || rows[0].Total != 1 || rows[0].Completed != 0 || rows[0].Failed != 0 {
		t.Fatalf("unexpected cancelled daily row: %+v", rows)
	}
}

func TestUpsertDailyStatByTaskIsIdempotentByTaskID(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-1 * time.Minute)
	completedAt := createdAt.Add(10 * time.Second)
	task := &tasks.Task{
		TaskID:      "idem-1",
		UserID:      3,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"idem-model","username":"idem-user"}`,
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 || rows[0].Completed != 1 {
		t.Fatalf("idempotent aggregate expected 1 row/1 total: %+v", rows)
	}
}

func TestUpsertDailyStatByCancelledTaskIsIdempotentByTaskID(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-1 * time.Minute)
	cancelledAt := createdAt.Add(10 * time.Second)
	task := &tasks.Task{
		TaskID:      "idem-cancelled-1",
		UserID:      4,
		Status:      tasks.StatusCancelled,
		Result:      `{"model":"idem-cancelled-model","username":"idem-cancelled-user"}`,
		CreatedAt:   createdAt,
		CompletedAt: &cancelledAt,
		CancelledAt: &cancelledAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 || rows[0].Cancelled != 1 || rows[0].Failed != 0 || rows[0].Completed != 0 {
		t.Fatalf("idempotent cancelled aggregate expected 1 row/1 cancelled: %+v", rows)
	}
}

func TestUpsertDailyStatByTaskWithoutResolvedModelUsesUnknownModel(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt.Add(15 * time.Second)
	task := &tasks.Task{
		TaskID:      "fallback-1",
		UserID:      7,
		AccountID:   tasks.IDPtr(12345),
		Status:      tasks.StatusCompleted,
		Result:      `{}`,
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("upsert without resolved model failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	if rows[0].ModelName != unknownModelName || rows[0].UserKey != "7" {
		t.Fatalf("stats should use normalized task snapshot dimensions, got %+v", rows[0])
	}
}

func TestUpsertDailyStatByTaskUsesSnapshotModelNotResultJSON(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt.Add(15 * time.Second)
	task := &tasks.Task{
		TaskID:        "snapshot-model-not-result-json",
		UserID:        7,
		Status:        tasks.StatusCompleted,
		ResolvedModel: "snapshot-model",
		Result:        `{"model":"json-model","username":"json-user"}`,
		CreatedAt:     createdAt,
		CompletedAt:   &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("upsert snapshot model task failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelName != "snapshot-model" || rows[0].UserKey != "7" {
		t.Fatalf("stats should use task snapshot dimensions, got %+v", rows)
	}
}

func TestUpsertDailyStatByTaskNormalizesLongDimensions(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt.Add(20 * time.Second)
	longModel := strings.Repeat("模", maxStatDimensionSize+10)
	task := &tasks.Task{
		TaskID:        "long-stat-dimensions",
		UserID:        17,
		Status:        tasks.StatusCompleted,
		ResolvedModel: longModel,
		CreatedAt:     createdAt,
		CompletedAt:   &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("upsert with long dimensions failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	if want := strings.Repeat("模", maxStatDimensionSize); rows[0].ModelName != want {
		t.Fatalf("model_name length/content mismatch: got %q want %q", rows[0].ModelName, want)
	}
	if rows[0].UserKey != "17" {
		t.Fatalf("user_key=%q, want normalized task user id", rows[0].UserKey)
	}
}

func TestRebuildDailyStatsFromEventsByDateRangeReconstructsProjection(t *testing.T) {
	ctx := context.Background()
	taskRepo, db := newRepositoryTestDB(t)
	occurredAt := time.Now().Add(-2 * time.Hour)
	statDate := taskStatDateFromTime(occurredAt, time.Now().Location())

	if _, err := db.ExecContext(ctx, `
INSERT INTO task_daily_stats (stat_date, model_name, user_key, total, completed, failed, cancelled)
VALUES ($1, 'stale-model', 'stale-user', 9, 9, 0, 0)`, statDate); err != nil {
		t.Fatalf("seed stale daily stat failed: %v", err)
	}
	for _, correction := range []TaskStatEventCorrection{
		{TaskID: "rebuild-event-1", OccurredAt: occurredAt, UserID: 41, ModelName: "event-model", UserKey: "41", Status: tasks.StatusCompleted, LatencySeconds: 12, CostMicros: 100},
		{TaskID: "rebuild-event-2", OccurredAt: occurredAt.Add(time.Minute), UserID: 41, ModelName: "event-model", UserKey: "41", Status: tasks.StatusFailed, CostMicros: 200},
	} {
		if _, err := taskRepo.CorrectTaskStatEvent(ctx, correction); err != nil {
			t.Fatalf("seed stat event failed: %v", err)
		}
	}

	result, err := taskRepo.RebuildDailyStatsFromEventsByDateRange(ctx, statDate, statDate)
	if err != nil {
		t.Fatalf("rebuild daily stats failed: %v", err)
	}
	if result.DeletedGroups != 1 || result.RebuiltGroups != 1 {
		t.Fatalf("rebuild result mismatch: %+v", result)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, statDate, statDate)
	if err != nil {
		t.Fatalf("list rebuilt daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelName != "event-model" || rows[0].Total != 2 || rows[0].Completed != 1 || rows[0].Failed != 1 || rows[0].CostMicrosSum != 300 {
		t.Fatalf("rebuilt projection mismatch: %+v", rows)
	}
}

func TestCorrectTaskStatEventAndRebuildUpdatesProjection(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	occurredAt := time.Now().Add(-time.Hour)
	statDate := taskStatDateFromTime(occurredAt, time.Now().Location())

	if _, err := taskRepo.CorrectTaskStatEvent(ctx, TaskStatEventCorrection{
		TaskID:         "correction-event-1",
		OccurredAt:     occurredAt,
		UserID:         42,
		ModelName:      "old-model",
		UserKey:        "42",
		Status:         tasks.StatusCompleted,
		LatencySeconds: 7,
		CostMicros:     100,
	}); err != nil {
		t.Fatalf("seed original stat event failed: %v", err)
	}
	if _, err := taskRepo.RebuildDailyStatsFromEventsByDateRange(ctx, statDate, statDate); err != nil {
		t.Fatalf("initial rebuild failed: %v", err)
	}

	if _, err := taskRepo.CorrectTaskStatEvent(ctx, TaskStatEventCorrection{
		TaskID:     "correction-event-1",
		OccurredAt: occurredAt,
		UserID:     42,
		ModelName:  "corrected-model",
		UserKey:    "42",
		Status:     tasks.StatusFailed,
		CostMicros: 250,
	}); err != nil {
		t.Fatalf("correct stat event failed: %v", err)
	}
	if _, err := taskRepo.RebuildDailyStatsFromEventsByDateRange(ctx, statDate, statDate); err != nil {
		t.Fatalf("rebuild after correction failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, statDate, statDate)
	if err != nil {
		t.Fatalf("list corrected daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelName != "corrected-model" || rows[0].Total != 1 || rows[0].Completed != 0 || rows[0].Failed != 1 || rows[0].LatencyCount != 0 || rows[0].CostMicrosSum != 250 {
		t.Fatalf("corrected projection mismatch: %+v", rows)
	}
}

func TestUpsertDailyStatByTaskIgnoresZeroLatencySample(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt
	task := &tasks.Task{
		TaskID:      "zero-latency-stat",
		UserID:      18,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"zero-latency-model","username":"zero-latency-user"}`,
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(ctx, task); err != nil {
		t.Fatalf("upsert zero latency task failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	if rows[0].Completed != 1 || rows[0].LatencyCount != 0 || rows[0].LatencySecondsSum != 0 {
		t.Fatalf("zero latency should not contribute latency sample: %+v", rows[0])
	}
}
