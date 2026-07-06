package taskstats

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

type Store interface {
	CountByStatus(ctx context.Context, status string) (int64, error)
	CountAllTasks(ctx context.Context) (int64, error)
	ListBackfilledDatesByRange(ctx context.Context, startDate, endDate time.Time) ([]time.Time, error)
	ListDailyStatsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]taskstore.DailyStatAggregate, error)
	ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]taskstore.ExecutionBottleneckAggregate, error)
	ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]taskstore.ExecutionFailureCategoryAggregate, error)
	ListStatEventLatencySamplesByDateRange(ctx context.Context, startDate, endDate time.Time) ([]taskstore.StatEventLatencySample, error)
	ListStatisticsByCreatedAtRange(ctx context.Context, start, end time.Time) ([]taskstore.TaskStatisticLite, error)
	ListMonitorExecutionPressureByRange(ctx context.Context, start, now time.Time) ([]taskstore.MonitorExecutionPressureAggregate, error)
	BackfillDailyStatsByCreatedRange(ctx context.Context, startedAt, endedAt, statDate time.Time, markDone bool) (taskstore.BackfillDailyStatsResult, error)
}

type Service struct {
	store Store
}

type statisticsTrendBucket struct {
	date    string
	label   string
	total   int64
	success int64
	fail    int64
	models  map[string]int64
}

// MonitorExecutionPressurePoint is an admin-facing chart DTO. Runtime queue
// packages should stay unaware of these presentation metrics.
type MonitorExecutionPressurePoint struct {
	Key                string  `json:"key"`
	Label              string  `json:"label"`
	Count              int64   `json:"count"`
	SharePercent       float64 `json:"share_percent"`
	DurationPressure   float64 `json:"duration_pressure"`
	AnomalyPressure    float64 `json:"anomaly_pressure"`
	AvgDurationSeconds float64 `json:"avg_duration_seconds"`
	MaxDurationSeconds float64 `json:"max_duration_seconds"`
	FailedCount        int64   `json:"failed_count"`
	RetryCount         int64   `json:"retry_count"`
}

const statisticsDateLayout = "2006-01-02"
const monitorExecutionPressureWindow = 6 * time.Hour

func NewService(store Store) *Service {
	return &Service{store: store}
}

func normalizeStatisticsRange(raw string) (StatisticsRange, int, error) {
	switch normalized := StatisticsRange(strings.TrimSpace(strings.ToLower(raw))); normalized {
	case StatisticsRange1D:
		return StatisticsRange1D, 1, nil
	case "":
		return StatisticsRange7D, 7, nil
	case StatisticsRange7D:
		return StatisticsRange7D, 7, nil
	case StatisticsRange30D:
		return StatisticsRange30D, 30, nil
	case StatisticsRange1Y:
		return StatisticsRange1Y, 365, nil
	default:
		return "", 0, validation.New("range 仅支持 1d、7d、30d 或 1y")
	}
}

func (s *Service) GetTaskStats(ctx context.Context) (map[string]int64, error) {
	statuses := []string{
		tasks.StatusPending,
		tasks.StatusRunning,
		tasks.StatusCompleted,
		tasks.StatusFailed,
		tasks.StatusCancelled,
	}
	stats := make(map[string]int64, len(statuses))
	for _, status := range statuses {
		count, err := s.store.CountByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("count tasks by status %s: %w", status, err)
		}
		stats[status] = count
	}
	return stats, nil
}

type executionPressureBucket struct {
	key          string
	label        string
	basePressure float64
}

var monitorExecutionPressureBuckets = []executionPressureBucket{
	{key: "fast_completed", label: "快速完成", basePressure: 8},
	{key: "slow_completed", label: "慢速完成", basePressure: 28},
	{key: "normal_pending", label: "正常排队", basePressure: 22},
	{key: "backlog_pending", label: "积压排队", basePressure: 58},
	{key: "normal_running", label: "正常执行", basePressure: 32},
	{key: "stuck_running", label: "卡住执行", basePressure: 82},
	{key: "fast_failed", label: "快速失败", basePressure: 78},
	{key: "slow_failed", label: "慢速失败", basePressure: 94},
	{key: "cancelled", label: "已取消", basePressure: 34},
	{key: "retrying", label: "重试中", basePressure: 68},
	{key: "other", label: "其他状态", basePressure: 40},
}

