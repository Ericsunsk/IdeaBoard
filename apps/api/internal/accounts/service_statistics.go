package accounts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"api/internal/tasks"
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
		date   string
		label  string
		total  int64
		models map[string]int64
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

	modelCounter := map[string]int64{}
	userCounter := map[string]int64{}
	type latencyAcc struct {
		total float64
		count int64
	}
	latencyCounter := map[string]*latencyAcc{}
	healthCells := make([]AdminStatisticsHealthCell, days)
	for i := range healthCells {
		healthCells[i] = AdminStatisticsHealthCell{Status: 0, Success: 0, Fail: 0, Total: 0}
	}
	dayIndexByDate := make(map[string]int, len(orderedDays))
	for i, dayKey := range orderedDays {
		dayIndexByDate[dayKey] = i
	}

	var todayCalls, todaySuccess, todayFailed int64
	for _, row := range rangeDailyRows {
		dateKey := row.StatDate
		model := row.ModelName
		user := row.UserKey
		if bucket, ok := buckets[dateKey]; ok {
			bucket.total += row.Total
			bucket.models[model] += row.Total
		}
		modelCounter[model] += row.Total
		userCounter[user] += row.Total

		if row.LatencyCount > 0 {
			acc := latencyCounter[model]
			if acc == nil {
				acc = &latencyAcc{}
				latencyCounter[model] = acc
			}
			acc.total += row.LatencySecondsSum
			acc.count += row.LatencyCount
		}

		if cellIdx, ok := dayIndexByDate[dateKey]; ok && cellIdx >= 0 && cellIdx < len(healthCells) {
			healthCells[cellIdx].Total += row.Total
			healthCells[cellIdx].Success += row.Completed
			healthCells[cellIdx].Fail += row.Failed
		}

		if dateKey == todayStart.Format("2006-01-02") {
			todayCalls += row.Total
			todaySuccess += row.Completed
			todayFailed += row.Failed
		}
	}

	for i := range healthCells {
		cell := &healthCells[i]
		if cell.Total == 0 {
			cell.Status = 0
			continue
		}
		ratio := float64(cell.Success) / float64(cell.Total)
		if ratio < 0.5 {
			cell.Status = 1
		} else if ratio < 0.9 {
			cell.Status = 2
		} else {
			cell.Status = 3
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

	modelRanking := make([]AdminStatisticsModelItem, 0, len(modelCounter))
	var rangeTotalCalls int64
	for _, row := range rangeDailyRows {
		rangeTotalCalls += row.Total
	}
	for name, value := range modelCounter {
		percent := int64(0)
		if rangeTotalCalls > 0 {
			percent = int64((float64(value) / float64(rangeTotalCalls)) * 100)
		}
		modelRanking = append(modelRanking, AdminStatisticsModelItem{Name: name, Value: value, Percent: percent})
	}
	sort.Slice(modelRanking, func(i, j int) bool {
		if modelRanking[i].Value == modelRanking[j].Value {
			return modelRanking[i].Name < modelRanking[j].Name
		}
		return modelRanking[i].Value > modelRanking[j].Value
	})
	if len(modelRanking) > 5 {
		modelRanking = modelRanking[:5]
	}

	userRank := make([]AdminStatisticsUserItem, 0, len(userCounter))
	for name, value := range userCounter {
		userRank = append(userRank, AdminStatisticsUserItem{Name: name, Value: value})
	}
	sort.Slice(userRank, func(i, j int) bool {
		if userRank[i].Value == userRank[j].Value {
			return userRank[i].Name < userRank[j].Name
		}
		return userRank[i].Value > userRank[j].Value
	})
	if len(userRank) > 6 {
		userRank = userRank[:6]
	}

	latency := make([]AdminStatisticsLatencyItem, 0, len(latencyCounter))
	for name, acc := range latencyCounter {
		if acc.count == 0 {
			continue
		}
		latency = append(latency, AdminStatisticsLatencyItem{
			Name:       name,
			AvgSeconds: acc.total / float64(acc.count),
		})
	}
	sort.Slice(latency, func(i, j int) bool {
		if latency[i].AvgSeconds == latency[j].AvgSeconds {
			return latency[i].Name < latency[j].Name
		}
		return latency[i].AvgSeconds < latency[j].AvgSeconds
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
		ModelRanking: modelRanking,
		UserRank:     userRank,
		Latency:      latency,
		HealthCells:  healthCells,
		Backfill: AdminStatisticsBackfillState{
			RangeStart:     rangeStartDate,
			RangeEnd:       rangeEndDate,
			MissingDays:    len(missingDaysForStatus),
			BackfilledDays: len(backfilledDates),
		},
	}, nil
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
		accountIDs := make([]uint64, 0, len(rows))
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
