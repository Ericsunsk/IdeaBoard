package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"api/internal/accounts"
	"api/internal/channels"
	"api/internal/runtime/taskqueue"
	"api/internal/storage"

	"github.com/hibiken/asynq"
)

type AsynqTaskHandler struct {
	processor *taskProcessor
}

type AsynqServerOptions struct {
	QueueName            string
	Concurrency          int
	NoCapacityRetryDelay time.Duration
}

func NewAsynqTaskHandler(
	accountRepo *accounts.AccountRepository,
	channelRepo *channels.ChannelRepository,
	taskRepo *accounts.TaskRepository,
	enqueuer taskqueue.TaskEnqueuer,
	assigner taskqueue.TaskAssigner,
	cancelStore taskqueue.TaskCancellationStore,
	storageService *storage.StorageService,
	taskEventHub *accounts.TaskEventHub,
) *AsynqTaskHandler {
	executor := &ProviderExecutor{
		channelRepo:    channelRepo,
		storageService: storageService,
	}
	taskRuntime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)
	taskRuntime.SetEventHub(taskEventHub)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, enqueuer, taskRuntime, executor.callDrawingAPI)
	return newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, taskRuntime, orchestrator.execute, false))
}

func newAsynqTaskHandlerWithProcessor(processor *taskProcessor) *AsynqTaskHandler {
	return &AsynqTaskHandler{processor: processor}
}

func (h *AsynqTaskHandler) HandleTask(ctx context.Context, task *asynq.Task) error {
	if task.Type() != taskqueue.AsynqTaskTypeDrawingGenerate {
		return fmt.Errorf("%w: unsupported task type %q", asynq.SkipRetry, task.Type())
	}
	decoded, err := taskqueue.DecodeAsynqPayload(task.Payload())
	if err != nil {
		return fmt.Errorf("%w: invalid asynq payload: %v", asynq.SkipRetry, err)
	}
	if err := h.processor.Process(ctx, decoded); err != nil {
		return err
	}
	return nil
}

func NewAsynqServer(redisOpt asynq.RedisConnOpt, opts AsynqServerOptions, handler *AsynqTaskHandler) (*asynq.Server, *asynq.ServeMux) {
	queueName := opts.QueueName
	if queueName == "" {
		queueName = taskqueue.AsynqQueueDefault
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	noCapacityRetryDelay := opts.NoCapacityRetryDelay
	if noCapacityRetryDelay <= 0 {
		noCapacityRetryDelay = 5 * time.Second
	}

	cfg := asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			queueName: 1,
		},
		IsFailure: func(err error) bool {
			return !errors.Is(err, taskqueue.ErrNoAccountCapacity)
		},
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			if errors.Is(err, taskqueue.ErrNoAccountCapacity) {
				return noCapacityRetryDelay
			}
			return asynq.DefaultRetryDelayFunc(n, err, task)
		},
	}

	server := asynq.NewServer(redisOpt, cfg)
	mux := asynq.NewServeMux()
	mux.HandleFunc(taskqueue.AsynqTaskTypeDrawingGenerate, handler.HandleTask)
	return server, mux
}

func BuildAsynqRedisConnOpt(host, port, auth string) asynq.RedisConnOpt {
	return asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: auth,
	}
}
