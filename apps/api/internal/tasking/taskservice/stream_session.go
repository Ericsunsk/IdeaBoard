package taskservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/gin-gonic/gin"
)

const (
	defaultSSEMaxConnPeriod       = 30 * time.Minute
	taskStreamFullRefreshEveryN   = 4
	taskStreamCloseMaxConnReason  = "max_connection_duration_reached"
	taskStreamFetchFailedReason   = "fetch_failed"
	taskStreamInitialSnapshotName = "initial_snapshot"
)

var (
	errTaskStreamConnectionLimit = errors.New("task stream connection limit reached")
	taskStreamHeartbeatInterval  = 15 * time.Second
)

type taskStreamTaskLoader interface {
	GetUserTask(ctx context.Context, taskID string, userID int64) (*tasks.Task, error)
}

type taskStreamEventSource interface {
	Subscribe(taskID string, userID int64) (<-chan TaskEvent, func(), bool)
}

type taskStreamSession struct {
	taskID      string
	userID      int64
	initialTask *tasks.Task
	taskLoader  taskStreamTaskLoader
	events      taskStreamEventSource
}

func (s taskStreamSession) Run(c *gin.Context) error {
	if tasks.IsTerminalStatus(s.initialTask.Status) {
		s.prepare(c)
		writeTaskSSEvent(c, s.initialTask)
		return nil
	}

	events, unsubscribe, ok := s.events.Subscribe(s.taskID, s.userID)
	if !ok {
		return errTaskStreamConnectionLimit
	}
	defer unsubscribe()

	s.prepare(c)
	if !s.sendLatestTask(c, taskStreamInitialSnapshotName) {
		return nil
	}

	heartbeat := time.NewTicker(taskStreamHeartbeatInterval)
	defer heartbeat.Stop()
	maxConnTimer := time.NewTimer(defaultSSEMaxConnPeriod)
	defer maxConnTimer.Stop()

	heartbeatTicks := 0
	for {
		select {
		case <-maxConnTimer.C:
			_ = writeSSEvent(c, "close", gin.H{"reason": taskStreamCloseMaxConnReason})
			return nil
		case <-c.Request.Context().Done():
			return nil
		case <-heartbeat.C:
			heartbeatTicks++
			if heartbeatTicks%taskStreamFullRefreshEveryN == 0 {
				if !s.sendLatestTask(c, "heartbeat") {
					return nil
				}
				continue
			}
			if !writeTaskKeepalive(c) {
				return nil
			}
		case <-events:
			if !s.sendLatestTask(c, "event") {
				return nil
			}
		}
	}
}

func (s taskStreamSession) prepare(c *gin.Context) {
	if err := sse.Prepare(c); err != nil {
		slog.Warn("clear SSE write deadline failed", "task_id", s.taskID, "err", err)
	}
}

func (s taskStreamSession) sendLatestTask(c *gin.Context, source string) bool {
	latest, err := s.taskLoader.GetUserTask(c.Request.Context(), s.taskID, s.userID)
	if err != nil {
		writeTaskStatusFetchErrorSSE(c, s.taskID, s.userID, source, err)
		return false
	}
	if !writeTaskSSEvent(c, latest) {
		return false
	}
	return !tasks.IsTerminalStatus(latest.Status)
}

func writeTaskSSEvent(c *gin.Context, task *tasks.Task) bool {
	return writeSSEvent(c, "task", task.ToUserView())
}

func writeTaskKeepalive(c *gin.Context) bool {
	if _, err := fmt.Fprintf(c.Writer, ":keepalive\n\n"); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

func writeTaskStatusFetchErrorSSE(c *gin.Context, taskID string, userID int64, source string, err error) {
	slog.Warn("fetch SSE task status failed", "task_id", taskID, "user_id", userID, "source", source, "err", err)
	if writeErr := sse.WriteEvent(c, "error", gin.H{"message": "获取任务状态失败"}); writeErr != nil {
		slog.Warn("write SSE error event failed", "task_id", taskID, "user_id", userID, "source", source, "err", writeErr)
	}
	sse.WriteEvent(c, "close", gin.H{"reason": taskStreamFetchFailedReason})
}

func writeSSEvent(c *gin.Context, event string, data interface{}) bool {
	return sse.WriteEvent(c, event, data) == nil
}
