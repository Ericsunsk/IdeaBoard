package users

import (
	"strconv"

	"api/internal/platform/response"
	"api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService *UserService
	configRepo  *settings.SystemConfigRepository
}

func NewAdminHandler(userService *UserService, configRepo *settings.SystemConfigRepository) *AdminHandler {
	return &AdminHandler{
		userService: userService,
		configRepo:  configRepo,
	}
}

// GetRegistrationSetting 获取注册开关状态
func (h *AdminHandler) GetRegistrationSetting(c *gin.Context) {
	val, err := h.configRepo.Get("auth_registration_enabled")
	if err != nil {
		response.Success(c, gin.H{"enabled": false})
		return
	}
	response.Success(c, gin.H{"enabled": val == "true"})
}

// UpdateRegistrationSetting 更新注册开关状态
func (h *AdminHandler) UpdateRegistrationSetting(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	val := "false"
	if req.Enabled {
		val = "true"
	}

	if err := h.configRepo.Set("auth_registration_enabled", val); err != nil {
		response.InternalError(c, 500, "保存设置失败", err)
		return
	}

	response.Success(c, gin.H{"enabled": req.Enabled})
}

// GetRateLimitSettings 获取频率限制设置
func (h *AdminHandler) GetRateLimitSettings(c *gin.Context) {
	windowStr, err1 := h.configRepo.Get("auth_rate_limit_window")
	maxReqStr, err2 := h.configRepo.Get("auth_rate_limit_max_requests")

	window := 60
	maxReq := 30

	if err1 == nil {
		if w, err := strconv.Atoi(windowStr); err == nil {
			window = w
		}
	}
	if err2 == nil {
		if m, err := strconv.Atoi(maxReqStr); err == nil {
			maxReq = m
		}
	}

	response.Success(c, gin.H{
		"window_seconds": window,
		"max_requests":   maxReq,
	})
}

// UpdateRateLimitSettings 更新频率限制设置
func (h *AdminHandler) UpdateRateLimitSettings(c *gin.Context) {
	var req struct {
		WindowSeconds int `json:"window_seconds"`
		MaxRequests   int `json:"max_requests"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	if req.WindowSeconds < 1 || req.WindowSeconds > 3600 {
		response.Error(c, 400, "window_seconds 必须在 1 到 3600 秒之间")
		return
	}
	if req.MaxRequests < 1 || req.MaxRequests > 10000 {
		response.Error(c, 400, "max_requests 必须在 1 到 10000 之间")
		return
	}

	if err := h.configRepo.Set("auth_rate_limit_window", strconv.Itoa(req.WindowSeconds)); err != nil {
		response.InternalError(c, 500, "保存设置失败", err)
		return
	}
	if err := h.configRepo.Set("auth_rate_limit_max_requests", strconv.Itoa(req.MaxRequests)); err != nil {
		response.InternalError(c, 500, "保存设置失败", err)
		return
	}

	response.Success(c, req)
}

// ListUsers 获取用户列表
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	result, err := h.userService.ListUsers(&req)
	if err != nil {
		response.InternalError(c, 500, "获取用户列表失败", err)
		return
	}

	response.Success(c, result)
}

// CreateUser 创建用户
func (h *AdminHandler) CreateUser(c *gin.Context) {
	response.Error(c, 400, "请使用注册页面创建账号，管理员后台已停用手工建号")
}

// UpdateUserStatus 更新用户状态
func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	var req struct {
		Status *int8 `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	if targetUser.Role == "admin" {
		response.Error(c, 400, "不能修改管理员状态")
		return
	}

	if err := h.userService.UpdateStatus(targetUser.ID, *req.Status); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}

// UpdateUserPassword 管理员修改用户密码
func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	response.Error(c, 400, "Better Auth 接管后不支持 Go 后台改密；当前高级选项未暴露改密，请走前台找回流程或受控身份运维流程")
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	response.Error(c, 400, "Better Auth 接管后已停用 Go 后台删除；请先封禁，确需物理删号时走受控身份运维流程")
}

func (h *AdminHandler) getTargetUserByParam(c *gin.Context) (*User, bool) {
	userID := c.Param("id")
	userIDUint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的用户ID")
		return nil, false
	}

	user, err := h.userService.GetByID(userIDUint)
	if err != nil {
		response.Error(c, 400, "用户不存在")
		return nil, false
	}

	return user, true
}
