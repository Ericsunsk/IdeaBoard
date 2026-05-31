package accounts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStatisticsTestService(t *testing.T) (*AccountService, *TaskRepository, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:statistics_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&ApiAccount{}, &tasks.Task{}, &tasks.TaskOutput{}, &TaskDailyStat{}, &TaskStatEvent{}, &TaskDailyStatBackfill{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	taskRepo := NewTaskRepository(db)
	svc := NewAccountService(AccountServiceDeps{TaskRepo: taskRepo}, AccountServiceOptions{})
	return svc, taskRepo, db
}

func TestGetStatisticsReadsFromDailySnapshot(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	seed := []TaskDailyStat{
		{StatDate: today, ModelName: "model-a", UserKey: "u1", Total: 2, Completed: 2, Failed: 0, LatencySecondsSum: 30, LatencyCount: 2},
		{StatDate: today, ModelName: "model-b", UserKey: "u2", Total: 1, Completed: 0, Failed: 1, LatencySecondsSum: 0, LatencyCount: 0},
		{StatDate: yesterday, ModelName: "model-a", UserKey: "u1", Total: 1, Completed: 1, Failed: 0, LatencySecondsSum: 12, LatencyCount: 1},
	}
	for i := range seed {
		if err := taskRepo.db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed daily stat failed: %v", err)
		}
	}
	if err := taskRepo.Create(ctx, &tasks.Task{TaskID: "p1", UserID: 1, ChannelID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("seed pending task failed: %v", err)
	}
	if err := taskRepo.Create(ctx, &tasks.Task{TaskID: "r1", UserID: 1, ChannelID: 1, Status: tasks.StatusRunning}); err != nil {
		t.Fatalf("seed running task failed: %v", err)
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if stats.Summary.TodayCalls != 3 {
		t.Fatalf("today_calls = %d, want 3", stats.Summary.TodayCalls)
	}
	if stats.Summary.TodaySuccess != 2 || stats.Summary.TodayFailed != 1 {
		t.Fatalf("today success/failed = %d/%d", stats.Summary.TodaySuccess, stats.Summary.TodayFailed)
	}
	if stats.Summary.YesterdayCalls != 1 || stats.Summary.YesterdaySuccess != 1 {
		t.Fatalf("yesterday calls/success = %d/%d", stats.Summary.YesterdayCalls, stats.Summary.YesterdaySuccess)
	}
	if len(stats.ModelHealth) < 2 {
		t.Fatalf("expected model health rows, got %+v", stats.ModelHealth)
	}
	if stats.ModelHealth[0].Name != "model-b" || stats.ModelHealth[0].FailureRate != 100 {
		t.Fatalf("highest risk model should be model-b with 100%% failure rate, got %+v", stats.ModelHealth[0])
	}
	if stats.ModelHealth[1].Name != "model-a" || stats.ModelHealth[1].AvgSeconds != 14 {
		t.Fatalf("model-a health should include avg latency, got %+v", stats.ModelHealth[1])
	}
	if len(stats.UserRank) == 0 || stats.UserRank[0].Name != "u1" || stats.UserRank[0].Value != 3 {
		t.Fatalf("unexpected user ranking: %+v", stats.UserRank)
	}
	foundLatency := false
	for _, item := range stats.Latency {
		if item.Name == "model-a" {
			foundLatency = true
			if item.AvgSeconds <= 0 {
				t.Fatalf("model-a avg latency should be > 0")
			}
		}
	}
	if !foundLatency {
		t.Fatalf("expected model-a latency in response")
	}
}

func TestGetStatisticsHealthCellsAggregatedByDate(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	seed := []TaskDailyStat{
		{StatDate: today, ModelName: "m1", UserKey: "u1", Total: 2, Completed: 2, Failed: 0},
		{StatDate: today, ModelName: "m2", UserKey: "u2", Total: 3, Completed: 1, Failed: 2},
		{StatDate: yesterday, ModelName: "m1", UserKey: "u1", Total: 1, Completed: 1, Failed: 0},
	}
	for i := range seed {
		if err := taskRepo.db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed daily stat failed: %v", err)
		}
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if len(stats.HealthCells) != 7 {
		t.Fatalf("health cells len=%d, want 7", len(stats.HealthCells))
	}

	var foundToday bool
	for _, cell := range stats.HealthCells {
		if cell.Total == 5 {
			foundToday = true
			if cell.Success != 3 || cell.Fail != 2 {
				t.Fatalf("today health cell mismatch: %+v", cell)
			}
		}
	}
	if !foundToday {
		t.Fatalf("expected one aggregated today health cell total=5, got=%+v", stats.HealthCells)
	}
}

func TestGetStatisticsDoesNotBackfillWhenSnapshotEmpty(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	now := time.Now()
	created := now.Add(-2 * time.Hour)
	completed := created.Add(40 * time.Second)
	failedAt := created.Add(20 * time.Second)

	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:      "c1",
		UserID:      99,
		ChannelID:   1,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"m-backfill","username":"bf-user"}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed completed task failed: %v", err)
	}
	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:      "f1",
		UserID:      99,
		ChannelID:   1,
		Status:      tasks.StatusFailed,
		Result:      `{"model":"m-backfill","username":"bf-user"}`,
		CreatedAt:   created,
		CompletedAt: &failedAt,
	}); err != nil {
		t.Fatalf("seed failed task failed: %v", err)
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if stats.Summary.TodayCalls != 0 {
		t.Fatalf("today_calls = %d, want 0 before explicit backfill", stats.Summary.TodayCalls)
	}
	day := created.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("GetStatistics should be read-only; got rows=%+v", rows)
	}
}

