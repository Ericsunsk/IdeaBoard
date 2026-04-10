package handler

import (
	"fmt"

	"backend/internal/model"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	accountService *service.AccountService
}

func NewAccountHandler(accountService *service.AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// ============ 账号池 API ============

func (h *AccountHandler) ListAccounts(c *gin.Context) {
	var req model.AccountListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	accounts, total, err := h.accountService.ListAccounts(&req)
	if err != nil {
		response.Error(c, 500, "获取账号列表失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"total": total, "list": accounts})
}

func (h *AccountHandler) GetAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	account, err := h.accountService.GetAccount(id)
	if err != nil {
		response.Error(c, 404, "账号不存在")
		return
	}
	concurrent, maxConcurrent, _ := h.accountService.GetAccountQueueInfo(id)
	response.Success(c, gin.H{"account": account, "current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}

func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req model.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	account, err := h.accountService.CreateAccount(&req)
	if err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"account": account})
}

func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	var req model.UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	if err := h.accountService.UpdateAccount(id, &req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.DeleteAccount(id); err != nil {
		response.Error(c, 500, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StartAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StartAccount(id); err != nil {
		response.Error(c, 500, "启动失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StopAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StopAccount(id); err != nil {
		response.Error(c, 500, "停止失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) GetAccountQueue(c *gin.Context) {
	idStr := c.Param("id")
	var id uint64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	concurrent, maxConcurrent, err := h.accountService.GetAccountQueueInfo(id)
	if err != nil {
		response.Error(c, 500, "获取失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}

// ============ 任务 API ============

func (h *AccountHandler) SubmitTask(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req model.SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	taskID, err := h.accountService.SubmitTask(userID, &req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"task_id": taskID})
}

func (h *AccountHandler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task, err := h.accountService.GetTask(taskID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": task})
}

func (h *AccountHandler) ListTasks(c *gin.Context) {
	var req model.TaskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	tasks, total, err := h.accountService.ListTasks(&req)
	if err != nil {
		response.Error(c, 500, "获取任务列表失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"total": total, "list": tasks})
}

func (h *AccountHandler) GetUserTasks(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var req model.TaskListRequest
	c.ShouldBindQuery(&req)
	tasks, total, _ := h.accountService.GetUserTasks(userID, req.Page, req.PageSize)
	// 返回用户安全视图，隐藏内部关键词
	safeList := make([]map[string]interface{}, len(tasks))
	for i, t := range tasks {
		safeList[i] = t.ToUserView()
	}
	response.Success(c, gin.H{"total": total, "list": safeList})
}

// GetUserTask 用户端获取单个任务详情（隐藏内部关键词）
func (h *AccountHandler) GetUserTask(c *gin.Context) {
	userID := c.GetUint64("user_id")
	taskID := c.Param("task_id")
	task, err := h.accountService.GetUserTask(taskID, userID)
	if err != nil {
		response.Error(c, 404, "任务不存在")
		return
	}
	response.Success(c, gin.H{"task": task.ToUserView()})
}

// DeleteUserTask 用户删除自己的任务
func (h *AccountHandler) DeleteUserTask(c *gin.Context) {
	userID := c.GetUint64("user_id")
	taskID := c.Param("task_id")
	if err := h.accountService.DeleteUserTask(taskID, userID); err != nil {
		response.Error(c, 500, "删除失败")
		return
	}
	response.Success(c, gin.H{"message": "删除成功"})
}

// ============ 监控 API ============

func (h *AccountHandler) GetMonitor(c *gin.Context) {
	info, _ := h.accountService.GetMonitorInfo()
	stats, _ := h.accountService.GetTaskStats()
	response.Success(c, gin.H{"monitor": info, "stats": stats})
}
