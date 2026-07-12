package taskstats

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func newStatisticsTestService(t *testing.T) (*Service, *taskstore.Repository, *sql.DB) {
	t.Helper()

	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	svc := NewService(taskRepo)
	return svc, taskRepo, db
}

func seedStatisticsAccount(t testing.TB, ctx context.Context, db *sql.DB, name string) *accounts.ApiAccount {
	t.Helper()
	accountRepo := accounts.NewAccountRepository(db)
	var channelID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`, name+"-channel", "https://example.com/"+name).Scan(&channelID); err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	account := &accounts.ApiAccount{
		Name:               name,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "test",
		ProxyURL:           "https://example.com/" + name,
		Status:             1,
	}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return account
}

func TestGetStatisticsReadsFromDailySnapshot(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	seed := []taskDailyStatSeed{
		{StatDate: today, ModelName: "model-a", UserKey: "u1", Total: 2, Completed: 2, Failed: 0, LatencySecondsSum: 30, LatencyCount: 2},
		{StatDate: today, ModelName: "model-b", UserKey: "u2", Total: 2, Completed: 0, Failed: 1, Cancelled: 1, LatencySecondsSum: 0, LatencyCount: 0},
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
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{TaskID: "p1", UserID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("seed pending task failed: %v", err)
	}
	runningAccount := seedStatisticsAccount(t, ctx, db, "stats-running-account")
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:                     "r1",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(runningAccount.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(runningAccount.ID),
		Status:                     tasks.StatusRunning,
	}); err != nil {
		t.Fatalf("seed running task failed: %v", err)
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if stats.Summary.TodayCalls != 4 {
		t.Fatalf("today_calls = %d, want 4", stats.Summary.TodayCalls)
	}
	if stats.Summary.TodaySuccess != 2 || stats.Summary.TodayFailed != 1 || stats.Summary.TodayCancelled != 1 {
		t.Fatalf("today success/failed/cancelled = %d/%d/%d", stats.Summary.TodaySuccess, stats.Summary.TodayFailed, stats.Summary.TodayCancelled)
	}
	if stats.Summary.YesterdayCalls != 1 || stats.Summary.YesterdaySuccess != 1 {
		t.Fatalf("yesterday calls/success = %d/%d", stats.Summary.YesterdayCalls, stats.Summary.YesterdaySuccess)
	}
	if stats.Summary.ThisMonthCancelled != 1 {
		t.Fatalf("this_month_cancelled = %d, want 1", stats.Summary.ThisMonthCancelled)
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
		{StatDate: today, ModelName: "m2", UserKey: "u2", Total: 4, Completed: 1, Failed: 2, Cancelled: 1},
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
			if item.Total != 6 || item.Success != 3 || item.Fail != 2 || item.SuccessRate != 60 {
				t.Fatalf("today quality trend mismatch: %+v", item)
			}
		}
	}
	if !foundToday {
		t.Fatalf("expected one aggregated today quality trend item, got=%+v", stats.QualityTrend)
	}
}

func TestGetStatisticsOneDayTrendUsesFourHourBuckets(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	today := todayStart.Format(statisticsDateLayout)

	seed := []struct {
		id         string
		status     string
		model      string
		occurredAt time.Time
	}{
		{id: "bucket-00-completed", status: tasks.StatusCompleted, model: "model-a", occurredAt: todayStart.Add(1 * time.Hour)},
		{id: "bucket-04-failed", status: tasks.StatusFailed, model: "model-b", occurredAt: todayStart.Add(5 * time.Hour)},
		{id: "bucket-20-completed", status: tasks.StatusCompleted, model: "model-a", occurredAt: todayStart.Add(21 * time.Hour)},
	}
	for _, item := range seed {
		insertTaskStatEvent(t, db, taskStatEventSeed{
			TaskID:         item.id,
			OccurredAt:     item.occurredAt,
			StatDate:       today,
			ModelName:      item.model,
			UserKey:        "1",
			Status:         item.status,
			LatencySeconds: 30,
		})
	}

	stats, err := svc.GetStatistics(ctx, "1d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if len(stats.QualityTrend) != 6 {
		t.Fatalf("quality trend len=%d, want 6", len(stats.QualityTrend))
	}
	if stats.QualityTrend[0].Label != "00:00" || stats.QualityTrend[0].Total != 1 || stats.QualityTrend[0].Success != 1 {
		t.Fatalf("00:00 bucket mismatch: %+v", stats.QualityTrend[0])
	}
	if stats.QualityTrend[1].Label != "04:00" || stats.QualityTrend[1].Total != 1 || stats.QualityTrend[1].Fail != 1 {
		t.Fatalf("04:00 bucket mismatch: %+v", stats.QualityTrend[1])
	}
	if stats.QualityTrend[5].Label != "20:00" || stats.QualityTrend[5].Total != 1 || stats.QualityTrend[5].Success != 1 {
		t.Fatalf("20:00 bucket mismatch: %+v", stats.QualityTrend[5])
	}
	if len(stats.Trend) != 6 || len(stats.Trend[0].Segments) != 1 || stats.Trend[0].Segments[0].Label != "model-a" {
		t.Fatalf("trend model bucket mismatch: %+v", stats.Trend)
	}
}

func TestGetStatisticsFailureCategoriesRankByFailureVolume(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	now := time.Now()
	totalLatencyMS := int64(3000)
	providerCategory := tasks.FailureCategoryProviderError
	timeoutCategory := tasks.FailureCategoryTimeout

	seed := []struct {
		id       string
		category string
	}{
		{id: "provider-1", category: providerCategory},
		{id: "provider-2", category: providerCategory},
		{id: "timeout-1", category: timeoutCategory},
	}
	for _, item := range seed {
		if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
			TaskID:    item.id,
			UserID:    1,
			Status:    tasks.StatusFailed,
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed failed task failed: %v", err)
		}
		if err := taskRepo.PatchExecutionMetrics(ctx, tasks.TaskExecutionMetricsPatch{
			TaskID:          item.id,
			FailureCategory: &item.category,
			TotalLatencyMS:  &totalLatencyMS,
		}); err != nil {
			t.Fatalf("patch execution metrics failed: %v", err)
		}
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if len(stats.Failures) < 2 {
		t.Fatalf("expected failure category stats, got %+v", stats.Failures)
	}
	if stats.Failures[0].Category != providerCategory || stats.Failures[0].Total != 2 {
		t.Fatalf("provider category should rank first by failures, got %+v", stats.Failures)
	}
	if stats.Failures[0].Label != "上游错误" {
		t.Fatalf("provider category label mismatch: %+v", stats.Failures[0])
	}
}

func TestGetStatisticsDoesNotBackfillWhenSnapshotEmpty(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	now := time.Now()
	created := now.Add(-2 * time.Hour)
	completed := created.Add(40 * time.Second)
	failedAt := created.Add(20 * time.Second)

	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "c1",
		UserID:      99,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"m-backfill","username":"bf-user"}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed completed task failed: %v", err)
	}
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "f1",
		UserID:      99,
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
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, created, created)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("GetStatistics should be read-only; got rows=%+v", rows)
	}
}

func TestBackfillStatisticsBackfillsOnlyMissingTasksForSameDate(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	missingDayStart := todayStart.AddDate(0, 0, -1)

	alreadyCountedCompletedAt := missingDayStart.Add(80 * time.Second)
	alreadyCounted := &tasks.Task{
		TaskID:      "same-day-counted",
		UserID:      88,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"model-missing","username":"u-missing"}`,
		CreatedAt:   missingDayStart.Add(20 * time.Second),
		CompletedAt: &alreadyCountedCompletedAt,
	}
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, alreadyCounted); err != nil {
		t.Fatalf("seed already-counted task failed: %v", err)
	}
	if err := taskRepo.UpsertDailyStatByTask(ctx, alreadyCounted); err != nil {
		t.Fatalf("seed already-counted stat event failed: %v", err)
	}

	missingCompletedAt := missingDayStart.Add(120 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "same-day-missing",
		UserID:      88,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"model-missing","username":"u-missing"}`,
		CreatedAt:   missingDayStart.Add(30 * time.Second),
		CompletedAt: &missingCompletedAt,
	}); err != nil {
		t.Fatalf("seed missing-day task failed: %v", err)
	}

	beforeRows, err := taskRepo.ListDailyStatsByDateRange(ctx, missingDayStart, missingDayStart)
	if err != nil {
		t.Fatalf("list day before failed: %v", err)
	}
	if len(beforeRows) != 1 || beforeRows[0].Total != 1 {
		t.Fatalf("unexpected day rows before: %+v", beforeRows)
	}
	beforeEvents := countTaskStatEventsByDate(t, db, missingDayStart)
	if beforeEvents != 1 {
		t.Fatalf("stat events before=%d, want 1", beforeEvents)
	}

	result, err := svc.BackfillStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	if result.ScannedDays < 1 {
		t.Fatalf("expected scanned_days >= 1, got %+v", result)
	}

	afterRows, err := taskRepo.ListDailyStatsByDateRange(ctx, missingDayStart, missingDayStart)
	if err != nil {
		t.Fatalf("list day after failed: %v", err)
	}
	if len(afterRows) != 1 || afterRows[0].Total != 2 || afterRows[0].Completed != 2 {
		t.Fatalf("same-day backfill should only add missing task once, rows=%+v", afterRows)
	}
	afterEvents := countTaskStatEventsByDate(t, db, missingDayStart)
	if afterEvents != 2 {
		t.Fatalf("stat events after=%d, want 2", afterEvents)
	}
}

func TestBackfillStatisticsMarkerSkipsPastDateOnSecondRun(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -2)
	completedAt := pastDay.Add(60 * time.Second)

	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "marker-task-1",
		UserID:      90,
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
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list marker day rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 {
		t.Fatalf("marker day should remain stable after second run, rows=%+v", rows)
	}

	markers, err := taskRepo.ListBackfilledDatesByRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list backfill markers failed: %v", err)
	}
	if len(markers) != 1 || statisticsDateKey(markers[0]) != day {
		t.Fatalf("unexpected marker rows: %+v", markers)
	}
}

func TestBackfillStatisticsAlwaysScansTodayButDoesNotDuplicateEvents(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	created := todayStart.Add(2 * time.Hour)
	completed := created.Add(25 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "today-backfill-idempotent",
		UserID:      91,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"today-model","username":"today-user"}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed today task failed: %v", err)
	}

	first, err := svc.BackfillStatistics(ctx, "1d")
	if err != nil {
		t.Fatalf("first BackfillStatistics failed: %v", err)
	}
	second, err := svc.BackfillStatistics(ctx, "1d")
	if err != nil {
		t.Fatalf("second BackfillStatistics failed: %v", err)
	}
	if first.ScannedDays != 1 || second.ScannedDays != 1 {
		t.Fatalf("today should be scanned every run, first=%+v second=%+v", first, second)
	}
	if first.BackfilledDays != 0 || second.BackfilledDays != 0 {
		t.Fatalf("today should not be marked done, first=%+v second=%+v", first, second)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, todayStart, todayStart)
	if err != nil {
		t.Fatalf("list today rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 || rows[0].Completed != 1 {
		t.Fatalf("today daily stats should not duplicate, rows=%+v", rows)
	}
	if events := countTaskStatEventsByDate(t, db, todayStart); events != 1 {
		t.Fatalf("today stat events=%d, want 1", events)
	}
}

func TestBackfillStatisticsIncludesCancelledWithoutFailure(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -1)
	cancelledAt := pastDay.Add(90 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "cancelled-backfill-1",
		UserID:      92,
		Status:      tasks.StatusCancelled,
		Result:      `{"model":"cancelled-model","username":"cancelled-user"}`,
		CreatedAt:   pastDay.Add(30 * time.Second),
		CompletedAt: &cancelledAt,
	}); err != nil {
		t.Fatalf("seed cancelled task failed: %v", err)
	}

	if _, err := svc.BackfillStatistics(ctx, "7d"); err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list cancelled day rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 || rows[0].Cancelled != 1 || rows[0].Completed != 0 || rows[0].Failed != 0 {
		t.Fatalf("cancelled task stats mismatch: %+v", rows)
	}

	stats, err := svc.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	day := statisticsDateKey(pastDay)
	for _, item := range stats.QualityTrend {
		if item.Date == day {
			if item.Total != 1 || item.Success != 0 || item.Fail != 0 || item.SuccessRate != 0 {
				t.Fatalf("cancelled quality trend mismatch: %+v", item)
			}
			return
		}
	}
	t.Fatalf("quality trend missing cancelled day %s: %+v", day, stats.QualityTrend)
}

func TestBackfillStatisticsDoesNotUseSingleEnabledAccountModelFallback(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	account := seedStatisticsAccount(t, ctx, db, "stats-single-model-fallback")
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -1)
	created := pastDay.Add(45 * time.Second)
	completed := created.Add(8 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:                     "single-model-fallback",
		UserID:                     93,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		FeatureSlug:                "fusion",
		APIType:                    "openai-compatible",
		Status:                     tasks.StatusCompleted,
		Result:                     `{}`,
		CreatedAt:                  created,
		CompletedAt:                &completed,
	}); err != nil {
		t.Fatalf("seed fallback task failed: %v", err)
	}

	if _, err := svc.BackfillStatistics(ctx, "7d"); err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list fallback day rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelName != "未识别模型" || rows[0].UserKey != "93" || rows[0].Total != 1 {
		t.Fatalf("stats should not infer model from account fallback: %+v", rows)
	}

	var userID, accountID, channelID int64
	var featureSlug, apiType string
	if err := db.QueryRowContext(ctx, `
