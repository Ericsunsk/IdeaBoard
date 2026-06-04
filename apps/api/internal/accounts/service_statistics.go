package accounts

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ideaboard/api/internal/tasks"
)

func normalizeStatisticsRange(raw string) (StatisticsRange, int) {
	switch StatisticsRange(strings.TrimSpace(strings.ToLower(raw))) {
	case StatisticsRange30D:
		return StatisticsRange30D, 30
	case StatisticsRange1Y:
		return StatisticsRange1Y, 365
	default:
		return StatisticsRange7D, 7
	}
}

func (s *AccountService) GetStatistics(ctx context.Context, rawRange string) (*AdminStatisticsResponse, error) {
	_, days := normalizeStatisticsRange(rawRange)
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))

	totalCalls, err := s.taskRepo.CountAllTasks(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := s.GetTaskStats(ctx)
	if err != nil {
		return nil, err
	}

	rangeStartDate := rangeStart.Format("2006-01-02")
	rangeEndDate := todayStart.Format("2006-01-02")
	backfilledDates, err := s.taskRepo.ListBackfilledDatesByRange(ctx, rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, err
	}
	missingDaysForStatus := buildMissingStatisticsDaysByMarkerForStatus(rangeStart, todayStart, days, backfilledDates)

	// Single query covering the wider of month-start vs range-start through today,
	// then split in memory to avoid 3 DB round-trips.
	queryStart := monthStart
	if rangeStart.Before(monthStart) {
		queryStart = rangeStart
	}
	allDailyRows, err := s.taskRepo.ListDailyStatsByDateRange(ctx, queryStart.Format("2006-01-02"), rangeEndDate)
	if err != nil {
		return nil, err
	}
	latencySamples, err := s.taskRepo.ListStatEventLatencySamplesByDateRange(ctx, rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, err
	}

	yesterdayDate := yesterdayStart.Format("2006-01-02")
	monthStartDate := monthStart.Format("2006-01-02")
	var rangeDailyRows, yesterdayRows, monthRows []DailyStatAggregate
	for _, row := range allDailyRows {
		if row.StatDate >= rangeStartDate && row.StatDate <= rangeEndDate {
			rangeDailyRows = append(rangeDailyRows, row)
		}
		if row.StatDate == yesterdayDate {
			yesterdayRows = append(yesterdayRows, row)
		}
		if row.StatDate >= monthStartDate {
			monthRows = append(monthRows, row)
		}
	}

	type trendBucket struct {
		date    string
		label   string
		total   int64
		success int64
		fail    int64
		models  map[string]int64
	}
	buckets := make(map[string]*trendBucket, days)
	orderedDays := make([]string, 0, days)
	for i := 0; i < days; i++ {
		d := rangeStart.AddDate(0, 0, i)
		dateKey := d.Format("2006-01-02")
		buckets[dateKey] = &trendBucket{
			date:   dateKey,
			label:  d.Format("01/02"),
			total:  0,
			models: map[string]int64{},
		}
		orderedDays = append(orderedDays, dateKey)
	}

	type userMetricAcc struct {
		total     int64
		completed int64
		failed    int64
	}
	userMetrics := map[string]*userMetricAcc{}
	type modelMetricAcc struct {
		total        int64
		completed    int64
		failed       int64
		latencyTotal float64
		latencyCount int64
	}
	modelMetrics := map[string]*modelMetricAcc{}

	var todayCalls, todaySuccess, todayFailed int64
	for _, row := range rangeDailyRows {
		dateKey := row.StatDate
		model := row.ModelName
		user := row.UserKey
		if bucket, ok := buckets[dateKey]; ok {
			bucket.total += row.Total
			bucket.success += row.Completed
			bucket.fail += row.Failed
			bucket.models[model] += row.Total
		}
		modelAcc := modelMetrics[model]
		if modelAcc == nil {
			modelAcc = &modelMetricAcc{}
			modelMetrics[model] = modelAcc
		}
		modelAcc.total += row.Total
		modelAcc.completed += row.Completed
		modelAcc.failed += row.Failed
		userAcc := userMetrics[user]
		if userAcc == nil {
			userAcc = &userMetricAcc{}
			userMetrics[user] = userAcc
		}
		userAcc.total += row.Total
		userAcc.completed += row.Completed
		userAcc.failed += row.Failed

		if row.LatencyCount > 0 {
			modelAcc.latencyTotal += row.LatencySecondsSum
			modelAcc.latencyCount += row.LatencyCount
		}

		if dateKey == todayStart.Format("2006-01-02") {
			todayCalls += row.Total
			todaySuccess += row.Completed
			todayFailed += row.Failed
		}
	}

	var yesterdayCalls, yesterdaySuccess, yesterdayFailed int64
	for _, row := range yesterdayRows {
		yesterdayCalls += row.Total
		yesterdaySuccess += row.Completed
		yesterdayFailed += row.Failed
	}

	trend := make([]AdminStatisticsTrendItem, 0, len(orderedDays))
	for _, dayKey := range orderedDays {
		b := buckets[dayKey]
		segments := make([]AdminStatisticsTrendItemModel, 0, len(b.models))
		for model, value := range b.models {
			segments = append(segments, AdminStatisticsTrendItemModel{Label: model, Value: value})
		}
		sort.Slice(segments, func(i, j int) bool {
			if segments[i].Value == segments[j].Value {
				return segments[i].Label < segments[j].Label
			}
			return segments[i].Value > segments[j].Value
		})
		if len(segments) > 3 {
			segments = segments[:3]
		}
		trend = append(trend, AdminStatisticsTrendItem{
			Date:     b.date,
			Label:    b.label,
			Total:    b.total,
			Segments: segments,
		})
	}

	qualityTrend := make([]AdminStatisticsQualityTrendItem, 0, len(orderedDays))
	for _, dayKey := range orderedDays {
		b := buckets[dayKey]
		successRate := 0.0
		if b.total > 0 {
			successRate = (float64(b.success) / float64(b.total)) * 100
		}
		qualityTrend = append(qualityTrend, AdminStatisticsQualityTrendItem{
			Date:        b.date,
			Label:       b.label,
			Total:       b.total,
			Success:     b.success,
			Fail:        b.fail,
			SuccessRate: successRate,
		})
	}

	userRisk := make([]AdminStatisticsUserRiskItem, 0, len(userMetrics))
	for name, acc := range userMetrics {
		failureRate := 0.0
		if acc.total > 0 {
			failureRate = (float64(acc.failed) / float64(acc.total)) * 100
		}
		userRisk = append(userRisk, AdminStatisticsUserRiskItem{
			Name:        name,
			Total:       acc.total,
			Completed:   acc.completed,
			Failed:      acc.failed,
			FailureRate: failureRate,
		})
	}
	sort.Slice(userRisk, func(i, j int) bool {
		if userRisk[i].Failed == userRisk[j].Failed {
			if userRisk[i].FailureRate == userRisk[j].FailureRate {
				if userRisk[i].Total == userRisk[j].Total {
					return userRisk[i].Name < userRisk[j].Name
				}
				return userRisk[i].Total > userRisk[j].Total
			}
			return userRisk[i].FailureRate > userRisk[j].FailureRate
		}
		return userRisk[i].Failed > userRisk[j].Failed
	})
	if len(userRisk) > 6 {
		userRisk = userRisk[:6]
	}

	latencyByModel := make(map[string][]float64)
	for _, sample := range latencySamples {
		latencyByModel[sample.ModelName] = append(latencyByModel[sample.ModelName], sample.LatencySeconds)
	}

	slowModels := make([]AdminStatisticsSlowModel, 0, len(modelMetrics))
	modelHealth := make([]AdminStatisticsModelHealth, 0, len(modelMetrics))
	for name, acc := range modelMetrics {
		avgSeconds := 0.0
		if acc.latencyCount > 0 {
			avgSeconds = acc.latencyTotal / float64(acc.latencyCount)
		}
		samples := latencyByModel[name]
		if acc.latencyCount > 0 || len(samples) > 0 {
			slowModels = append(slowModels, AdminStatisticsSlowModel{
				Name:       name,
				AvgSeconds: avgSeconds,
				P95Seconds: percentile95(samples),
				Samples:    int64(len(samples)),
			})
		}
		if acc.total == 0 {
			continue
		}
		modelHealth = append(modelHealth, AdminStatisticsModelHealth{
			Name:        name,
			Total:       acc.total,
			Completed:   acc.completed,
			Failed:      acc.failed,
			FailureRate: (float64(acc.failed) / float64(acc.total)) * 100,
			AvgSeconds:  avgSeconds,
		})
	}
	sort.Slice(slowModels, func(i, j int) bool {
		if slowModels[i].P95Seconds == slowModels[j].P95Seconds {
			if slowModels[i].AvgSeconds == slowModels[j].AvgSeconds {
				if slowModels[i].Samples == slowModels[j].Samples {
					return slowModels[i].Name < slowModels[j].Name
				}
				return slowModels[i].Samples > slowModels[j].Samples
			}
			return slowModels[i].AvgSeconds > slowModels[j].AvgSeconds
		}
		return slowModels[i].P95Seconds > slowModels[j].P95Seconds
	})
	if len(slowModels) > 6 {
		slowModels = slowModels[:6]
	}
	sort.Slice(modelHealth, func(i, j int) bool {
		if modelHealth[i].FailureRate == modelHealth[j].FailureRate {
			if modelHealth[i].AvgSeconds == modelHealth[j].AvgSeconds {
				if modelHealth[i].Total == modelHealth[j].Total {
					return modelHealth[i].Name < modelHealth[j].Name
				}
				return modelHealth[i].Total > modelHealth[j].Total
			}
			return modelHealth[i].AvgSeconds > modelHealth[j].AvgSeconds
		}
		return modelHealth[i].FailureRate > modelHealth[j].FailureRate
	})

	return &AdminStatisticsResponse{
		Summary: AdminStatisticsSummary{
			TotalCalls:       totalCalls,
			TodayCalls:       todayCalls,
			TodaySuccess:     todaySuccess,
			TodayFailed:      todayFailed,
			YesterdayCalls:   yesterdayCalls,
			YesterdaySuccess: yesterdaySuccess,
			YesterdayFailed:  yesterdayFailed,
			ThisMonthCalls:   sumDailyTotal(monthRows),
			Pending:          stats[tasks.StatusPending],
			Running:          stats[tasks.StatusRunning],
			Completed:        stats[tasks.StatusCompleted],
			Failed:           stats[tasks.StatusFailed],
		},
		Trend:        trend,
		ModelHealth:  modelHealth,
		QualityTrend: qualityTrend,
		SlowModels:   slowModels,
		UserRisk:     userRisk,
		Backfill: AdminStatisticsBackfillState{
			RangeStart:     rangeStartDate,
			RangeEnd:       rangeEndDate,
			MissingDays:    len(missingDaysForStatus),
			BackfilledDays: len(backfilledDates),
		},
	}, nil
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(math.Ceil(float64(len(values))*0.95)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func (s *AccountService) BackfillStatistics(ctx context.Context, rawRange string) (*AdminStatisticsBackfillResult, error) {
	normalizedRange, days := normalizeStatisticsRange(rawRange)
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))
	rangeStartDate := rangeStart.Format("2006-01-02")
	rangeEndDate := todayStart.Format("2006-01-02")

	backfilledDates, err := s.taskRepo.ListBackfilledDatesByRange(ctx, rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, fmt.Errorf("list backfilled dates: %w", err)
	}
	missingDays := buildMissingStatisticsDaysByMarker(rangeStart, todayStart, days, backfilledDates)
	backfilledCount, err := s.backfillDailyStatsByMissingDays(ctx, missingDays)
	if err != nil {
		return nil, fmt.Errorf("backfill daily stats: %w", err)
	}

	return &AdminStatisticsBackfillResult{
		Range:          normalizedRange,
		RangeStart:     rangeStartDate,
		RangeEnd:       rangeEndDate,
		ScannedDays:    len(missingDays),
		BackfilledDays: backfilledCount,
	}, nil
}

