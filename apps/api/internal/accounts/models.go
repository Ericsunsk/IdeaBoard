package accounts

import "time"

type ApiAccount struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"type:varchar(100);not null" json:"name"`
	ChannelID     uint64    `gorm:"not null;default:0" json:"channel_id"`
	Platform      string    `gorm:"type:varchar(50);not null" json:"platform"`
	ApiType       string    `gorm:"type:varchar(50);not null" json:"api_type"`
	ModelSet      string    `gorm:"type:varchar(255)" json:"model_set"`
	ApiKey        string    `gorm:"type:text;not null" json:"-"`
	ProxyURL      string    `gorm:"type:text" json:"proxy_url"`
	MaxConcurrent int       `gorm:"type:int;default:3" json:"max_concurrent"`
	Timeout       int       `gorm:"type:int;default:120" json:"timeout"`
	ExtraConfig   string    `gorm:"type:text" json:"extra_config"`
	Status        int8      `gorm:"type:tinyint;default:1" json:"status"`
	FailCount     int       `gorm:"type:int;default:0" json:"fail_count"`
	Weight        int       `gorm:"type:int;default:1" json:"weight"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// 请求结构
type CreateAccountRequest struct {
	Name          string `json:"name"`
	ChannelID     uint64 `json:"channel_id" binding:"required"`
	Platform      string `json:"platform"`
	ApiType       string `json:"api_type"`
	ModelSet      string `json:"model_set"`
	ApiKey        string `json:"api_key" binding:"required"`
	ProxyURL      string `json:"proxy_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	Timeout       int    `json:"timeout"`
	ExtraConfig   string `json:"extra_config"`
	Weight        int    `json:"weight"`
}

type UpdateAccountRequest struct {
	Name          string `json:"name"`
	ChannelID     uint64 `json:"channel_id"`
	Platform      string `json:"platform"`
	ApiType       string `json:"api_type"`
	ModelSet      string `json:"model_set"`
	ApiKey        string `json:"api_key"`
	ProxyURL      string `json:"proxy_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	Timeout       int    `json:"timeout"`
	ExtraConfig   string `json:"extra_config"`
	Weight        int    `json:"weight"`
	Status        int8   `json:"status"`
}

// 列表响应
type AccountListRequest struct {
	Keyword  string `form:"keyword"`
	Platform string `form:"platform"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

type AccountWithLoad struct {
	ApiAccount
	CurrentConcurrent int    `json:"current_concurrent"`
	WaitQueueCount    int    `json:"wait_queue_count"`
	RunningStatus     string `json:"running_status"`
}

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

// TaskDailyStat 按天+模型+用户的任务终态聚合快照（仅终态任务计入）。
type TaskDailyStat struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	StatDate          string    `gorm:"type:char(10);not null;uniqueIndex:uniq_task_daily_stat" json:"stat_date"`
	ModelName         string    `gorm:"type:varchar(128);not null;default:'';uniqueIndex:uniq_task_daily_stat" json:"model_name"`
	UserKey           string    `gorm:"type:varchar(128);not null;default:'';uniqueIndex:uniq_task_daily_stat" json:"user_key"`
	Total             int64     `gorm:"type:bigint;not null;default:0" json:"total"`
	Completed         int64     `gorm:"type:bigint;not null;default:0" json:"completed"`
	Failed            int64     `gorm:"type:bigint;not null;default:0" json:"failed"`
	LatencySecondsSum float64   `gorm:"type:double;not null;default:0" json:"latency_seconds_sum"`
	LatencyCount      int64     `gorm:"type:bigint;not null;default:0" json:"latency_count"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TaskStatEvent 终态任务统计事件，按 task_id 幂等去重。
type TaskStatEvent struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID         string    `gorm:"type:varchar(64);not null;uniqueIndex:uniq_task_stat_event_task_id" json:"task_id"`
	StatDate       string    `gorm:"type:char(10);not null;index" json:"stat_date"`
	ModelName      string    `gorm:"type:varchar(128);not null;default:''" json:"model_name"`
	UserKey        string    `gorm:"type:varchar(128);not null;default:''" json:"user_key"`
	Status         string    `gorm:"type:varchar(20);not null" json:"status"`
	LatencySeconds float64   `gorm:"type:double;not null;default:0" json:"latency_seconds"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TaskDailyStatBackfill 标记某日期已完成历史终态任务回填扫描。
type TaskDailyStatBackfill struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	StatDate  string    `gorm:"type:char(10);not null;uniqueIndex:uniq_task_daily_stat_backfill_date" json:"stat_date"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