SELECT user_id, account_id, provider_endpoint_id, feature_slug, api_type
FROM task_stat_events
WHERE task_id = 'single-model-fallback'`,
	).Scan(&userID, &accountID, &channelID, &featureSlug, &apiType); err != nil {
		t.Fatalf("read backfill stat event dimensions failed: %v", err)
	}
	if userID != 93 || accountID != account.ID || channelID != account.ProviderEndpointID ||
		featureSlug != "fusion" || apiType != "" {
		t.Fatalf("unexpected backfill event dimensions: user=%d account=%d channel=%d feature=%q api=%q",
			userID, accountID, channelID, featureSlug, apiType)
	}
}

func TestBackfillStatisticsIgnoresInvalidResultJSON(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -1)
	created := pastDay.Add(65 * time.Second)
	completed := created.Add(9 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "invalid-json-backfill",
		UserID:      94,
		Status:      tasks.StatusCompleted,
		Result:      `{bad-json`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed invalid json task failed: %v", err)
	}

	if _, err := svc.BackfillStatistics(ctx, "7d"); err != nil {
		t.Fatalf("BackfillStatistics should ignore invalid result JSON: %v", err)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list invalid-json day rows failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelName != "未识别模型" || rows[0].UserKey != "94" || rows[0].Total != 1 {
		t.Fatalf("invalid result JSON fallback mismatch: %+v", rows)
	}
}

func TestBackfillStatisticsMatchesRealtimeDimensionExtraction(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	loc := time.Now().Location()
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	pastDay := todayStart.AddDate(0, 0, -1)
	created := pastDay.Add(75 * time.Second)
	completed := created
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "non-string-result-dimensions",
		UserID:      95,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":{"name":"bad"},"username":["bad"],"email":false,"user_id":0}`,
		CreatedAt:   created,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatalf("seed non-string result task failed: %v", err)
	}

	if _, err := svc.BackfillStatistics(ctx, "7d"); err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, pastDay, pastDay)
	if err != nil {
		t.Fatalf("list non-string-result day rows failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1: %+v", len(rows), rows)
	}
	if rows[0].ModelName != "未识别模型" || rows[0].UserKey != "95" {
		t.Fatalf("non-string result dimensions should fall back like realtime path: %+v", rows[0])
	}
	if rows[0].Completed != 1 || rows[0].LatencyCount != 0 || rows[0].LatencySecondsSum != 0 {
		t.Fatalf("zero latency backfill should not contribute latency sample: %+v", rows[0])
	}
}

