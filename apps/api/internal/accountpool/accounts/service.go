package accounts

import (
	"context"
	"time"

	"ideaboard/api/internal/events"
	taskqueue "ideaboard/api/internal/tasking/ports/queue"
)

type AccountTaskStore interface {
	ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error)
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

const taskCancelTTL = 24 * time.Hour

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
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}
