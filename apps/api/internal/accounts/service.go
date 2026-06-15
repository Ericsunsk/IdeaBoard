package accounts

import (
	"context"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/runtime/taskqueue"
)

type AccountTaskStore interface {
	ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error)
	FailRunningByAccountIDTx(ctx context.Context, q *dbsqlc.Queries, accountID int64, errMsg string, completedAt time.Time) ([]string, error)
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

const defaultAccountMaxConcurrent = accountlimits.DefaultMaxConcurrent
const taskCancelTTL = 24 * time.Hour
const defaultAccountTimeoutSeconds = 120
const maxAccountMaxConcurrent = accountlimits.MaxMaxConcurrent
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
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}
