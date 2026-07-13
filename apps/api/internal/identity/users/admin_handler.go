package users

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService     *UserService
	runtimeSettings *settings.RuntimeSettingsService
	authService     AuthUserManager
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

type updateOperationalSettingsRequest struct {
	TaskRuntime    settings.TaskRuntimeSetting    `json:"task_runtime" binding:"required"`
	OutboundPolicy settings.OutboundPolicySetting `json:"outbound_policy" binding:"required"`
	EventRetention settings.EventRetentionSetting `json:"event_retention" binding:"required"`
	Reason         string                         `json:"reason" binding:"required"`
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

func NewAdminHandler(userService *UserService, runtimeSettings *settings.RuntimeSettingsService, authService AuthUserManager) *AdminHandler {
	return &AdminHandler{
		userService:     userService,
		runtimeSettings: runtimeSettings,
		authService:     authService,
	}
}

// GetRegistrationSetting 获取注册开关状态
func (h *AdminHandler) GetRegistrationSetting(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetRegistration(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateRegistrationSetting 更新注册开关状态
func (h *AdminHandler) UpdateRegistrationSetting(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.RegistrationSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateRegistration(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}

	response.SuccessDTO(c, updated)
}

// GetRateLimitSettings 获取频率限制设置
func (h *AdminHandler) GetRateLimitSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetRateLimit(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateRateLimitSettings 更新频率限制设置
func (h *AdminHandler) UpdateRateLimitSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.RateLimitSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateRateLimit(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}

	response.SuccessDTO(c, updated)
}

// GetPublicURLSettings 获取公开链接设置
func (h *AdminHandler) GetPublicURLSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetPublicURL(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdatePublicURLSettings 更新公开链接设置
func (h *AdminHandler) UpdatePublicURLSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.PublicURLSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdatePublicURL(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

// GetMediaUploadSettings 获取媒体上传设置
func (h *AdminHandler) GetMediaUploadSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetMediaUpload(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateMediaUploadSettings 更新媒体上传设置
func (h *AdminHandler) UpdateMediaUploadSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.MediaUploadSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateMediaUpload(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

// GetGoogleNativeGenerationSettings 获取 Google AI 原生生成策略设置
func (h *AdminHandler) GetGoogleNativeGenerationSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetGoogleNativeGeneration(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateGoogleNativeGenerationSettings 更新 Google AI 原生生成策略设置
func (h *AdminHandler) UpdateGoogleNativeGenerationSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.GoogleNativeGenerationSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateGoogleNativeGeneration(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

// GetBrandingSettings 获取后台品牌设置
func (h *AdminHandler) GetBrandingSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	branding, err := runtimeSettings.GetBranding(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, branding)
}

// UpdateBrandingSettings 更新后台品牌设置
func (h *AdminHandler) UpdateBrandingSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.BrandingSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateBranding(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

// GetTaskMonitorSettings 获取任务监控设置
func (h *AdminHandler) GetTaskMonitorSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetTaskMonitor(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateTaskMonitorSettings 更新任务监控设置
func (h *AdminHandler) UpdateTaskMonitorSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req settings.TaskMonitorSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	updated, err := runtimeSettings.UpdateTaskMonitor(c.Request.Context(), req)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

// GetOperationalSettings returns persisted values alongside the snapshot used by this process.
func (h *AdminHandler) GetOperationalSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetOperationalSettings(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

// UpdateOperationalSettings atomically persists the typed groups and their admin audit record.
func (h *AdminHandler) UpdateOperationalSettings(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req updateOperationalSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误，reason 与所有 operational settings 分组均为必填")
		return
	}
	updated, err := runtimeSettings.UpdateOperationalSettings(c.Request.Context(), settings.OperationalSettings{
		TaskRuntime:    req.TaskRuntime,
		OutboundPolicy: req.OutboundPolicy,
		EventRetention: req.EventRetention,
	}, c.GetInt64("user_id"), req.Reason)
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
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
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "获取用户列表失败", err)
		return
	}

	response.Success(c, result)
}

func (h *AdminHandler) requireRuntimeSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if h.runtimeSettings == nil {
		response.InternalError(c, 500, "系统设置服务未初始化", errors.New("runtime settings service is not initialized"))
		return nil, false
	}
	return h.runtimeSettings, true
}

func (h *AdminHandler) respondRuntimeSettingsWriteError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if validation.Is(err) {
		response.Error(c, 400, err.Error())
		return true
	}
	response.InternalError(c, 500, "保存设置失败", err)
	return true
}

func (h *AdminHandler) respondRuntimeSettingsReadError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.InternalError(c, 500, "读取系统设置失败", err)
	return true
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

	if h.authService == nil {
		response.InternalError(c, 500, "认证服务未配置", errors.New("admin auth service client not configured"))
		return
	}

	if err := h.userService.DeleteUser(c.Request.Context(), targetUser.ID, h.authService); err != nil {
		response.InternalError(c, 500, "用户删除流程已安全冻结，可重试以继续收敛", err)
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
