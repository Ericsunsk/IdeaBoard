package queuetypes

// TaskPayload is the durable drawing task payload carried by River.
type TaskPayload struct {
	TaskID     string  `json:"task_id"`
	UserID     int64   `json:"user_id"`
	Prompt     string  `json:"prompt"`
	Params     string  `json:"params"`
	AccountIDs []int64 `json:"account_ids"`
	RetryCount int     `json:"retry_count"`
	CreatedAt  int64   `json:"created_at"`
	// RiverJobID is injected by the worker at runtime and is not persisted in job args.
	RiverJobID int64 `json:"-"`
}

// AccountLoad describes current account concurrency state.
type AccountLoad struct {
	CurrentConcurrent int `json:"current_concurrent"`
	MaxConcurrent     int `json:"max_concurrent"`
}

// MonitorInfo is the queue/account runtime snapshot returned to the admin monitor API.
// RunningAttempts and AccountLoads are derived from running task attempts, so the
// monitor reflects active execution facts instead of task status alone.
type MonitorInfo struct {
	GlobalQueueLen  int64                  `json:"global_queue_len"`
	WaitQueueLen    int64                  `json:"wait_queue_len"`
	RunningAttempts int64                  `json:"running_attempts"`
	AccountLoads    map[int64]*AccountLoad `json:"account_loads"`
}
