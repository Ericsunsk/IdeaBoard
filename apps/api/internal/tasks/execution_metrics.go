package tasks

import "time"

const (
	ExecutionStageQueued     = "queued"
	ExecutionStageAssigned   = "assigned"
	ExecutionStageProvider   = "provider"
	ExecutionStageFinalizing = "finalizing"
	ExecutionStageCompleted  = "completed"
	ExecutionStageFailed     = "failed"
	ExecutionStageCancelled  = "cancelled"
)

const (
	BottleneckSourceQueue    = "queue"
	BottleneckSourceWorker   = "worker"
	BottleneckSourceProvider = "provider"
	BottleneckSourceStorage  = "storage"
	BottleneckSourceFinalize = "finalize"
	BottleneckSourceInput    = "input"
	BottleneckSourceUnknown  = "unknown"
)

const (
	FailureCategoryInputInvalid  = "input_invalid"
	FailureCategoryAuth          = "auth"
	FailureCategoryQuota         = "quota"
	FailureCategoryTimeout       = "timeout"
	FailureCategoryProviderError = "provider_error"
	FailureCategoryStorageError  = "storage_error"
	FailureCategoryFinalizeError = "finalize_error"
	FailureCategoryCancelled     = "cancelled"
	FailureCategoryUnknown       = "unknown"
)

const (
	TaskAttemptStatusRunning   = "running"
	TaskAttemptStatusCompleted = "completed"
	TaskAttemptStatusFailed    = "failed"
	TaskAttemptStatusRequeued  = "requeued"
	TaskAttemptStatusCancelled = "cancelled"
)

type TaskExecutionSnapshot struct {
	TaskID         string
	FeatureSlug    string
	RequestedModel string
	ResolvedModel  string
	APIType        string
}

type TaskAttemptStart struct {
	TaskID            string
	RiverJobID        int64
	LeaseToken        string
	AccountID         int64
	ChannelID         int64
	FeatureSlug       string
	RequestedModel    string
	ResolvedModel     string
	APIType           string
	ProviderStartedAt time.Time
}

type TaskAttemptRef struct {
	ID        int64
	AttemptNo int
}

type TaskAttemptFinish struct {
	ID                 int64
	Status             string
	FailureStage       string
	FailureCategory    string
	ErrorMsg           string
	ProviderFinishedAt *time.Time
	FinalizeStartedAt  *time.Time
	FinalizeFinishedAt *time.Time
	ProviderLatencyMS  int64
	FinalizeLatencyMS  int64
	TotalLatencyMS     int64
	CompletedAt        time.Time
}

type TaskCancellationSnapshot struct {
	TaskID      string
	CancelledAt time.Time
	CancelledBy *int64
	Reason      string
}

type TaskExecutionMetrics struct {
	TaskID             string     `json:"task_id"`
	CurrentStage       string     `json:"current_stage"`
	BottleneckSource   string     `json:"bottleneck_source"`
	FailureStage       string     `json:"failure_stage"`
	FailureCategory    string     `json:"failure_category"`
	AssignedAt         *time.Time `json:"assigned_at"`
	ProviderStartedAt  *time.Time `json:"provider_started_at"`
	ProviderFinishedAt *time.Time `json:"provider_finished_at"`
	FinalizeStartedAt  *time.Time `json:"finalize_started_at"`
	FinalizeFinishedAt *time.Time `json:"finalize_finished_at"`
	QueueWaitMS        int64      `json:"queue_wait_ms"`
	WorkerOverheadMS   int64      `json:"worker_overhead_ms"`
	ProviderLatencyMS  int64      `json:"provider_latency_ms"`
	FinalizeLatencyMS  int64      `json:"finalize_latency_ms"`
	TotalLatencyMS     int64      `json:"total_latency_ms"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type TaskExecutionMetricsPatch struct {
	TaskID             string
	CurrentStage       *string
	BottleneckSource   *string
	FailureStage       *string
	FailureCategory    *string
	AssignedAt         *time.Time
	ProviderStartedAt  *time.Time
	ProviderFinishedAt *time.Time
	FinalizeStartedAt  *time.Time
	FinalizeFinishedAt *time.Time
	QueueWaitMS        *int64
	WorkerOverheadMS   *int64
	ProviderLatencyMS  *int64
	FinalizeLatencyMS  *int64
	TotalLatencyMS     *int64
}

func ResolveExecutionBottleneck(metrics *TaskExecutionMetrics, failureStage string, failureCategory string) string {
	switch failureCategory {
	case FailureCategoryStorageError:
		return BottleneckSourceStorage
	case FailureCategoryFinalizeError:
		return BottleneckSourceFinalize
	case FailureCategoryInputInvalid:
		if failureStage == ExecutionStageAssigned || failureStage == ExecutionStageQueued {
			return BottleneckSourceInput
		}
	case FailureCategoryAuth, FailureCategoryQuota, FailureCategoryTimeout, FailureCategoryProviderError:
		return BottleneckSourceProvider
	case FailureCategoryCancelled:
		return BottleneckSourceUnknown
	}
	if metrics == nil {
		return BottleneckSourceUnknown
	}
	type stageLatency struct {
		source string
		value  int64
	}
	latencies := []stageLatency{
		{source: BottleneckSourceQueue, value: metrics.QueueWaitMS},
		{source: BottleneckSourceWorker, value: metrics.WorkerOverheadMS},
		{source: BottleneckSourceProvider, value: metrics.ProviderLatencyMS},
		{source: BottleneckSourceFinalize, value: metrics.FinalizeLatencyMS},
	}
	best := stageLatency{source: BottleneckSourceUnknown}
	for _, latency := range latencies {
		if latency.value > best.value {
			best = latency
		}
	}
	if best.value <= 0 {
		return BottleneckSourceUnknown
	}
	return best.source
}
