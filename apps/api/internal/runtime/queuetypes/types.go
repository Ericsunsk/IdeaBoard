package queuetypes

// TaskPayload is the durable drawing task payload carried by Asynq.
type TaskPayload struct {
	TaskID     string   `json:"task_id"`
	UserID     uint64   `json:"user_id"`
	Prompt     string   `json:"prompt"`
	Params     string   `json:"params"`
	AccountIDs []uint64 `json:"account_ids"`
	RetryCount int      `json:"retry_count"`
	CreatedAt  int64    `json:"created_at"`
}

// AccountLoad describes current account concurrency state.
type AccountLoad struct {
	CurrentConcurrent int `json:"current_concurrent"`
	MaxConcurrent     int `json:"max_concurrent"`
}

// MonitorInfo is the queue/account runtime snapshot returned to the admin monitor API.
type MonitorInfo struct {
	GlobalQueueLen int64                   `json:"global_queue_len"`
	WaitQueueLen   int64                   `json:"wait_queue_len"`
	RunningTasks   int64                   `json:"running_tasks"`
	AccountLoads   map[uint64]*AccountLoad `json:"account_loads"`
}