func TestBackfillStatisticsBackfillsOnlyMissingTasksForSameDate(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	missingDayStart := todayStart.AddDate(0, 0, -1)

	alreadyCountedCompletedAt := missingDayStart.Add(80 * time.Second)
	alreadyCounted := &tasks.Task{
		TaskID:      "same-day-counted",
		UserID:      88,
		ChannelID:   1,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"model-missing","username":"u-missing"}`,
		CreatedAt:   missingDayStart.Add(20 * time.Second),
		CompletedAt: &alreadyCountedCompletedAt,
	}
	if err := taskRepo.Create(ctx, alreadyCounted); err != nil {
		t.Fatalf("seed already-counted task failed: %v", err)
	}
	if err := taskRepo.UpsertDailyStatByTask(ctx, alreadyCounted); err != nil {
		t.Fatalf("seed already-counted stat event failed: %v", err)
	}

	missingCompletedAt := missingDayStart.Add(120 * time.Second)
	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:      "same-day-missing",
		UserID:      88,
		ChannelID:   1,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"model-missing","username":"u-missing"}`,
		CreatedAt:   missingDayStart.Add(30 * time.Second),
		CompletedAt: &missingCompletedAt,
	}); err != nil {
		t.Fatalf("seed missing-day task failed: %v", err)
	}

	day := missingDayStart.Format("2006-01-02")
	beforeRows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list day before failed: %v", err)
	}
	if len(beforeRows) != 1 || beforeRows[0].Total != 1 {
		t.Fatalf("unexpected day rows before: %+v", beforeRows)
	}

	result, err := svc.BackfillStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	if result.ScannedDays < 1 {
		t.Fatalf("expected scanned_days >= 1, got %+v", result)
	}

	afterRows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list day after failed: %v", err)
	}
	if len(afterRows) != 1 || afterRows[0].Total != 2 || afterRows[0].Completed != 2 {
		t.Fatalf("same-day backfill should only add missing task once, rows=%+v", afterRows)
	}
}

