package taskservice

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	taskService *Service
}

type submitTaskResponse struct {
	TaskID string `json:"task_id"`
}

type submitTaskReadinessResponse struct {
	Ready     bool                   `json:"ready"`
	Message   string                 `json:"message,omitempty"`
	ErrorCode string                 `json:"error_code,omitempty"`
	Feature   string                 `json:"feature,omitempty"`
	Billing   *taskbilling.Readiness `json:"billing,omitempty"`
}

type adminTaskResponse struct {
	Task AdminTaskMonitorDetail `json:"task"`
}

type adminTaskListResponse struct {
	Total          int64                      `json:"total"`
	PendingCount   int64                      `json:"pending_count"`
	RunningCount   int64                      `json:"running_count"`
	CompletedCount int64                      `json:"completed_count"`
	FailedCount    int64                      `json:"failed_count"`
	CancelledCount int64                      `json:"cancelled_count"`
	OtherCount     int64                      `json:"other_count"`
	List           []AdminTaskMonitorListItem `json:"list"`
}

type userTaskListResponse struct {
	Total int64                `json:"total"`
	List  []tasks.TaskUserView `json:"list"`
}

type userTaskResponse struct {
	Task tasks.TaskUserView `json:"task"`
}

type taskDeleteResponse struct {
	Message string `json:"message"`
}

type adminDeleteTasksRequest struct {
	TaskIDs []string `json:"task_ids"`
}

func NewHandler(taskService *Service) *Handler {
	return &Handler{taskService: taskService}
}

func requireUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
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
		var submissionErr *SubmissionError
		if errors.As(err, &submissionErr) {
			slog.Warn("task submission failed", "user_id", userID, "feature", req.Feature, "error_code", submissionErr.Code, "err", err)
			status := http.StatusConflict
			if submissionErr.Code == "provider_unavailable" || submissionErr.Code == "prompt_unavailable" || submissionErr.Code == "submission_failed" {
				status = http.StatusServiceUnavailable
			}
			response.MachineError(c, status, submissionErr.Code, submissionErr.SafeMessage)
			return
		}
		if errors.Is(err, taskbilling.ErrInsufficientCredits) {
			response.MachineError(c, http.StatusPaymentRequired, "insufficient_credits", "Idea不足")
			return
		}
		if errors.Is(err, taskbilling.ErrPricingUnavailable) {
			response.MachineError(c, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
			return
		}
		response.Error(c, 400, userFacingSubmitError(err))
		return
	}
	response.SuccessDTO(c, submitTaskResponse{TaskID: taskID})
}

func (h *Handler) CheckSubmitTaskReadiness(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var req tasks.SubmitTaskReadinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	readiness, err := h.taskService.CheckSubmitTaskReadiness(c.Request.Context(), userID, &req.SubmitTaskRequest, req.BillingQuantity)
	if err != nil {
		slog.Warn("task submit readiness failed", "user_id", userID, "feature", req.Feature, "err", err)
		if errors.Is(err, taskbilling.ErrPricingUnavailable) {
			response.MachineError(c, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
			return
		}
		var submissionErr *SubmissionError
		if errors.As(err, &submissionErr) {
			response.SuccessDTO(c, submitTaskReadinessResponse{Ready: false, Message: submissionErr.SafeMessage, ErrorCode: submissionErr.Code})
			return
		}
		response.SuccessDTO(c, submitTaskReadinessResponse{Ready: false, Message: userFacingSubmitError(err), ErrorCode: "submission_invalid"})
		return
	}
	ready := readiness.Billing == nil || readiness.Billing.Sufficient
	message := ""
	if !ready {
		message = "Idea不足"
	}
	response.SuccessDTO(c, submitTaskReadinessResponse{Ready: ready, Message: message, Feature: readiness.Feature, Billing: readiness.Billing})
}

func (h *Handler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.SuccessDTO(c, adminTaskResponse{Task: *task})
}

func (h *Handler) ListTasks(c *gin.Context) {
	var req tasks.TaskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := validateTaskListQueryIDs(c, req.UserID, req.AccountID); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	taskItems, counts, err := h.taskService.ListTasks(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "获取任务列表失败", err)
		return
	}
	response.SuccessDTO(c, adminTaskListResponse{
		Total:          counts.Total,
		PendingCount:   counts.PendingCount,
		RunningCount:   counts.RunningCount,
		CompletedCount: counts.CompletedCount,
		FailedCount:    counts.FailedCount,
		CancelledCount: counts.CancelledCount,
		OtherCount:     counts.OtherCount,
		List:           taskItems,
	})
}

func (h *Handler) DeleteTasks(c *gin.Context) {
	var req adminDeleteTasksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	result, err := h.taskService.DeleteTasks(c.Request.Context(), req.TaskIDs)
	if err != nil {
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "删除任务失败", err)
		return
	}
	response.SuccessDTO(c, result)
}

func validateTaskListQueryIDs(c *gin.Context, userID int64, accountID int64) error {
	if c.Query("user_id") != "" && userID <= 0 {
		return validation.New("user_id 必须大于 0")
	}
	if c.Query("account_id") != "" && accountID <= 0 {
		return validation.New("account_id 必须大于 0")
	}
	return nil
}

// userFacingSubmitError maps internal errors to user-safe messages.
// Validation errors and expected user-facing sentinels pass through unchanged.
func userFacingSubmitError(err error) string {
	if err == nil {
		return "未知错误"
	}
	if errors.Is(err, errNoAvailableAPIAccount) {
		return providerUnavailableMessage
	}
	msg := err.Error()
	// Known user-facing messages from the validation layer are safe to expose.
	if strings.Contains(msg, "提示词") || strings.Contains(msg, "图片") || strings.Contains(msg, "参数") ||
		strings.Contains(msg, "格式") || strings.Contains(msg, "数量") || strings.Contains(msg, "大小") ||
		strings.Contains(msg, "feature") || strings.Contains(msg, "任务提交失败") {
		return msg
	}
	return "任务提交失败，请稍后重试"
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
	taskItems, total, err := h.taskService.GetUserTasks(c.Request.Context(), userID, req.Page, req.PageSize)
	if err != nil {
		response.InternalError(c, 500, "获取任务列表失败", err)
		return
	}
	safeList := make([]tasks.TaskUserView, len(taskItems))
	for i, t := range taskItems {
		safeList[i] = t.ToUserView()
	}
	response.SuccessDTO(c, userTaskListResponse{Total: total, List: safeList})
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
	response.SuccessDTO(c, userTaskResponse{Task: task.ToUserView()})
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

	session := taskStreamSession{
		taskID:      taskID,
		userID:      userID,
		initialTask: task,
		taskLoader:  h.taskService,
		events:      hub,
	}
	if err := session.Run(c); errors.Is(err, errTaskStreamConnectionLimit) {
		response.Error(c, http.StatusTooManyRequests, "SSE 连接数已达上限，请稍后重试")
	}
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
	response.SuccessDTO(c, taskDeleteResponse{Message: "删除成功"})
}

func (h *Handler) CancelUserTask(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	taskID := c.Param("task_id")
	if err := h.taskService.CancelUserTask(c.Request.Context(), taskID, userID); err != nil {
		response.Error(c, 500, "停止失败")
		return
	}
	response.SuccessDTO(c, taskDeleteResponse{Message: "停止成功"})
}
