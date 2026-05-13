package users

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"api/internal/autherrors"
	"api/internal/platform/response"
	"api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService *UserService
	configRepo  *settings.SystemConfigRepository
	authService AuthUserManager
}

type AuthUserManager interface {
	DeleteUser(ctx context.Context, authUserID string) error
	SetUserRole(ctx context.Context, authUserID string, role string) error
}

func NewAdminHandler(userService *UserService, configRepo *settings.SystemConfigRepository, authService AuthUserManager) *AdminHandler {
	return &AdminHandler{
		userService: userService,
		configRepo:  configRepo,
		authService: authService,
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

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	nextRole := normalizeAdminRole(req.Role)
	if nextRole == "" {
		response.Error(c, 400, "role 仅支持 admin 或 user")
		return
	}

	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	currentUserID := c.GetUint64("user_id")
	if currentUserID != 0 && currentUserID == targetUser.ID {
		response.Error(c, 400, "不能修改当前登录管理员自己的角色")
		return
	}

	authUserID := strings.TrimSpace(derefString(targetUser.AuthUserID))
	if authUserID == "" {
		response.Error(c, 400, "目标用户缺少 auth_user_id，已拒绝改角色以避免状态漂移")
		return
	}
	if h.authService == nil {
		response.InternalError(c, 500, "认证服务未配置", errors.New("admin auth service client not configured"))
		return
	}

	currentRole := normalizeAdminRole(targetUser.Role)
	if currentRole == "" {
		currentRole = "user"
	}
	if currentRole == nextRole {
		response.Success(c, nil)
		return
	}

	if nextRole == "admin" {
		if err := h.authService.SetUserRole(c.Request.Context(), authUserID, nextRole); err != nil {
			response.InternalError(c, 502, "同步 Auth 管理员角色失败", err)
			return
		}
		if err := h.userService.UpdateRole(targetUser.ID, nextRole); err != nil {
			_ = h.authService.SetUserRole(c.Request.Context(), authUserID, currentRole)
			response.InternalError(c, 500, "业务侧角色更新失败", err)
			return
		}
		response.Success(c, nil)
		return
	}

	if err := h.userService.UpdateRole(targetUser.ID, nextRole); err != nil {
		response.InternalError(c, 500, "业务侧角色更新失败", err)
		return
	}
	if err := h.authService.SetUserRole(c.Request.Context(), authUserID, nextRole); err != nil {
		log.Printf("[users] role drift detected: local updated but auth sync failed user_id=%d auth_user_id=%s target_role=%s", targetUser.ID, authUserID, nextRole)
		response.InternalError(c, 502, "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理", err)
		return
	}

	response.Success(c, nil)
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	currentUserID := c.GetUint64("user_id")
	if currentUserID != 0 && currentUserID == targetUser.ID {
		response.Error(c, 400, "不能彻底注销当前登录管理员自己")
		return
	}
	if targetUser.Role == "admin" {
		response.Error(c, 400, "不能彻底注销管理员账号")
		return
	}

	authUserID := strings.TrimSpace(derefString(targetUser.AuthUserID))
	if authUserID == "" {
		response.Error(c, 400, "目标用户缺少 auth_user_id，已拒绝删除以避免状态漂移")
		return
	}
	if h.authService == nil {
		response.InternalError(c, 500, "认证服务未配置", errors.New("admin auth service client not configured"))
		return
	}

	if err := h.authService.DeleteUser(c.Request.Context(), authUserID); err != nil {
		if !errors.Is(err, autherrors.ErrAuthUserNotFound) {
			response.InternalError(c, 502, "删除 Auth 用户失败", err)
			return
		}
		log.Printf("[users] auth user already missing during admin delete, continue local cleanup user_id=%d auth_user_id=%s", targetUser.ID, authUserID)
	}

	if err := h.userService.DeleteUser(targetUser.ID); err != nil {
		response.InternalError(c, 500, "业务侧数据清理失败（Auth 已删除，请按运维流程补偿处理）", err)
		return
	}

	response.Success(c, nil)
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeAdminRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	if value == "admin" || value == "user" {
		return value
	}
	return ""
}