func TestBackfillStatisticsMarkerSkipsPastDateOnSecondRun(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -2)
	completedAt := pastDay.Add(60 * time.Second)

	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:      "marker-task-1",
		UserID:      90,
		ChannelID:   1,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"marker-model","username":"marker-user"}`,
		CreatedAt:   pastDay.Add(15 * time.Second),
		CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("seed marker task failed: %v", err)
	}

	first, err := svc.BackfillStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("first BackfillStatistics failed: %v", err)
	}
	second, err := svc.BackfillStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("second BackfillStatistics failed: %v", err)
	}
	if first.BackfilledDays < 1 {
		t.Fatalf("first backfill should process past day marker: %+v", first)
	}
	if second.BackfilledDays != 0 {
		t.Fatalf("second backfill should skip already-marked past day: %+v", second)
	}

	day := pastDay.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list marker day rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 {
		t.Fatalf("marker day should remain stable after second run, rows=%+v", rows)
	}

	markers, err := taskRepo.ListBackfilledDatesByRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list backfill markers failed: %v", err)
	}
	if len(markers) != 1 || markers[0] != day {
		t.Fatalf("unexpected marker rows: %+v", markers)
	}
}

func TestGetStatisticsBackfillStatusReportsMissingDays(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, _ := newStatisticsTestService(t)
	now := time.Now()
	created := now.Add(-2 * time.Hour)
	completed := created.Add(20 * time.Second)
	if err := taskRepo.Create(ctx, &tasks.Task{
		TaskID:      "status-task-1",
		UserID:      101,
		ChannelID:   1,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"status-model","username":"status-user"}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	before, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics before backfill failed: %v", err)
	}
	if before.Backfill.MissingDays < 1 {
		t.Fatalf("expected missing days before backfill, got %+v", before.Backfill)
	}

	if _, err := svc.BackfillStatistics(ctx, "7d"); err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}

	after, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics after backfill failed: %v", err)
	}
	if after.Backfill.MissingDays != 0 {
		t.Fatalf("expected missing_days=0 after backfill status check, got after=%+v", after.Backfill)
	}
}

func TestExtractTaskModelPrefersModelAndAvoidsMixedModelSet(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "prefer model", raw: `{"model":"gpt-image-1","model_set":"a,b"}`, want: "gpt-image-1"},
		{name: "single model_set", raw: `{"model_set":"gpt-image-1"}`, want: "gpt-image-1"},
		{name: "mixed model_set", raw: `{"model_set":"a,b"}`, want: "未识别模型"},
		{name: "empty", raw: `{}`, want: "未识别模型"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskModel(tt.raw)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetStatisticsKeepsUnknownWhenAccountModelSetIsMulti(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	accountRepo := NewAccountRepository(db)
	svc.accountRepo = accountRepo

	account := &ApiAccount{
		Name:      "acc-multi",
		ChannelID: 1,
		Platform:  "openai-compatible",
		ApiType:   "openai-compatible",
		ModelSet:  "a,b",
		ApiKey:    "test",
		Status:    1,
	}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	created := time.Now().Add(-3 * time.Hour)
	completed := created.Add(8 * time.Second)
	task := &tasks.Task{
		TaskID:      "repair-multi-1",
		UserID:      12,
		ChannelID:   1,
		AccountID:   account.ID,
		Status:      tasks.StatusCompleted,
		Result:      `{}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	day := created.Format("2006-01-02")
	if err := db.Create(&TaskDailyStat{
		StatDate:  day,
		ModelName: "unknown",
		UserKey:   "12",
		Total:     1,
		Completed: 1,
		Failed:    0,
	}).Error; err != nil {
		t.Fatalf("seed unknown daily stat failed: %v", err)
	}

	if _, err := svc.GetStatistics(ctx, "7d"); err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	foundUnknown := false
	for _, row := range rows {
		if row.ModelName == "未识别模型" || row.ModelName == "unknown" || row.ModelName == "Unknown" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatalf("expected unresolved unknown model row for multi-model account, rows=%+v", rows)
	}
}
