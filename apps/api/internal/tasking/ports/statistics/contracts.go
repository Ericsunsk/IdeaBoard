package statistics

import "time"

type DailyStatAggregate struct {
	StatDate          time.Time
	ModelName         string
	UserKey           string
	Total             int64
	Completed         int64
	Failed            int64
	Cancelled         int64
	LatencySecondsSum float64
	LatencyCount      int64
	CostMicrosSum     int64
}

type BackfillDailyStatsResult struct {
	InsertedEvents int64
	TouchedGroups  int64
	MarkedDays     int64
}

type RebuildDailyStatsResult struct {
	DeletedGroups int64
	RebuiltGroups int64
}

type TaskStatEventCorrection struct {
	TaskID             string
	OccurredAt         time.Time
	UserID             int64
	AccountID          int64
	ProviderEndpointID int64
	CredentialID       int64
	FeatureSlug        string
	APIType            string
	ModelName          string
	UserKey            string
	Status             string
	OutputCount        int64
	LatencySeconds     float64
	CostMicros         int64
}

type StatEventTrendRow struct {
	OccurredAt     time.Time
	StatDate       time.Time
	ModelName      string
	UserKey        string
	Status         string
	OutputCount    int64
	LatencySeconds float64
	CostMicros     int64
}

type StatEventLatencySample struct {
	ModelName      string
	LatencySeconds float64
}

type ExecutionBottleneckAggregate struct {
	Source       string
	Total        int64
	Failed       int64
	AvgLatencyMS float64
}

type ExecutionFailureCategoryAggregate struct {
	Category     string
	Total        int64
	AvgLatencyMS float64
}

type MonitorExecutionPressureAggregate struct {
	Key                string
	Total              int64
	AvgDurationSeconds float64
	MaxDurationSeconds float64
	Failed             int64
	Retry              int64
}

type MonitorGoldenSignalSample struct {
	Status            string
	FailureStage      string
	FailureCategory   string
	QueueWaitMS       int64
	ProviderLatencyMS int64
	FinalizeLatencyMS int64
	ProviderObserved  bool
	ProviderError     bool
	ProviderAttempts  int64
	ProviderErrors    int64
}
