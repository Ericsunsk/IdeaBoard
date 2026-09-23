package domain

// TaskPayload is the durable drawing task payload carried by River.
// Retry bounding always reads the fresh tasks.retry_count row: a queued
// payload copy would go stale across redeliveries, so the count lives here
// no longer.
type TaskPayload struct {
	TaskID        string `json:"task_id"`
	CorrelationID string `json:"correlation_id,omitempty"`
	UserID        int64  `json:"user_id"`
	Prompt        string `json:"prompt"`
	Params        string `json:"params"`
	Priority      int    `json:"priority"`
	CreatedAt     int64  `json:"created_at"`
	// RiverJobID is injected by the worker at runtime and is not persisted in job args.
	RiverJobID int64 `json:"-"`
}

func (p *TaskPayload) EffectiveCorrelationID() string {
	if p == nil {
		return ""
	}
	if correlationID := NormalizeCorrelationID(p.CorrelationID); correlationID != "" {
		return correlationID
	}
	return CorrelationIDForTaskID(p.TaskID)
}

func (p *TaskPayload) EnsureCorrelationID() string {
	correlationID := p.EffectiveCorrelationID()
	if p != nil {
		p.CorrelationID = correlationID
	}
	return correlationID
}

// MonitorInfo is the queue runtime snapshot returned to the admin monitor API.
type MonitorInfo struct {
	GlobalQueueLen  int64 `json:"global_queue_len"`
	WaitQueueLen    int64 `json:"wait_queue_len"`
	RunningAttempts int64 `json:"running_attempts"`
}