func sumDailyTotal(rows []DailyStatAggregate) int64 {
	var total int64
	for _, row := range rows {
		total += row.Total
	}
	return total
}

func buildMissingStatisticsDaysByMarker(rangeStart, todayStart time.Time, days int, markerDates []string) []time.Time {
	return buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart, days, markerDates, true)
}

func buildMissingStatisticsDaysByMarkerForStatus(rangeStart, todayStart time.Time, days int, markerDates []string) []time.Time {
	return buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart, days, markerDates, false)
}

func buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart time.Time, days int, markerDates []string, includeToday bool) []time.Time {
	markers := make(map[string]struct{}, len(markerDates))
	for _, date := range markerDates {
		markers[date] = struct{}{}
	}
	missing := make([]time.Time, 0)
	for i := 0; i < days; i++ {
		dayStart := rangeStart.AddDate(0, 0, i)
		dayKey := dayStart.Format("2006-01-02")
		if dayKey == todayStart.Format("2006-01-02") {
			if includeToday {
				// 今天始终允许回填，覆盖部署前同一天历史终态任务；事件幂等保证不会重复计数。
				missing = append(missing, dayStart)
			}
			continue
		}
		if _, ok := markers[dayKey]; ok {
			continue
		}
		missing = append(missing, dayStart)
	}
	return missing
}

