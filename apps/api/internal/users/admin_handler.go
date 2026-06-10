package users

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"ideaboard/api/internal/autherrors"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"

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
	BanUser(ctx context.Context, authUserID string, reason string, expiresInSeconds int) error
	UnbanUser(ctx context.Context, authUserID string) error
	RevokeUserSessions(ctx context.Context, authUserID string) error
}

type adminActionError struct {
	status int
	msg    string
	err    error
}

func (e *adminActionError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.msg) != "" {
		return e.msg
	}
	if e.err != nil {
		return e.err.Error()
	}
	return "admin action failed"
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
	val, err := h.configRepo.Get(c.Request.Context(), "auth_registration_enabled")
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

	if err := h.configRepo.Set(c.Request.Context(), "auth_registration_enabled", val); err != nil {
		response.InternalError(c, 500, "保存设置失败", err)
		return
	}

	response.Success(c, gin.H{"enabled": req.Enabled})
}

// GetRateLimitSettings 获取频率限制设置
func (h *AdminHandler) GetRateLimitSettings(c *gin.Context) {
	windowStr, err1 := h.configRepo.Get(c.Request.Context(), "auth_rate_limit_window")
	maxReqStr, err2 := h.configRepo.Get(c.Request.Context(), "auth_rate_limit_max_requests")

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

	if err := h.configRepo.Set(c.Request.Context(), "auth_rate_limit_window", strconv.Itoa(req.WindowSeconds)); err != nil {
		response.InternalError(c, 500, "保存设置失败", err)
		return
	}
	if err := h.configRepo.Set(c.Request.Context(), "auth_rate_limit_max_requests", strconv.Itoa(req.MaxRequests)); err != nil {
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

	result, err := h.userService.ListUsers(c.Request.Context(), &req)
	if err != nil {
		if isUserListValidationError(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "获取用户列表失败", err)
		return
	}

	response.Success(c, result)
}

func (h *AdminHandler) BanUser(c *gin.Context) {
	var req AdminBanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.ExpiresInDays < 0 {
		response.Error(c, 400, "expires_in_days 不能小于 0")
		return
	}

	targetUser, authUserID, currentStatus, ok := h.prepareManagedAuthUser(c)
	if !ok {
		return
	}
	if err := h.performBan(c.Request.Context(), targetUser, authUserID, currentStatus, req); err != nil {
		h.respondAdminActionError(c, err, "封禁")
		return
	}

	response.Success(c, nil)
}

func (h *AdminHandler) UnbanUser(c *gin.Context) {
	targetUser, authUserID, currentStatus, ok := h.prepareManagedAuthUser(c)
	if !ok {
		return
	}
	if err := h.performUnban(c.Request.Context(), targetUser, authUserID, currentStatus); err != nil {
		h.respondAdminActionError(c, err, "解封")
		return
	}

	response.Success(c, nil)
}

func (h *AdminHandler) RevokeUserSessions(c *gin.Context) {
	targetUser, authUserID, _, ok := h.prepareManagedAuthUser(c)
	if !ok {
		return
	}
	if targetUser.ID == 0 {
		response.Error(c, 400, "用户不存在")
		return
	}
	if err := h.authService.RevokeUserSessions(c.Request.Context(), authUserID); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			response.Error(c, 400, "目标用户在身份系统中不存在，无法吊销会话")
			return
		}
		response.InternalError(c, 502, "身份侧会话吊销失败", err)
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

	currentUserID := c.GetInt64("user_id")
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
		if err := h.userService.UpdateRole(c.Request.Context(), targetUser.ID, nextRole); err != nil {
			_ = h.authService.SetUserRole(c.Request.Context(), authUserID, currentRole)
			response.InternalError(c, 500, "业务侧角色更新失败", err)
			return
		}
		response.Success(c, nil)
		return
	}

	if err := h.userService.UpdateRole(c.Request.Context(), targetUser.ID, nextRole); err != nil {
		response.InternalError(c, 500, "业务侧角色更新失败", err)
		return
	}
	if err := h.authService.SetUserRole(c.Request.Context(), authUserID, nextRole); err != nil {
		slog.Warn("role drift detected after local role update", "user_id", targetUser.ID, "auth_user_id", authUserID, "target_role", nextRole, "err", err)
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

	currentUserID := c.GetInt64("user_id")
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

	if err := h.userService.DeleteUser(c.Request.Context(), targetUser.ID); err != nil {
		response.InternalError(c, 500, "业务侧数据清理失败，已保留 Auth 用户以便重试", err)
		return
	}

	if err := h.authService.DeleteUser(c.Request.Context(), authUserID); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			slog.Warn("auth user already missing after local admin delete", "user_id", targetUser.ID, "auth_user_id", authUserID)
			response.Success(c, nil)
			return
		}
		response.InternalError(c, 502, "业务侧已清理，但删除 Auth 用户失败，请按运维流程补偿处理", err)
		return
	}

	response.Success(c, nil)
}

