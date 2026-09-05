package ports

import (
	taskqueue "ideaboard/api/internal/tasking/ports/queue"
)

type QueueStatsStore = taskqueue.QueueStatsStore
type AccountLoadStore = taskqueue.AccountLoadStore
type AccountConcurrencyStore = taskqueue.AccountConcurrencyStore
type TaskCancellationStore = taskqueue.TaskCancellationStore
type TaskMonitorStore = taskqueue.TaskMonitorStore
