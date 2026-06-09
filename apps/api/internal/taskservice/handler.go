package taskservice

import (
	"log/slog"
	"net/http"
	"time"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/tasks"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	taskService *Service
}

type adminTaskDTO struct {
	ID                int64                       `json:"id"`
	TaskID            string                      `json:"task_id"`
	UserID            int64                       `json:"user_id"`
	AssignedChannelID *int64                      `json:"assigned_channel_id"`
	AccountID         *int64                      `json:"account_id"`
	Prompt            string                      `json:"prompt"`
	UserPrompt        string                      `json:"user_prompt"`
	Params            string                      `json:"params"`
	FeatureSlug       string                      `json:"feature_slug"`
	RequestedModel    string                      `json:"requested_model"`
	ResolvedModel     string                      `json:"resolved_model"`
	APIType           string                      `json:"api_type"`
	Status            string                      `json:"status"`
	RetryCount        int                         `json:"retry_count"`
	ErrorMsg          string                      `json:"error_msg"`
	Result            string                      `json:"result"`
	Outputs           []tasks.TaskOutput          `json:"outputs,omitempty"`
	ExecutionMetrics  *tasks.TaskExecutionMetrics `json:"execution_metrics,omitempty"`
	CreatedAt         time.Time                   `json:"created_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
	CompletedAt       *time.Time                  `json:"completed_at"`
	CancelledAt       *time.Time                  `json:"cancelled_at"`
	CancelledBy       *int64                      `json:"cancelled_by"`
	CancelReason      string                      `json:"cancel_reason"`
}

func NewHandler(taskService *Service) *Handler {
	return &Handler{taskService: taskService}
}

func requireUserID(c *gin.Context) (int64, bool) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return 0, false
	}
	return userID, true
}

func (h *Handler) SubmitTask(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req tasks.SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	taskID, err := h.taskService.SubmitTask(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"task_id": taskID})
}

func (h *Handler) CheckSubmitTaskReadiness(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req tasks.SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	featureSlug, err := h.taskService.CheckSubmitTaskReadiness(c.Request.Context(), userID, &req)
	if err != nil {
		response.Success(c, gin.H{"ready": false, "message": err.Error()})
		return
	}
	response.Success(c, gin.H{"ready": true, "feature": featureSlug})
}

func (h *Handler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": toAdminTaskDTO(task)})
}

func (h *Handler) ListTasks(c *gin.Context) {
	var req tasks.TaskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	taskItems, counts, err := h.taskService.ListTasks(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 500, "获取任务列表失败", err)
		return
	}
	response.Success(c, gin.H{
		"total":           counts.Total,
		"pending_count":   counts.PendingCount,
		"running_count":   counts.RunningCount,
		"completed_count": counts.CompletedCount,
		"failed_count":    counts.FailedCount,
		"cancelled_count": counts.CancelledCount,
		"other_count":     counts.OtherCount,
		"list":            toAdminTaskDTOs(taskItems),
	})
}

func (h *Handler) GetUserTasks(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req tasks.TaskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	tasks, total, err := h.taskService.GetUserTasks(c.Request.Context(), userID, req.Page, req.PageSize)
	if err != nil {
		response.InternalError(c, 500, "获取任务列表失败", err)
		return
	}
	safeList := make([]map[string]interface{}, len(tasks))
	for i, t := range tasks {
		safeList[i] = t.ToUserView()
	}
	response.Success(c, gin.H{"total": total, "list": safeList})
}

func (h *Handler) GetUserTask(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	taskID := c.Param("task_id")
	task, err := h.taskService.GetUserTask(c.Request.Context(), taskID, userID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": task.ToUserView()})
}

func (h *Handler) StreamUserTask(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	taskID := c.Param("task_id")
	ctx := c.Request.Context()
	task, err := h.taskService.GetUserTask(ctx, taskID, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "任务不存在")
		return
	}
	hub := h.taskService.EventHub()
	if hub == nil {
		response.Error(c, http.StatusServiceUnavailable, "任务事件服务未配置")
		return
	}

	if err := sse.Prepare(c); err != nil {
		slog.Warn("clear SSE write deadline failed", "task_id", taskID, "err", err)
	}

	if tasks.IsTerminalStatus(task.Status) {
		if !writeTaskSSEvent(c, task) {
			return
		}
		return
	}

	events, unsubscribe, ok := hub.Subscribe(taskID, userID)
	if !ok {
		response.Error(c, http.StatusTooManyRequests, "SSE 连接数已达上限，请稍后重试")
		return
	}
	defer unsubscribe()

	task, err = h.taskService.GetUserTask(ctx, taskID, userID)
	if err != nil {
		return
	}
	if !writeTaskSSEvent(c, task) {
		return
	}
	if tasks.IsTerminalStatus(task.Status) {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	maxConnTimer := time.NewTimer(defaultSSEMaxConnPeriod)
	defer maxConnTimer.Stop()

	for {
		select {
		case <-maxConnTimer.C:
			_ = writeSSEvent(c, "close", gin.H{"reason": "max_connection_duration_reached"})
			return
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if !writeSSEvent(c, "ping", gin.H{"ts": time.Now().Unix()}) {
				return
			}
		case <-events:
			latest, err := h.taskService.GetUserTask(ctx, taskID, userID)
			if err != nil {
				return
			}
			if !writeTaskSSEvent(c, latest) {
				return
			}
			if tasks.IsTerminalStatus(latest.Status) {
				return
			}
		}
	}
}

func toAdminTaskDTOs(items []*tasks.Task) []adminTaskDTO {
	result := make([]adminTaskDTO, 0, len(items))
	for _, item := range items {
		result = append(result, toAdminTaskDTO(item))
	}
	return result
}

func toAdminTaskDTO(task *tasks.Task) adminTaskDTO {
	if task == nil {
		return adminTaskDTO{}
	}
	return adminTaskDTO{
		ID:                task.ID,
		TaskID:            task.TaskID,
		UserID:            task.UserID,
		AssignedChannelID: task.AssignedChannelID,
		AccountID:         task.AccountID,
		Prompt:            task.Prompt,
		UserPrompt:        task.UserPrompt,
		Params:            task.Params,
		FeatureSlug:       task.FeatureSlug,
		RequestedModel:    task.RequestedModel,
		ResolvedModel:     task.ResolvedModel,
		APIType:           task.APIType,
		Status:            task.Status,
		RetryCount:        task.RetryCount,
		ErrorMsg:          task.ErrorMsg,
		Result:            task.Result,
		Outputs:           task.Outputs,
		ExecutionMetrics:  task.ExecutionMetrics,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		CompletedAt:       task.CompletedAt,
		CancelledAt:       task.CancelledAt,
		CancelledBy:       task.CancelledBy,
		CancelReason:      task.CancelReason,
	}
}

func writeTaskSSEvent(c *gin.Context, task *tasks.Task) bool {
	return writeSSEvent(c, "task", task.ToUserView())
}

func writeSSEvent(c *gin.Context, event string, data interface{}) bool {
	return sse.WriteEvent(c, event, data) == nil
}

func (h *Handler) DeleteUserTask(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	taskID := c.Param("task_id")
	if err := h.taskService.DeleteUserTask(c.Request.Context(), taskID, userID); err != nil {
		response.Error(c, 500, "删除失败")
		return
	}
	response.Success(c, gin.H{"message": "删除成功"})
}
