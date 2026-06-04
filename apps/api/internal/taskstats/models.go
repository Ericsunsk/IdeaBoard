package taskstats

type StatisticsRange string

const (
	StatisticsRange7D  StatisticsRange = "7d"
	StatisticsRange30D StatisticsRange = "30d"
	StatisticsRange1Y  StatisticsRange = "1y"
)

type AdminStatisticsResponse struct {
	Summary      AdminStatisticsSummary            `json:"summary"`
	Trend        []AdminStatisticsTrendItem        `json:"trend"`
	ModelHealth  []AdminStatisticsModelHealth      `json:"model_health"`
	QualityTrend []AdminStatisticsQualityTrendItem `json:"quality_trend"`
	SlowModels   []AdminStatisticsSlowModel        `json:"slow_models"`
	UserRisk     []AdminStatisticsUserRiskItem     `json:"user_risk"`
	Backfill     AdminStatisticsBackfillState      `json:"backfill_status,omitempty"`
}

type AdminStatisticsBackfillState struct {
	RangeStart     string `json:"range_start"`
	RangeEnd       string `json:"range_end"`
	MissingDays    int    `json:"missing_days"`
	BackfilledDays int    `json:"backfilled_days"`
}

type AdminStatisticsBackfillResult struct {
	Range          StatisticsRange `json:"range"`
	RangeStart     string          `json:"range_start"`
	RangeEnd       string          `json:"range_end"`
	ScannedDays    int             `json:"scanned_days"`
	BackfilledDays int             `json:"backfilled_days"`
}

type AdminStatisticsSummary struct {
	TotalCalls       int64 `json:"total_calls"`
	TodayCalls       int64 `json:"today_calls"`
	TodaySuccess     int64 `json:"today_success"`
	TodayFailed      int64 `json:"today_failed"`
	YesterdayCalls   int64 `json:"yesterday_calls"`
	YesterdaySuccess int64 `json:"yesterday_success"`
	YesterdayFailed  int64 `json:"yesterday_failed"`
	ThisMonthCalls   int64 `json:"this_month_calls"`
	Pending          int64 `json:"pending"`
	Running          int64 `json:"running"`
	Completed        int64 `json:"completed"`
	Failed           int64 `json:"failed"`
}

type AdminStatisticsTrendItem struct {
	Date     string                          `json:"date"`
	Label    string                          `json:"label"`
	Total    int64                           `json:"total"`
	Segments []AdminStatisticsTrendItemModel `json:"segments"`
}

type AdminStatisticsTrendItemModel struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type AdminStatisticsModelHealth struct {
	Name        string  `json:"name"`
	Total       int64   `json:"total"`
	Completed   int64   `json:"completed"`
	Failed      int64   `json:"failed"`
	FailureRate float64 `json:"failure_rate"`
	AvgSeconds  float64 `json:"avg_seconds"`
}

type AdminStatisticsQualityTrendItem struct {
	Date        string  `json:"date"`
	Label       string  `json:"label"`
	Total       int64   `json:"total"`
	Success     int64   `json:"success"`
	Fail        int64   `json:"fail"`
	SuccessRate float64 `json:"success_rate"`
}

type AdminStatisticsSlowModel struct {
	Name       string  `json:"name"`
	AvgSeconds float64 `json:"avg_seconds"`
	P95Seconds float64 `json:"p95_seconds"`
	Samples    int64   `json:"samples"`
}

type AdminStatisticsUserRiskItem struct {
	Name        string  `json:"name"`
	Total       int64   `json:"total"`
	Completed   int64   `json:"completed"`
	Failed      int64   `json:"failed"`
	FailureRate float64 `json:"failure_rate"`
}
