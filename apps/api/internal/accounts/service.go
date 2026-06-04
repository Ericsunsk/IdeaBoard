package accounts

import (
	"context"
	"log/slog"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/storage"
)

type AccountService struct {
	accountRepo       *AccountRepository
	taskRepo          *TaskRepository
	queue             taskqueue.TaskEnqueuer
	queueStats        taskqueue.QueueStatsStore
	accountLoadStore  taskqueue.AccountLoadStore
	concurrencyStore  taskqueue.AccountConcurrencyStore
	cancelStore       taskqueue.TaskCancellationStore
	monitorStore      taskqueue.TaskMonitorStore
	taskEventHub      *TaskEventHub
	taskEvents        events.TaskStatusPublisher
	systemConfigRepo  *settings.SystemConfigRepository
	uploadBoards      storage.UploadBoardAccessResolver
	uploadOwners      storage.UploadOwnerStorageUIDResolver
	taskOutputCleaner TaskOutputObjectCleaner
}

type TaskOutputObjectCleaner interface {
	DeleteTaskOutputsByUserTask(ctx context.Context, userID int64, taskID string) error
}

type AccountServiceDeps struct {
	AccountRepo       *AccountRepository
	TaskRepo          *TaskRepository
	Queue             taskqueue.TaskEnqueuer
	QueueStats        taskqueue.QueueStatsStore
	AccountLoadStore  taskqueue.AccountLoadStore
	ConcurrencyStore  taskqueue.AccountConcurrencyStore
	CancelStore       taskqueue.TaskCancellationStore
	MonitorStore      taskqueue.TaskMonitorStore
	SystemConfigRepo  *settings.SystemConfigRepository
	UploadBoards      storage.UploadBoardAccessResolver
	UploadOwners      storage.UploadOwnerStorageUIDResolver
	TaskOutputCleaner TaskOutputObjectCleaner
}

type AccountServiceOptions struct {
	TaskEventHub *TaskEventHub
	TaskEvents   events.TaskStatusPublisher
}

const defaultAccountMaxConcurrent = 3
const taskCancelTTL = 24 * time.Hour
const defaultAccountTimeoutSeconds = 120
const maxTaskKeywordLength = 200
const maxSubmitTaskPromptBytes = 8 * 1024
const maxSubmitTaskFinalPromptBytes = 32 * 1024
const maxSubmitTaskImageURLs = 16
const maxSubmitTaskImageURLBytes = 4096
const maxSubmitTaskParamsBytes = 2 * 1024 * 1024
const maxSubmitTaskParamsDepth = 24
const maxSubmitTaskMediaDataURLBytes = 10 * 1024 * 1024
const maxAccountMaxConcurrent = 64
const maxAccountTimeoutSeconds = 600
const noAvailableAPIAccountMessage = "没有可用的 API 账号，请先添加账号"

func NewAccountService(deps AccountServiceDeps, opts AccountServiceOptions) *AccountService {
	return &AccountService{
		accountRepo:       deps.AccountRepo,
		taskRepo:          deps.TaskRepo,
		queue:             deps.Queue,
		queueStats:        deps.QueueStats,
		accountLoadStore:  deps.AccountLoadStore,
		concurrencyStore:  deps.ConcurrencyStore,
		cancelStore:       deps.CancelStore,
		monitorStore:      deps.MonitorStore,
		systemConfigRepo:  deps.SystemConfigRepo,
		uploadBoards:      deps.UploadBoards,
		uploadOwners:      deps.UploadOwners,
		taskOutputCleaner: deps.TaskOutputCleaner,
		taskEventHub:      opts.TaskEventHub,
		taskEvents:        opts.TaskEvents,
	}
}

func (s *AccountService) TaskEventHub() *TaskEventHub {
	return s.taskEventHub
}

func (s *AccountService) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	if err := events.PublishTaskStatusBestEffort(ctx, s.taskEvents, events.NewTaskStatusEvent(taskID, status)); err != nil {
		slog.Warn("publish task status event failed", "task_id", taskID, "status", status, "err", err)
	}
}
