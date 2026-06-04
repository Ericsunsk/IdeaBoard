package taskstats

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/taskstore"
	"ideaboard/api/internal/testdb"
)

func newStatisticsTestService(t *testing.T) (*Service, *taskstore.Repository, *sql.DB) {
	t.Helper()

	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	svc := NewService(taskRepo)
	return svc, taskRepo, db
}

func TestGetStatisticsReadsFromDailySnapshot(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	seed := []taskDailyStatSeed{
		{StatDate: today, ModelName: "model-a", UserKey: "u1", Total: 2, Completed: 2, Failed: 0, LatencySecondsSum: 30, LatencyCount: 2},
		{StatDate: today, ModelName: "model-b", UserKey: "u2", Total: 1, Completed: 0, Failed: 1, LatencySecondsSum: 0, LatencyCount: 0},
		{StatDate: yesterday, ModelName: "model-a", UserKey: "u1", Total: 1, Completed: 1, Failed: 0, LatencySecondsSum: 12, LatencyCount: 1},
	}
	for i := range seed {
		insertTaskDailyStat(t, db, seed[i])
	}
	events := []taskStatEventSeed{
		{TaskID: "latency-a-1", StatDate: today, ModelName: "model-a", UserKey: "u1", Status: tasks.StatusCompleted, LatencySeconds: 18},
		{TaskID: "latency-a-2", StatDate: today, ModelName: "model-a", UserKey: "u1", Status: tasks.StatusCompleted, LatencySeconds: 30},
		{TaskID: "latency-a-3", StatDate: yesterday, ModelName: "model-a", UserKey: "u1", Status: tasks.StatusCompleted, LatencySeconds: 12},
		{TaskID: "latency-zero", StatDate: today, ModelName: "model-b", UserKey: "u2", Status: tasks.StatusCompleted, LatencySeconds: 0},
	}
	for i := range events {
		insertTaskStatEvent(t, db, events[i])
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
	if len(stats.UserRisk) == 0 || stats.UserRisk[0].Name != "u2" || stats.UserRisk[0].Failed != 1 {
		t.Fatalf("unexpected user risk ranking: %+v", stats.UserRisk)
	}
	foundSlowModel := false
	for _, item := range stats.SlowModels {
		if item.Name == "model-a" {
			foundSlowModel = true
			if item.AvgSeconds <= 0 {
				t.Fatalf("model-a avg latency should be > 0")
			}
			if item.P95Seconds != 30 || item.Samples != 3 {
				t.Fatalf("model-a p95/samples mismatch: %+v", item)
			}
		}
	}
	if !foundSlowModel {
		t.Fatalf("expected model-a slow model in response")
	}
}

func TestGetStatisticsQualityTrendAggregatedByDate(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newStatisticsTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	seed := []taskDailyStatSeed{
		{StatDate: today, ModelName: "m1", UserKey: "u1", Total: 2, Completed: 2, Failed: 0},
		{StatDate: today, ModelName: "m2", UserKey: "u2", Total: 3, Completed: 1, Failed: 2},
		{StatDate: yesterday, ModelName: "m1", UserKey: "u1", Total: 1, Completed: 1, Failed: 0},
	}
	for i := range seed {
		insertTaskDailyStat(t, db, seed[i])
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if len(stats.QualityTrend) != 7 {
		t.Fatalf("quality trend len=%d, want 7", len(stats.QualityTrend))
	}

	var foundToday bool
	for _, item := range stats.QualityTrend {
		if item.Date == today {
			foundToday = true
			if item.Total != 5 || item.Success != 3 || item.Fail != 2 || item.SuccessRate != 60 {
				t.Fatalf("today quality trend mismatch: %+v", item)
			}
		}
	}
	if !foundToday {
		t.Fatalf("expected one aggregated today quality trend item, got=%+v", stats.QualityTrend)
	}
}

func TestGetStatisticsUserRiskRanksByFailureVolume(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newStatisticsTestService(t)
	today := time.Now().Format("2006-01-02")

	seed := []taskDailyStatSeed{
		{StatDate: today, ModelName: "m1", UserKey: "low-volume", Total: 1, Completed: 0, Failed: 1},
		{StatDate: today, ModelName: "m1", UserKey: "high-volume", Total: 100, Completed: 50, Failed: 50},
	}
	for i := range seed {
		insertTaskDailyStat(t, db, seed[i])
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if len(stats.UserRisk) == 0 || stats.UserRisk[0].Name != "high-volume" {
		t.Fatalf("user risk should prioritize failure volume, got %+v", stats.UserRisk)
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

func TestGetStatisticsKeepsUnknownWhenAccountModelSetIsMulti(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	accountRepo := accounts.NewAccountRepository(db)

	account := &accounts.ApiAccount{
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
	insertTaskDailyStat(t, db, taskDailyStatSeed{
		StatDate:  day,
		ModelName: "unknown",
		UserKey:   "12",
		Total:     1,
		Completed: 1,
		Failed:    0,
	})

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

type taskDailyStatSeed struct {
	StatDate          string
	ModelName         string
	UserKey           string
	Total             int64
	Completed         int64
	Failed            int64
	LatencySecondsSum float64
	LatencyCount      int64
}

type taskStatEventSeed struct {
	TaskID         string
	StatDate       string
	ModelName      string
	UserKey        string
	Status         string
	LatencySeconds float64
}

func insertTaskDailyStat(t *testing.T, db *sql.DB, stat taskDailyStatSeed) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO task_daily_stats (stat_date, model_name, user_key, total, completed, failed, latency_seconds_sum, latency_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		stat.StatDate, stat.ModelName, stat.UserKey, stat.Total, stat.Completed, stat.Failed, stat.LatencySecondsSum, stat.LatencyCount,
	)
	if err != nil {
		t.Fatalf("seed daily stat failed: %v", err)
	}
}

func insertTaskStatEvent(t *testing.T, db *sql.DB, event taskStatEventSeed) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO task_stat_events (task_id, stat_date, model_name, user_key, status, latency_seconds)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		event.TaskID, event.StatDate, event.ModelName, event.UserKey, event.Status, event.LatencySeconds,
	)
	if err != nil {
		t.Fatalf("seed stat event failed: %v", err)
	}
}