func (h *AdminHandler) getTargetUserByParam(c *gin.Context) (*User, bool) {
	userID := c.Param("id")
	userIDUint, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的用户ID")
		return nil, false
	}

	user, err := h.userService.GetByID(c.Request.Context(), userIDUint)
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

func (h *AdminHandler) prepareManagedAuthUser(c *gin.Context) (*User, string, int16, bool) {
	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return nil, "", 0, false
	}
	currentUserID := c.GetInt64("user_id")
	if currentUserID != 0 && currentUserID == targetUser.ID {
		response.Error(c, 400, "不能操作当前登录管理员自己")
		return nil, "", 0, false
	}
	if normalizeAdminRole(targetUser.Role) == "admin" {
		response.Error(c, 400, "不能操作管理员账号")
		return nil, "", 0, false
	}

	authUserID := strings.TrimSpace(derefString(targetUser.AuthUserID))
	if authUserID == "" {
		response.Error(c, 400, "目标用户缺少 auth_user_id，已拒绝操作以避免状态漂移")
		return nil, "", 0, false
	}
	if h.authService == nil {
		response.InternalError(c, 500, "认证服务未配置", errors.New("admin auth service client not configured"))
		return nil, "", 0, false
	}

	return targetUser, authUserID, targetUser.Status, true
}

func (h *AdminHandler) performBan(ctx context.Context, targetUser *User, authUserID string, currentStatus int16, req AdminBanUserRequest) error {
	if currentStatus == 0 {
		return nil
	}

	reason := strings.TrimSpace(req.Reason)
	expiresInSeconds := 0
	if req.ExpiresInDays > 0 {
		expiresInSeconds = req.ExpiresInDays * 24 * 60 * 60
	}
	if err := h.authService.BanUser(ctx, authUserID, reason, expiresInSeconds); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return &adminActionError{status: 400, msg: "目标用户在身份系统中不存在，已拒绝封禁以避免状态漂移", err: err}
		}
		return &adminActionError{status: 502, msg: "身份侧封禁失败", err: err}
	}
	if err := h.userService.UpdateStatus(ctx, targetUser.ID, 0); err != nil {
		if rollbackErr := h.authService.UnbanUser(ctx, authUserID); rollbackErr != nil {
			slog.Warn("ban compensation failed", "user_id", targetUser.ID, "auth_user_id", authUserID, "rollback_err", rollbackErr)
			return &adminActionError{status: 500, msg: "业务侧封禁失败，且身份侧回滚失败，请按运维流程补偿处理", err: err}
		}
		return &adminActionError{status: 400, msg: err.Error(), err: err}
	}
	return nil
}

func (h *AdminHandler) performUnban(ctx context.Context, targetUser *User, authUserID string, currentStatus int16) error {
	if currentStatus == 1 {
		return nil
	}
	if err := h.authService.UnbanUser(ctx, authUserID); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return &adminActionError{status: 400, msg: "目标用户在身份系统中不存在，已拒绝解封以避免状态漂移", err: err}
		}
		return &adminActionError{status: 502, msg: "身份侧解封失败", err: err}
	}
	if err := h.userService.UpdateStatus(ctx, targetUser.ID, 1); err != nil {
		rollbackErr := h.authService.BanUser(ctx, authUserID, "rollback after local unban failure", 0)
		if rollbackErr != nil {
			slog.Warn("unban compensation failed", "user_id", targetUser.ID, "auth_user_id", authUserID, "rollback_err", rollbackErr)
			return &adminActionError{status: 500, msg: "业务侧解封失败，且身份侧回滚失败，请按运维流程补偿处理", err: err}
		}
		return &adminActionError{status: 400, msg: err.Error(), err: err}
	}
	return nil
}

func (h *AdminHandler) respondAdminActionError(c *gin.Context, err error, action string) {
	if err == nil {
		return
	}
	var actionErr *adminActionError
	if errors.As(err, &actionErr) {
		if actionErr.status >= 500 {
			response.InternalError(c, actionErr.status, actionErr.msg, actionErr.err)
			return
		}
		response.Error(c, actionErr.status, actionErr.msg)
		return
	}
	response.InternalError(c, 500, action+"失败", err)
}