func (s *AccountService) backfillDailyStatsByMissingDays(ctx context.Context, days []time.Time) (int, error) {
	if len(days) == 0 {
		return 0, nil
	}
	loc := time.Now().Location()
	todayDate := time.Now().In(loc).Format("2006-01-02")
	backfilledCount := 0
	for _, dayStart := range days {
		dayEnd := dayStart.AddDate(0, 0, 1)
		rows, err := s.taskRepo.ListStatisticsByCreatedAtRange(ctx, dayStart, dayEnd)
		if err != nil {
			return backfilledCount, err
		}
		accountIDs := make([]int64, 0, len(rows))
		for _, row := range rows {
			if row.AccountID != 0 {
				accountIDs = append(accountIDs, row.AccountID)
			}
		}
		modelSetMap, err := s.taskRepo.listSingleModelSetByAccountIDs(ctx, accountIDs)
		if err != nil {
			return backfilledCount, err
		}
		for _, row := range rows {
			if row.Status != tasks.StatusCompleted && row.Status != tasks.StatusFailed {
				continue
			}
			task := &tasks.Task{
				TaskID:      row.TaskID,
				UserID:      row.UserID,
				AccountID:   row.AccountID,
				Status:      row.Status,
				Result:      row.Result,
				CreatedAt:   row.CreatedAt.In(loc),
				CompletedAt: row.CompletedAt,
			}
			if err := s.taskRepo.UpsertDailyStatByTaskWithFallbackModel(ctx, task, modelSetMap[row.AccountID]); err != nil {
				return backfilledCount, err
			}
		}
		dayDate := dayStart.In(loc).Format("2006-01-02")
		if dayDate != todayDate {
			if err := s.taskRepo.MarkDailyBackfillDone(ctx, dayDate); err != nil {
				return backfilledCount, err
			}
			backfilledCount++
		}
	}
	return backfilledCount, nil
}