func TestGetStatisticsBackfillStatusReportsMissingDays(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	now := time.Now()
	created := now.Add(-2 * time.Hour)
	completed := created.Add(20 * time.Second)
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, &tasks.Task{
		TaskID:      "status-task-1",
		UserID:      101,
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

func TestGetStatisticsKeepsUnknownWhenAccountHasMultipleModels(t *testing.T) {
	ctx := context.Background()
	svc, taskRepo, db := newStatisticsTestService(t)
	accountRepo := accounts.NewAccountRepository(db)
	var channelID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES ('openai-compatible', 'stats-channel-multi', 'https://example.com/stats-multi', 1)
RETURNING id`).Scan(&channelID); err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	account := &accounts.ApiAccount{
		Name:               "acc-multi",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models: []accounts.AccountModel{
			{Name: "a", Enabled: true},
			{Name: "b", Enabled: true},
		},
		ApiKey:   "test",
		ProxyURL: "https://example.com/multi",
		Status:   1,
	}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	created := time.Now().Add(-3 * time.Hour)
	completed := created.Add(8 * time.Second)
	task := &tasks.Task{
		TaskID:                     "repair-multi-1",
		UserID:                     12,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusCompleted,
		Result:                     `{}`,
		CreatedAt:                  created,
		CompletedAt:                &completed,
	}
	if err := createTaskStatsTestTask(t, db, taskRepo, ctx, task); err != nil {
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

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, created, created)
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
	Cancelled         int64
	LatencySecondsSum float64
	LatencyCount      int64
}

type taskStatEventSeed struct {
	TaskID         string
	OccurredAt     time.Time
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
		`INSERT INTO task_daily_stats (stat_date, model_name, user_key, total, completed, failed, cancelled, latency_seconds_sum, latency_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		stat.StatDate, stat.ModelName, stat.UserKey, stat.Total, stat.Completed, stat.Failed, stat.Cancelled, stat.LatencySecondsSum, stat.LatencyCount,
	)
	if err != nil {
		t.Fatalf("seed daily stat failed: %v", err)
	}
}

func insertTaskStatEvent(t *testing.T, db *sql.DB, event taskStatEventSeed) {
	t.Helper()
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		parsed, err := time.ParseInLocation(statisticsDateLayout, event.StatDate, time.Local)
		if err != nil {
			t.Fatalf("parse stat event date failed: %v", err)
		}
		occurredAt = parsed
	}
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO task_stat_events (task_id, occurred_at, stat_date, model_name, user_key, status, latency_seconds)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.TaskID, occurredAt, event.StatDate, event.ModelName, event.UserKey, event.Status, event.LatencySeconds,
	)
	if err != nil {
		t.Fatalf("seed stat event failed: %v", err)
	}
}

func countTaskStatEventsByDate(t *testing.T, db *sql.DB, statDate time.Time) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT count(*) FROM task_stat_events WHERE stat_date = $1`,
		statDate.Format(statisticsDateLayout),
	).Scan(&count); err != nil {
		t.Fatalf("count stat events failed: %v", err)
	}
	return count
}
