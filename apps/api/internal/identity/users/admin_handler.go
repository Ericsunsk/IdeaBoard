package users

import (
	"errors"
	"strconv"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService      *UserService
	runtimeSettings  *settings.RuntimeSettingsService
	identityCommands IdentityAdminCommands
}

type updateOperationalSettingsRequest struct {
	TaskRuntime    settings.TaskRuntimeSetting    `json:"task_runtime" binding:"required"`
	OutboundPolicy settings.OutboundPolicySetting `json:"outbound_policy" binding:"required"`
	EventRetention settings.EventRetentionSetting `json:"event_retention" binding:"required"`
	Reason         string                         `json:"reason" binding:"required"`
}

func NewAdminHandler(userService *UserService, runtimeSettings *settings.RuntimeSettingsService, identityCommands IdentityAdminCommands) *AdminHandler {
	return &AdminHandler{
		userService:      userService,
		runtimeSettings:  runtimeSettings,
		identityCommands: identityCommands,
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
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req updateOperationalSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误，reason 与所有 operational settings 分组均为必填")
		return
	}
	action, err := adminaction.New(identityPrincipal.UserID(), req.Reason)
	if err != nil {
		if errors.Is(err, adminaction.ErrInvalidActor) {
			response.Error(c, 400, "管理员身份无效")
		} else {
			response.Error(c, 400, "reason 必须在 1 到 1000 个字符之间")
		}
		return
	}
	updated, err := runtimeSettings.UpdateOperationalSettings(c.Request.Context(), settings.OperationalSettings{
		TaskRuntime:    req.TaskRuntime,
		OutboundPolicy: req.OutboundPolicy,
		EventRetention: req.EventRetention,
	}, action)
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
	identityPrincipal, authorized := principal.RequireAdmin(c)
	if !authorized {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	var req AdminBanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	targetUserID, ok := getTargetUserIDParam(c)
	if !ok {
		return
	}
	if err := commands.Ban(c.Request.Context(), identityPrincipal, targetUserID, IdentityAdminBanInput(req)); err != nil {
		respondIdentityAdminCommandError(c, err)
		return
	}

	response.Success(c, nil)
}

func (h *AdminHandler) UnbanUser(c *gin.Context) {
	identityPrincipal, authorized := principal.RequireAdmin(c)
	if !authorized {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	targetUserID, ok := getTargetUserIDParam(c)
	if !ok {
		return
	}
	if err := commands.Unban(c.Request.Context(), identityPrincipal, targetUserID); err != nil {
		respondIdentityAdminCommandError(c, err)
		return
	}

	response.Success(c, nil)
}

func (h *AdminHandler) RevokeUserSessions(c *gin.Context) {
	identityPrincipal, authorized := principal.RequireAdmin(c)
	if !authorized {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	targetUserID, ok := getTargetUserIDParam(c)
	if !ok {
		return
	}
	if err := commands.RevokeSessions(c.Request.Context(), identityPrincipal, targetUserID); err != nil {
		respondIdentityAdminCommandError(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	identityPrincipal, authorized := principal.RequireAdmin(c)
	if !authorized {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	targetUserID, ok := getTargetUserIDParam(c)
	if !ok {
		return
	}
	if err := commands.SetRole(c.Request.Context(), identityPrincipal, targetUserID, req.Role); err != nil {
		respondIdentityAdminCommandError(c, err)
		return
	}

	response.Success(c, nil)
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	identityPrincipal, authorized := principal.RequireAdmin(c)
	if !authorized {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	targetUserID, ok := getTargetUserIDParam(c)
	if !ok {
		return
	}
	if err := commands.Delete(c.Request.Context(), identityPrincipal, targetUserID); err != nil {
		respondIdentityAdminCommandError(c, err)
		return
	}

	response.Success(c, nil)
}

func getTargetUserIDParam(c *gin.Context) (int64, bool) {
	userID := c.Param("id")
	userIDUint, err := strconv.ParseInt(userID, 10, 64)
	if err != nil || userIDUint <= 0 {
		response.Error(c, 400, "无效的用户ID")
		return 0, false
	}
	return userIDUint, true
}

func (h *AdminHandler) requireIdentityAdminCommands(c *gin.Context) (IdentityAdminCommands, bool) {
	if h == nil || h.identityCommands == nil {
		response.InternalError(c, 500, "身份管理命令未配置", errors.New("identity admin commands not configured"))
		return nil, false
	}
	return h.identityCommands, true
}

func respondIdentityAdminCommandError(c *gin.Context, err error) {
	var commandErr *IdentityAdminCommandError
	if !errors.As(err, &commandErr) {
		response.InternalError(c, 500, "管理员操作失败", err)
		return
	}
	switch commandErr.Kind {
	case IdentityAdminInvalid, IdentityAdminTargetNotFound, IdentityAdminAuthIdentityNotFound:
		response.Error(c, 400, commandErr.Message)
	case IdentityAdminForbidden:
		response.Error(c, 403, commandErr.Message)
	case IdentityAdminAuthFailure:
		response.InternalError(c, 502, commandErr.Message, commandErr)
	case IdentityAdminBusinessFailure, IdentityAdminReconciliationRequired, IdentityAdminUnavailable:
		response.InternalError(c, 500, commandErr.Message, commandErr)
	default:
		response.InternalError(c, 500, "管理员操作失败", commandErr)
	}
}
