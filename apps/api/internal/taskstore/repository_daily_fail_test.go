package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
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

	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:            "acc-fail-1",
		UserID:            10,
		AssignedChannelID: tasks.IDPtr(accountID + 10000),
		AccountID:         tasks.IDPtr(accountID),
		FeatureSlug:       "image-generate",
		APIType:           "openai-compatible",
		Status:            tasks.StatusRunning,
		Result:            `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt:         createdAt,
	}); err != nil {
		t.Fatalf("seed task1 failed: %v", err)
	}
	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:            "acc-fail-2",
		UserID:            10,
		AssignedChannelID: tasks.IDPtr(accountID + 10000),
		AccountID:         tasks.IDPtr(accountID),
		FeatureSlug:       "image-generate",
		APIType:           "openai-compatible",
		Status:            tasks.StatusRunning,
		Result:            `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt:         createdAt,
	}); err != nil {
		t.Fatalf("seed task2 failed: %v", err)
	}

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
SELECT user_id, account_id, channel_id, feature_slug, api_type
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
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	rowsAffected, err := taskRepo.MarkTaskFailed(ctx, task.TaskID, []string{tasks.StatusPending}, "failed", task.Result, completedAt, nil)
	if err != nil {
		t.Fatalf("MarkTaskFailed failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("rows=%d, want 1", rowsAffected)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Failed != 1 || rows[0].Total != 1 || rows[0].Completed != 0 {
		t.Fatalf("unexpected failed daily row: %+v", rows)
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
	if err := taskRepo.Create(ctx, task); err != nil {
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

func TestUpsertDailyStatByTaskWithFallbackModelUsesProvidedSingleModel(t *testing.T) {
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

	if err := taskRepo.UpsertDailyStatByTaskWithFallbackModel(ctx, task, "model-fallback"); err != nil {
		t.Fatalf("upsert with fallback failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, createdAt, createdAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	if rows[0].ModelName != "model-fallback" {
		t.Fatalf("model_name=%q, want model-fallback", rows[0].ModelName)
	}
}

func TestUpsertDailyStatByTaskNormalizesLongDimensions(t *testing.T) {
	ctx := context.Background()
	taskRepo, _ := newRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt.Add(20 * time.Second)
	longModel := strings.Repeat("模", maxStatDimensionSize+10)
	longUser := strings.Repeat("用", maxStatDimensionSize+10)
	task := &tasks.Task{
		TaskID:      "long-stat-dimensions",
		UserID:      17,
		Status:      tasks.StatusCompleted,
		Result:      fmt.Sprintf(`{"model":%q,"username":%q}`, longModel, longUser),
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
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
	if want := strings.Repeat("用", maxStatDimensionSize); rows[0].UserKey != want {
		t.Fatalf("user_key length/content mismatch: got %q want %q", rows[0].UserKey, want)
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