func (s *Service) GetMonitorExecutionPressure(ctx context.Context) ([]MonitorExecutionPressurePoint, error) {
	now := time.Now()
	rows, err := s.store.ListMonitorExecutionPressureByRange(ctx, now.Add(-monitorExecutionPressureWindow), now)
	if err != nil {
		return nil, err
	}
	return buildMonitorExecutionPressure(rows), nil
}

func buildMonitorExecutionPressure(rows []taskstore.MonitorExecutionPressureAggregate) []MonitorExecutionPressurePoint {
	byKey := make(map[string]taskstore.MonitorExecutionPressureAggregate, len(rows))
	total := int64(0)
	for _, row := range rows {
		byKey[row.Key] = row
		total += row.Total
	}

	points := make([]MonitorExecutionPressurePoint, 0, len(monitorExecutionPressureBuckets))
	for _, bucket := range monitorExecutionPressureBuckets {
		row := byKey[bucket.key]
		point := MonitorExecutionPressurePoint{
			Key:                bucket.key,
			Label:              bucket.label,
			Count:              row.Total,
			SharePercent:       percentOf(float64(row.Total), float64(total)),
			DurationPressure:   percentOf(row.MaxDurationSeconds, 60),
			AvgDurationSeconds: row.AvgDurationSeconds,
			MaxDurationSeconds: row.MaxDurationSeconds,
			FailedCount:        row.Failed,
			RetryCount:         row.Retry,
		}
		if row.Total > 0 {
			point.AnomalyPressure = clampPercent(bucket.basePressure + math.Min(18, float64(row.Retry)*6))
		}
		points = append(points, point)
	}
	return points
}

func percentOf(value float64, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return clampPercent((value / total) * 100)
}

func clampPercent(value float64) float64 {
	return math.Min(100, math.Max(0, value))
}

