package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminUserService interface {
	ListUsers(ctx context.Context, req *ports.UserListRequest) (*ports.UserListResponse, error)
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
}

type AdminHandler struct {
	userService      AdminUserService
	runtimeSettings  *settings.RuntimeSettingsService
	identityCommands ports.IdentityAdminCommands
	impersonator     ports.AdminUserImpersonator
}

type updateOutboundPolicyRequest struct {
	AllowHosts *[]string `json:"allow_hosts" binding:"required"`
}

func NewAdminHandler(userService AdminUserService, runtimeSettings *settings.RuntimeSettingsService, identityCommands ports.IdentityAdminCommands, impersonator ...ports.AdminUserImpersonator) *AdminHandler {
	handler := &AdminHandler{
		userService:      userService,
		runtimeSettings:  runtimeSettings,
		identityCommands: identityCommands,
	}
	if len(impersonator) > 0 {
		handler.impersonator = impersonator[0]
	}
	return handler
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

func (h *AdminHandler) GetOutboundPolicy(c *gin.Context) {
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetOutboundPolicy(c.Request.Context())
	if h.respondRuntimeSettingsReadError(c, err) {
		return
	}
	response.SuccessDTO(c, setting)
}

func (h *AdminHandler) UpdateOutboundPolicy(c *gin.Context) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	runtimeSettings, ok := h.requireRuntimeSettings(c)
	if !ok {
		return
	}
	var req updateOutboundPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误，allow_hosts 为必填")
		return
	}
	if req.AllowHosts == nil {
		response.Error(c, 400, "allow_hosts 不能为空")
		return
	}
	updated, err := runtimeSettings.UpdateOutboundPolicy(c.Request.Context(), settings.OutboundPolicySetting{
		AllowHosts: *req.AllowHosts,
	}, identityPrincipal.UserID())
	if h.respondRuntimeSettingsWriteError(c, err) {
		return
	}
	response.SuccessDTO(c, updated)
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req ports.UserListRequest
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

	response.SuccessDTO(c, result)
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
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

	if err := commands.SetRole(c.Request.Context(), actor, publicID, req.Role); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"updated": true})
}

func (h *AdminHandler) BanUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	var req ports.AdminBanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.ExpiresInDays < 0 {
		response.Error(c, 400, "expires_in_days 不能小于 0")
		return
	}

	if err := commands.Ban(c.Request.Context(), actor, publicID, ports.IdentityAdminBanInput{
		Reason:        strings.TrimSpace(req.Reason),
		ExpiresInDays: req.ExpiresInDays,
	}); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"banned": true})
}

func (h *AdminHandler) UnbanUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.Unban(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"unbanned": true})
}

func (h *AdminHandler) RevokeUserSessions(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.RevokeSessions(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"revoked": true})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.Delete(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"deleted": true})
}

func (h *AdminHandler) ImpersonateUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}
	if h == nil || h.userService == nil || h.impersonator == nil {
		response.InternalError(c, http.StatusInternalServerError, "伪装登录服务未配置", errors.New("admin user impersonation dependencies not configured"))
		return
	}

	target, err := h.userService.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if database.IsNotFound(err) {
			response.Error(c, 404, "用户不存在")
			return
		}
		response.InternalError(c, 500, "读取用户失败", err)
		return
	}
	if target == nil || target.ID <= 0 {
		response.Error(c, 404, "用户不存在")
		return
	}
	if actor.UserID() == target.ID {
		response.Error(c, 400, "不能模拟当前登录管理员自己")
		return
	}
	if strings.ToLower(strings.TrimSpace(target.Role)) == domain.RoleAdmin {
		response.Error(c, 400, "不能模拟管理员账号")
		return
	}
	authUserID := strings.TrimSpace(derefString(target.AuthUserID))
	if authUserID == "" {
		response.Error(c, 400, "目标用户缺少 auth_user_id，无法模拟登录")
		return
	}

	cookies, err := h.impersonator.ImpersonateUser(c.Request.Context(), c.GetHeader("Cookie"), authUserID)
	if err != nil {
		respondAdminUserImpersonationError(c, err)
		return
	}
	for _, cookie := range cookies {
		c.Writer.Header().Add("Set-Cookie", cookie)
	}
	response.Success(c, nil)
}

// getTargetUserPublicIDParam enforces the public identity route contract.
// Internal database IDs and Auth IDs must never be accepted by admin routes.
func getTargetUserPublicIDParam(c *gin.Context) (string, bool) {
	publicID, err := uuid.Parse(strings.TrimSpace(c.Param("public_id")))
	if err != nil {
		response.Error(c, 400, "无效的用户 public_id")
		return "", false
	}
	return publicID.String(), true
}

type impersonationStatusError interface {
	error
	StatusCode() int
}

func respondAdminUserImpersonationError(c *gin.Context, err error) {
	var statusErr impersonationStatusError
	if !errors.As(err, &statusErr) {
		response.InternalError(c, http.StatusBadGateway, "认证服务伪装登录失败", err)
		return
	}

	switch statusErr.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		response.Error(c, statusErr.StatusCode(), "认证服务拒绝伪装登录")
	case http.StatusBadRequest, http.StatusNotFound:
		response.Error(c, http.StatusBadRequest, "目标用户无法进行伪装登录")
	default:
		response.InternalError(c, http.StatusBadGateway, "认证服务伪装登录失败", err)
	}
}

func RespondIdentityAdminCommandError(c *gin.Context, err error) {
	var commandErr *ports.IdentityAdminCommandError
	if !errors.As(err, &commandErr) {
		response.InternalError(c, 500, "管理员操作失败", err)
		return
	}
	switch commandErr.Kind {
	case ports.IdentityAdminInvalid, ports.IdentityAdminTargetNotFound, ports.IdentityAdminAuthIdentityNotFound:
		response.Error(c, 400, commandErr.Message)
	case ports.IdentityAdminConflict:
		response.Error(c, 409, commandErr.Message)
	case ports.IdentityAdminForbidden:
		response.Error(c, 403, commandErr.Message)
	case ports.IdentityAdminAuthFailure:
		response.InternalError(c, 502, commandErr.Message, commandErr)
	case ports.IdentityAdminBusinessFailure, ports.IdentityAdminReconciliationRequired, ports.IdentityAdminUnavailable:
		response.InternalError(c, 500, commandErr.Message, commandErr)
	default:
		response.InternalError(c, 500, "管理员操作失败", commandErr)
	}
}

func (h *AdminHandler) requireRuntimeSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if h == nil || h.runtimeSettings == nil {
		response.Error(c, 503, "系统设置服务未配置")
		return nil, false
	}
	return h.runtimeSettings, true
}

func (h *AdminHandler) requireIdentityAdminCommands(c *gin.Context) (ports.IdentityAdminCommands, bool) {
	if h == nil || h.identityCommands == nil {
		response.InternalError(c, http.StatusInternalServerError, "身份管理命令未配置", errors.New("identity admin commands not configured"))
		return nil, false
	}
	return h.identityCommands, true
}

func (h *AdminHandler) respondRuntimeSettingsReadError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.InternalError(c, 500, "读取系统设置失败", err)
	return true
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
