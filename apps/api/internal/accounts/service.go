package accounts

import (
	"context"
	"log/slog"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/runtime/taskqueue"
)

type AccountTaskStore interface {
	ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error)
	FailRunningByAccountID(ctx context.Context, accountID int64, errMsg string) ([]string, error)
}

type AccountService struct {
	accountRepo      *AccountRepository
	taskRepo         AccountTaskStore
	queueStats       taskqueue.QueueStatsStore
	accountLoadStore taskqueue.AccountLoadStore
	concurrencyStore taskqueue.AccountConcurrencyStore
	cancelStore      taskqueue.TaskCancellationStore
	monitorStore     taskqueue.TaskMonitorStore
	taskEvents       events.TaskStatusPublisher
}

type AccountServiceDeps struct {
	AccountRepo      *AccountRepository
	TaskRepo         AccountTaskStore
	QueueStats       taskqueue.QueueStatsStore
	AccountLoadStore taskqueue.AccountLoadStore
	ConcurrencyStore taskqueue.AccountConcurrencyStore
	CancelStore      taskqueue.TaskCancellationStore
	MonitorStore     taskqueue.TaskMonitorStore
}

type AccountServiceOptions struct {
	TaskEvents events.TaskStatusPublisher
}

const defaultAccountMaxConcurrent = 3
const taskCancelTTL = 24 * time.Hour
const defaultAccountTimeoutSeconds = 120
const maxAccountMaxConcurrent = 64
const maxAccountTimeoutSeconds = 600

func NewAccountService(deps AccountServiceDeps, opts AccountServiceOptions) *AccountService {
	return &AccountService{
		accountRepo:      deps.AccountRepo,
		taskRepo:         deps.TaskRepo,
		queueStats:       deps.QueueStats,
		accountLoadStore: deps.AccountLoadStore,
		concurrencyStore: deps.ConcurrencyStore,
		cancelStore:      deps.CancelStore,
		monitorStore:     deps.MonitorStore,
		taskEvents:       opts.TaskEvents,
	}
}

func (s *AccountService) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	if err := events.PublishTaskStatusBestEffort(ctx, s.taskEvents, events.NewTaskStatusEvent(taskID, status)); err != nil {
		slog.Warn("publish task status event failed", "task_id", taskID, "status", status, "err", err)
	}
}