func (s *Service) GetStatistics(ctx context.Context, rawRange string) (*AdminStatisticsResponse, error) {
	normalizedRange, days, err := normalizeStatisticsRange(rawRange)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))

	totalCalls, err := s.store.CountAllTasks(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := s.GetTaskStats(ctx)
	if err != nil {
		return nil, err
	}

	rangeStartDate := statisticsDateKey(rangeStart)
	rangeEndDate := statisticsDateKey(todayStart)
	backfilledDates, err := s.store.ListBackfilledDatesByRange(ctx, rangeStart, todayStart)
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
	allDailyRows, err := s.store.ListDailyStatsByDateRange(ctx, queryStart, todayStart)
	if err != nil {
		return nil, err
	}
	latencySamples, err := s.store.ListStatEventLatencySamplesByDateRange(ctx, rangeStart, todayStart)
	if err != nil {
		return nil, err
	}
	bottleneckRows, err := s.store.ListExecutionBottleneckStatsByCreatedRange(ctx, rangeStart, todayStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	failureCategoryRows, err := s.store.ListExecutionFailureCategoryStatsByCreatedRange(ctx, rangeStart, todayStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}

	rangeDailyRows, yesterdayRows, monthRows := splitDailyRows(
		allDailyRows,
		statisticsDateKey(rangeStart),
		statisticsDateKey(todayStart),
		statisticsDateKey(yesterdayStart),
		statisticsDateKey(monthStart),
	)

	buckets, orderedDays := buildDateBuckets(rangeStart, days)

	todayCalls, todaySuccess, todayFailed, todayCancelled,
		yesterdayCalls, yesterdaySuccess, yesterdayFailed, yesterdayCancelled,
		modelMetrics := buildDailyMetrics(rangeDailyRows, yesterdayRows, buckets, statisticsDateKey(todayStart))

	trend := buildTrendItems(orderedDays, buckets)
	qualityTrend := buildQualityTrendItems(orderedDays, buckets)
	if normalizedRange == StatisticsRange1D {
		trend, qualityTrend, err = s.buildIntradayTrendItems(ctx, todayStart, loc)
		if err != nil {
			return nil, fmt.Errorf("build intraday trend: %w", err)
		}
	}

	latencyByModel := make(map[string][]float64)
	for _, sample := range latencySamples {
		latencyByModel[sample.ModelName] = append(latencyByModel[sample.ModelName], sample.LatencySeconds)
	}

	modelHealth, slowModels := buildModelHealthAndSlowModels(modelMetrics, latencyByModel)
	bottlenecks := buildBottleneckSources(bottleneckRows)
	failures := buildFailureCategories(failureCategoryRows)
	var thisMonthCalls, thisMonthCancelled int64
	for _, row := range monthRows {
		thisMonthCalls += row.Total
		thisMonthCancelled += row.Cancelled
	}

	return &AdminStatisticsResponse{
		Summary: AdminStatisticsSummary{
			TotalCalls:         totalCalls,
			TodayCalls:         todayCalls,
			TodaySuccess:       todaySuccess,
			TodayFailed:        todayFailed,
			TodayCancelled:     todayCancelled,
			YesterdayCalls:     yesterdayCalls,
			YesterdaySuccess:   yesterdaySuccess,
			YesterdayFailed:    yesterdayFailed,
			YesterdayCancelled: yesterdayCancelled,
			ThisMonthCalls:     thisMonthCalls,
			ThisMonthCancelled: thisMonthCancelled,
			Pending:            stats[tasks.StatusPending],
			Running:            stats[tasks.StatusRunning],
			Completed:          stats[tasks.StatusCompleted],
			Failed:             stats[tasks.StatusFailed],
			Cancelled:          stats[tasks.StatusCancelled],
		},
		Trend:        trend,
		ModelHealth:  modelHealth,
		QualityTrend: qualityTrend,
		SlowModels:   slowModels,
		Bottlenecks:  bottlenecks,
		Failures:     failures,
		Backfill: AdminStatisticsBackfillState{
			RangeStart:     rangeStartDate,
			RangeEnd:       rangeEndDate,
			MissingDays:    len(missingDaysForStatus),
			BackfilledDays: len(backfilledDates),
		},
	}, nil
}

// buildDateBuckets creates empty trend buckets for each day in the range.
func buildDateBuckets(rangeStart time.Time, days int) (map[string]*statisticsTrendBucket, []string) {
	buckets := make(map[string]*statisticsTrendBucket, days)
	orderedDays := make([]string, 0, days)
	for i := 0; i < days; i++ {
		d := rangeStart.AddDate(0, 0, i)
		dateKey := statisticsDateKey(d)
		buckets[dateKey] = &statisticsTrendBucket{
			date:   dateKey,
			label:  d.Format("01/02"),
			total:  0,
			models: map[string]int64{},
		}
		orderedDays = append(orderedDays, dateKey)
	}
	return buckets, orderedDays
}

// splitDailyRows partitions a single merged daily-stats query into yesterday, month,
// and range slices so callers can compute per-period aggregates without separate DB calls.
func splitDailyRows(rows []taskstore.DailyStatAggregate, rangeStartDate, rangeEndDate, yesterdayDate, monthStartDate string) (rangeRows, yesterdayRows, monthRows []taskstore.DailyStatAggregate) {
	for _, row := range rows {
		dateKey := statisticsDateKey(row.StatDate)
		if dateKey >= rangeStartDate && dateKey <= rangeEndDate {
			rangeRows = append(rangeRows, row)
		}
		if dateKey == yesterdayDate {
			yesterdayRows = append(yesterdayRows, row)
		}
		if dateKey >= monthStartDate {
			monthRows = append(monthRows, row)
		}
	}
	return
}

// buildDailyMetrics aggregates daily stat rows into per-model accumulators and
// per-day trend buckets, returning today/yesterday call counts.
func buildDailyMetrics(rangeDailyRows, yesterdayRows []taskstore.DailyStatAggregate, buckets map[string]*statisticsTrendBucket, rangeEndDate string) (todayCalls, todaySuccess, todayFailed, todayCancelled int64, yesterdayCalls, yesterdaySuccess, yesterdayFailed, yesterdayCancelled int64, modelMetrics map[string]*modelMetricAcc) {
	modelMetrics = map[string]*modelMetricAcc{}
	for _, row := range rangeDailyRows {
		dateKey := statisticsDateKey(row.StatDate)
		model := row.ModelName
		if bucket, ok := buckets[dateKey]; ok {
			bucket.total += row.Total
			bucket.success += row.Completed
			bucket.fail += row.Failed
			bucket.models[model] += row.Total
		}
		acc := modelMetrics[model]
		if acc == nil {
			acc = &modelMetricAcc{}
			modelMetrics[model] = acc
		}
		acc.total += row.Total
		acc.completed += row.Completed
		acc.failed += row.Failed
		if row.LatencyCount > 0 {
			acc.latencyTotal += row.LatencySecondsSum
			acc.latencyCount += row.LatencyCount
		}
		if dateKey == rangeEndDate {
			todayCalls += row.Total
			todaySuccess += row.Completed
			todayFailed += row.Failed
			todayCancelled += row.Cancelled
		}
	}
	for _, row := range yesterdayRows {
		yesterdayCalls += row.Total
		yesterdaySuccess += row.Completed
		yesterdayFailed += row.Failed
		yesterdayCancelled += row.Cancelled
	}
	return
}

// buildModelHealthAndSlowModels transforms per-model accumulators and latency
// samples into the sorted slices returned to the frontend.
func buildModelHealthAndSlowModels(modelMetrics map[string]*modelMetricAcc, latencyByModel map[string][]float64) (modelHealth []AdminStatisticsModelHealth, slowModels []AdminStatisticsSlowModel) {
	modelHealth = make([]AdminStatisticsModelHealth, 0, len(modelMetrics))
	slowModels = make([]AdminStatisticsSlowModel, 0, len(modelMetrics))
	for name, acc := range modelMetrics {
		avgSeconds := 0.0
		if acc.latencyCount > 0 {
			avgSeconds = acc.latencyTotal / float64(acc.latencyCount)
		}
		samples := latencyByModel[name]
		if acc.latencyCount > 0 || len(samples) > 0 {
			slowModels = append(slowModels, AdminStatisticsSlowModel{
				Name: name, AvgSeconds: avgSeconds,
				P95Seconds: percentile95(samples), Samples: int64(len(samples)),
			})
		}
		if acc.total == 0 {
			continue
		}
		attempted := acc.completed + acc.failed
		failureRate := 0.0
		if attempted > 0 {
			failureRate = (float64(acc.failed) / float64(attempted)) * 100
		}
		modelHealth = append(modelHealth, AdminStatisticsModelHealth{
			Name: name, Total: acc.total, Completed: acc.completed,
			Failed: acc.failed, FailureRate: failureRate, AvgSeconds: avgSeconds,
		})
	}
	sort.Slice(slowModels, func(i, j int) bool {
		if slowModels[i].P95Seconds != slowModels[j].P95Seconds {
			return slowModels[i].P95Seconds > slowModels[j].P95Seconds
		}
		if slowModels[i].AvgSeconds != slowModels[j].AvgSeconds {
			return slowModels[i].AvgSeconds > slowModels[j].AvgSeconds
		}
		if slowModels[i].Samples != slowModels[j].Samples {
			return slowModels[i].Samples > slowModels[j].Samples
		}
		return slowModels[i].Name < slowModels[j].Name
	})
	if len(slowModels) > 6 {
		slowModels = slowModels[:6]
	}
	sort.Slice(modelHealth, func(i, j int) bool {
		if modelHealth[i].FailureRate != modelHealth[j].FailureRate {
			return modelHealth[i].FailureRate > modelHealth[j].FailureRate
		}
		if modelHealth[i].AvgSeconds != modelHealth[j].AvgSeconds {
			return modelHealth[i].AvgSeconds > modelHealth[j].AvgSeconds
		}
		if modelHealth[i].Total != modelHealth[j].Total {
			return modelHealth[i].Total > modelHealth[j].Total
		}
		return modelHealth[i].Name < modelHealth[j].Name
	})
	return
}

type modelMetricAcc struct {
	total        int64
	completed    int64
	failed       int64
	latencyTotal float64
	latencyCount int64
}

func (s *Service) buildIntradayTrendItems(ctx context.Context, dayStart time.Time, loc *time.Location) ([]AdminStatisticsTrendItem, []AdminStatisticsQualityTrendItem, error) {
	const bucketHours = 4

	dayEnd := dayStart.AddDate(0, 0, 1)
	rows, err := s.store.ListStatisticsByCreatedAtRange(ctx, dayStart, dayEnd)
	if err != nil {
		return nil, nil, err
	}

	buckets := make(map[string]*statisticsTrendBucket, 24/bucketHours)
	orderedKeys := make([]string, 0, 24/bucketHours)
	dayKey := statisticsDateKey(dayStart.In(loc))
	for hour := 0; hour < 24; hour += bucketHours {
		key := fmt.Sprintf("%sT%02d", dayKey, hour)
		buckets[key] = &statisticsTrendBucket{
			date:   key,
			label:  fmt.Sprintf("%02d:00", hour),
			models: map[string]int64{},
		}
		orderedKeys = append(orderedKeys, key)
	}

	for _, row := range rows {
		if row.Status != tasks.StatusCompleted && row.Status != tasks.StatusFailed && row.Status != tasks.StatusCancelled {
			continue
		}
		createdAt := row.CreatedAt.In(loc)
		if createdAt.Before(dayStart) || !createdAt.Before(dayEnd) {
			continue
		}
		bucketHour := (createdAt.Hour() / bucketHours) * bucketHours
		key := fmt.Sprintf("%sT%02d", dayKey, bucketHour)
		bucket := buckets[key]
		if bucket == nil {
			continue
		}

		bucket.total++
		switch row.Status {
		case tasks.StatusCompleted:
			bucket.success++
		case tasks.StatusFailed:
			bucket.fail++
		}
		model := statisticLiteModel(row)
		bucket.models[model]++
	}

	return buildTrendItems(orderedKeys, buckets), buildQualityTrendItems(orderedKeys, buckets), nil
}

func statisticLiteModel(row taskstore.TaskStatisticLite) string {
	if model := strings.TrimSpace(row.ResolvedModel); model != "" {
		return model
	}
	if model := strings.TrimSpace(row.RequestedModel); model != "" {
		return model
	}
	return taskstore.ExtractTaskModel(row.Result)
}

func buildTrendItems(orderedKeys []string, buckets map[string]*statisticsTrendBucket) []AdminStatisticsTrendItem {
	trend := make([]AdminStatisticsTrendItem, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		b := buckets[key]
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
	return trend
}

func buildQualityTrendItems(orderedKeys []string, buckets map[string]*statisticsTrendBucket) []AdminStatisticsQualityTrendItem {
	qualityTrend := make([]AdminStatisticsQualityTrendItem, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		b := buckets[key]
		successRate := 0.0
		attempted := b.success + b.fail
		if attempted > 0 {
			successRate = (float64(b.success) / float64(attempted)) * 100
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
	return qualityTrend
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

func buildBottleneckSources(rows []taskstore.ExecutionBottleneckAggregate) []AdminStatisticsBottleneckSource {
	result := make([]AdminStatisticsBottleneckSource, 0, len(rows))
	for _, row := range rows {
		source := strings.TrimSpace(row.Source)
		if source == "" {
			source = tasks.BottleneckSourceUnknown
		}
		result = append(result, AdminStatisticsBottleneckSource{
			Source:       source,
			Label:        bottleneckSourceLabel(source),
			Total:        row.Total,
			Failed:       row.Failed,
			AvgLatencyMS: row.AvgLatencyMS,
		})
	}
	if len(result) > 6 {
		result = result[:6]
	}
	return result
}

func buildFailureCategories(rows []taskstore.ExecutionFailureCategoryAggregate) []AdminStatisticsFailureCategory {
	result := make([]AdminStatisticsFailureCategory, 0, len(rows))
	for _, row := range rows {
		category := strings.TrimSpace(row.Category)
		if category == "" {
			category = tasks.FailureCategoryUnknown
		}
		result = append(result, AdminStatisticsFailureCategory{
			Category:     category,
			Label:        failureCategoryLabel(category),
			Total:        row.Total,
			AvgLatencyMS: row.AvgLatencyMS,
		})
	}
	if len(result) > 6 {
		result = result[:6]
	}
	return result
}

func bottleneckSourceLabel(source string) string {
	switch source {
	case tasks.BottleneckSourceQueue:
		return "队列等待"
	case tasks.BottleneckSourceWorker:
		return "Worker 调度"
	case tasks.BottleneckSourceProvider:
		return "上游模型"
	case tasks.BottleneckSourceStorage:
		return "存储链路"
	case tasks.BottleneckSourceFinalize:
		return "结果落库"
	case tasks.BottleneckSourceInput:
		return "输入校验"
	default:
		return "未知来源"
	}
}

func failureCategoryLabel(category string) string {
	switch category {
	case tasks.FailureCategoryInputInvalid:
		return "输入无效"
	case tasks.FailureCategoryAuth:
		return "认证失败"
	case tasks.FailureCategoryQuota:
		return "额度限制"
	case tasks.FailureCategoryTimeout:
		return "执行超时"
	case tasks.FailureCategoryProviderError:
		return "上游错误"
	case tasks.FailureCategoryStorageError:
		return "存储错误"
	case tasks.FailureCategoryFinalizeError:
		return "结果落库"
	case tasks.FailureCategoryCancelled:
		return "任务取消"
	default:
		return "未知错误"
	}
}

func (s *Service) BackfillStatistics(ctx context.Context, rawRange string) (*AdminStatisticsBackfillResult, error) {
	normalizedRange, days, err := normalizeStatisticsRange(rawRange)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))
	rangeStartDate := statisticsDateKey(rangeStart)
	rangeEndDate := statisticsDateKey(todayStart)

	backfilledDates, err := s.store.ListBackfilledDatesByRange(ctx, rangeStart, todayStart)
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

func buildMissingStatisticsDaysByMarker(rangeStart, todayStart time.Time, days int, markerDates []time.Time) []time.Time {
	return buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart, days, markerDates, true)
}

func buildMissingStatisticsDaysByMarkerForStatus(rangeStart, todayStart time.Time, days int, markerDates []time.Time) []time.Time {
	return buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart, days, markerDates, false)
}

func buildMissingStatisticsDaysByMarkerCore(rangeStart, todayStart time.Time, days int, markerDates []time.Time, includeToday bool) []time.Time {
	markers := make(map[string]struct{}, len(markerDates))
	for _, date := range markerDates {
		markers[statisticsDateKey(date)] = struct{}{}
	}
	missing := make([]time.Time, 0)
	for i := 0; i < days; i++ {
		dayStart := rangeStart.AddDate(0, 0, i)
		dayKey := statisticsDateKey(dayStart)
		if dayKey == statisticsDateKey(todayStart) {
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

func (s *Service) backfillDailyStatsByMissingDays(ctx context.Context, days []time.Time) (int, error) {
	if len(days) == 0 {
		return 0, nil
	}
	loc := time.Now().Location()
	todayDate := statisticsDateKey(time.Now().In(loc))
	backfilledCount := 0
	for _, dayStart := range days {
		dayEnd := dayStart.AddDate(0, 0, 1)
		dayDate := statisticsDateKey(dayStart.In(loc))
		markDone := dayDate != todayDate
		if _, err := s.store.BackfillDailyStatsByCreatedRange(ctx, dayStart, dayEnd, dayStart, markDone); err != nil {
			return backfilledCount, err
		}
		if markDone {
			backfilledCount++
		}
	}
	return backfilledCount, nil
}

func statisticsDateKey(value time.Time) string {
	return value.Format(statisticsDateLayout)
}
