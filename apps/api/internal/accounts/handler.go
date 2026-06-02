package accounts

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"api/internal/platform/response"
	"api/internal/tasks"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	accountService *AccountService
}

func NewAccountHandler(accountService *AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// ============ 账号池 API ============

func (h *AccountHandler) ListAccounts(c *gin.Context) {
	var req AccountListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	ctx := c.Request.Context()
	accounts, total, err := h.accountService.ListAccounts(ctx, &req)
	if err != nil {
		response.InternalError(c, 500, "获取账号列表失败", err)
		return
	}
	response.Success(c, gin.H{"total": total, "list": accounts})
}

func (h *AccountHandler) GetAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	ctx := c.Request.Context()
	account, err := h.accountService.GetAccount(ctx, id)
	if err != nil {
		response.Error(c, 404, "账号不存在")
		return
	}
	concurrent, maxConcurrent, _ := h.accountService.GetAccountQueueInfo(ctx, id)
	response.Success(c, gin.H{"account": account, "current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}

func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	account, err := h.accountService.CreateAccount(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 500, "创建失败", err)
		return
	}
	response.Success(c, gin.H{"account": account})
}

func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.accountService.UpdateAccount(c.Request.Context(), id, &req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.DeleteAccount(c.Request.Context(), id); err != nil {
		response.InternalError(c, 500, "删除失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StartAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StartAccount(c.Request.Context(), id); err != nil {
		response.InternalError(c, 500, "启动失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StopAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StopAccount(c.Request.Context(), id); err != nil {
		response.InternalError(c, 500, "停止失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) GetAccountQueue(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	concurrent, maxConcurrent, err := h.accountService.GetAccountQueueInfo(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.Success(c, gin.H{"current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}

// ============ 任务 API ============

func (h *AccountHandler) SubmitTask(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req tasks.SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	taskID, err := h.accountService.SubmitTask(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"task_id": taskID})
}

func (h *AccountHandler) CheckSubmitTaskReadiness(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req tasks.SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	featureSlug, err := h.accountService.CheckSubmitTaskReadiness(c.Request.Context(), userID, &req)
	if err != nil {
		response.Success(c, gin.H{"ready": false, "message": err.Error()})
		return
	}
	response.Success(c, gin.H{"ready": true, "feature": featureSlug})
}

func (h *AccountHandler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task, err := h.accountService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": task})
}

func (h *AccountHandler) ListTasks(c *gin.Context) {
	var req tasks.TaskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	tasks, total, err := h.accountService.ListTasks(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 500, "获取任务列表失败", err)
		return
	}
	response.Success(c, gin.H{"total": total, "list": tasks})
}

func (h *AccountHandler) GetUserTasks(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req tasks.TaskListRequest
	c.ShouldBindQuery(&req)
	tasks, total, _ := h.accountService.GetUserTasks(c.Request.Context(), userID, req.Page, req.PageSize)
	// 返回用户安全视图，隐藏内部关键词
	safeList := make([]map[string]interface{}, len(tasks))
	for i, t := range tasks {
		safeList[i] = t.ToUserView()
	}
	response.Success(c, gin.H{"total": total, "list": safeList})
}

// GetUserTask 用户端获取单个任务详情（隐藏内部关键词）
func (h *AccountHandler) GetUserTask(c *gin.Context) {
	userID := c.GetInt64("user_id")
	taskID := c.Param("task_id")
	task, err := h.accountService.GetUserTask(c.Request.Context(), taskID, userID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": task.ToUserView()})
}

// StreamUserTask streams user-safe task updates over Server-Sent Events.
func (h *AccountHandler) StreamUserTask(c *gin.Context) {
	userID := c.GetInt64("user_id")
	taskID := c.Param("task_id")
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
	}
	ctx := c.Request.Context()
	task, err := h.accountService.GetUserTask(ctx, taskID, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "任务不存在")
		return
	}
	hub := h.accountService.TaskEventHub()
	if hub == nil {
		response.Error(c, http.StatusServiceUnavailable, "任务事件服务未配置")
		return
	}

	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil {
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

	// Re-read after subscribing to close the race between initial load and status publish.
	task, err = h.accountService.GetUserTask(ctx, taskID, userID)
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
			latest, err := h.accountService.GetUserTask(ctx, taskID, userID)
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

func writeTaskSSEvent(c *gin.Context, task *tasks.Task) bool {
	return writeSSEvent(c, "task", task.ToUserView())
}

func writeSSEvent(c *gin.Context, event string, data interface{}) bool {
	payload, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

// DeleteUserTask 用户删除自己的任务
func (h *AccountHandler) DeleteUserTask(c *gin.Context) {
	userID := c.GetInt64("user_id")
	taskID := c.Param("task_id")
	if err := h.accountService.DeleteUserTask(c.Request.Context(), taskID, userID); err != nil {
		response.Error(c, 500, "删除失败")
		return
	}
	response.Success(c, gin.H{"message": "删除成功"})
}

// ============ 监控 API ============

func (h *AccountHandler) GetMonitor(c *gin.Context) {
	ctx := c.Request.Context()
	info, err := h.accountService.GetMonitorInfo(ctx)
	if err != nil {
		response.InternalError(c, 500, "获取监控数据失败", err)
		return
	}
	stats, _ := h.accountService.GetTaskStats(ctx)
	response.Success(c, gin.H{"monitor": info, "stats": stats})
}

func (h *AccountHandler) GetStatistics(c *gin.Context) {
	stats, err := h.accountService.GetStatistics(c.Request.Context(), c.Query("range"))
	if err != nil {
		response.InternalError(c, 500, "获取统计数据失败", err)
		return
	}
	response.Success(c, stats)
}

func (h *AccountHandler) BackfillStatistics(c *gin.Context) {
	var req struct {
		Range string `json:"range"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		if err != io.EOF {
			response.Error(c, 400, "请求参数错误")
			return
		}
	}
	req.Range = strings.TrimSpace(req.Range)
	if req.Range == "" {
		req.Range = string(StatisticsRange7D)
	}

	result, err := h.accountService.BackfillStatistics(c.Request.Context(), req.Range)
	if err != nil {
		response.InternalError(c, 500, "补录统计失败", err)
		return
	}
	response.Success(c, result)
}
