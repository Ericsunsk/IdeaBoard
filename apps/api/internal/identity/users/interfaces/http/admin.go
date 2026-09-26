package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/audit"
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
	userService          AdminUserService
	runtimeSettings      *settings.RuntimeSettingsService
	identityCommands     ports.IdentityAdminCommands
	impersonator         ports.AdminUserImpersonator
	impersonationAuditor ImpersonationAuditor
}

// ImpersonationAuditor persists one admin impersonation attempt
// best-effort. *audit.Store satisfies it directly against the immutable
// control_audit_records store; a nil auditor disables durable writes
// while the structured slog record always emits.
type ImpersonationAuditor interface {
	AppendRecord(ctx context.Context, params audit.RecordParams) error
}

// Impersonation audit facts for the legacy admin surface. The record
// shares the live Control Plane shape (users.impersonate under
// control:users) so both impersonation paths stay queryable in the one
// immutable store; metadata.route names the legacy origin.
const (
	impersonationAuditOperation  = "users.impersonate"
	impersonationAuditTargetType = "user"
	impersonationAuditRoute      = "POST /admin/users/:public_id/impersonate"
)

const (
	impersonationAuditSuccess = "success"
	impersonationAuditDenied  = "denied"
	impersonationAuditFailure = "failure"
)

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

// SetImpersonationAuditor attaches the durable impersonation-attempt sink.
// Nil clears it. Existing constructors are untouched; callers that never
// set an auditor keep the previous behavior plus the slog trail.
func (h *AdminHandler) SetImpersonationAuditor(auditor ImpersonationAuditor) *AdminHandler {
	if h != nil {
		h.impersonationAuditor = auditor
	}
	return h
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
		writeIdentityRequestInvalid(c, "参数错误")
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
		writeIdentityRequestInvalid(c, "参数错误")
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
		writeIdentityRequestInvalid(c, "参数错误")
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
		writeIdentityRequestInvalid(c, "参数错误")
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
		writeIdentityRequestInvalid(c, "参数错误，allow_hosts 为必填")
		return
	}
	if req.AllowHosts == nil {
		writeIdentityRequestInvalid(c, "allow_hosts 不能为空")
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
		writeIdentityRequestInvalid(c, "参数错误")
		return
	}

	result, err := h.userService.ListUsers(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			writeIdentityRequestInvalid(c, identityValidationMessage(err, "参数错误"))
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
		writeIdentityRequestInvalid(c, "参数错误")
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
		writeIdentityRequestInvalid(c, "参数错误")
		return
	}
	if req.ExpiresInDays < 0 {
		writeIdentityRequestInvalid(c, "expires_in_days 不能小于 0")
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
	// Every actor-attributed attempt below is audited (success, denied,
	// and failed) best-effort into the immutable admin-action store.
	// Guard allow/deny behavior is unchanged; audit writes never fail
	// the request.
	attempt := audit.RecordParams{
		OperatorUserID: actor.UserID(),
		Operation:      impersonationAuditOperation,
		Capability:     principal.ControlCapabilityUsers,
		TargetType:     impersonationAuditTargetType,
		TargetID:       strings.TrimSpace(c.Param("public_id")),
		Route:          impersonationAuditRoute,
		RequestID:      c.GetString(response.RequestIDHeader),
		Outcome:        impersonationAuditFailure,
		ErrorMessage:   "伪装登录失败",
		Status:         http.StatusInternalServerError,
	}
	if attempt.TargetID == "" {
		attempt.TargetID = "unknown"
	}
	defer func() {
		h.recordImpersonationAttempt(c.Request.Context(), attempt)
	}()
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		attempt.ErrorMessage = "无效的用户 public_id"
		attempt.Status = http.StatusBadRequest
		return
	}
	attempt.TargetID = publicID
	if h == nil || h.userService == nil || h.impersonator == nil {
		slog.Error("admin impersonation dependencies not configured")
		response.MachineError(c, http.StatusInternalServerError, codeIdentityOperationFailed, "伪装登录服务未配置")
		attempt.ErrorMessage = "伪装登录服务未配置"
		attempt.Status = http.StatusInternalServerError
		return
	}

	target, err := h.userService.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if database.IsNotFound(err) {
			response.MachineError(c, http.StatusNotFound, codeIdentityUserNotFound, "用户不存在")
			attempt.ErrorMessage = "用户不存在"
			attempt.Status = http.StatusNotFound
			return
		}
		response.InternalError(c, 500, "读取用户失败", err)
		attempt.ErrorMessage = "读取用户失败"
		attempt.Status = http.StatusInternalServerError
		return
	}
	if target == nil || target.ID <= 0 {
		response.MachineError(c, http.StatusNotFound, codeIdentityUserNotFound, "用户不存在")
		attempt.ErrorMessage = "用户不存在"
		attempt.Status = http.StatusNotFound
		return
	}
	if actor.UserID() == target.ID {
		writeIdentityRequestInvalid(c, "不能模拟当前登录管理员自己")
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = "不能模拟当前登录管理员自己"
		attempt.Status = http.StatusBadRequest
		return
	}
	if strings.ToLower(strings.TrimSpace(target.Role)) == domain.RoleAdmin {
		writeIdentityRequestInvalid(c, "不能模拟管理员账号")
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = "不能模拟管理员账号"
		attempt.Status = http.StatusBadRequest
		return
	}
	authUserID := strings.TrimSpace(derefString(target.AuthUserID))
	if authUserID == "" {
		writeIdentityRequestInvalid(c, "目标用户缺少 auth_user_id，无法模拟登录")
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = "目标用户缺少 auth_user_id，无法模拟登录"
		attempt.Status = http.StatusBadRequest
		return
	}
	attempt.AuthUserID = authUserID

	cookies, err := h.impersonator.ImpersonateUser(c.Request.Context(), c.GetHeader("Cookie"), authUserID)
	if err != nil {
		status, reason := respondAdminUserImpersonationError(c, err)
		attempt.ErrorMessage = reason
		attempt.Status = status
		return
	}
	for _, cookie := range cookies {
		c.Writer.Header().Add("Set-Cookie", cookie)
	}
	attempt.Outcome = impersonationAuditSuccess
	attempt.ErrorMessage = ""
	attempt.Status = http.StatusOK
	response.Success(c, nil)
}

// getTargetUserPublicIDParam enforces the public identity route contract.
// Internal database IDs and Auth IDs must never be accepted by admin routes.
func getTargetUserPublicIDParam(c *gin.Context) (string, bool) {
	publicID, err := uuid.Parse(strings.TrimSpace(c.Param("public_id")))
	if err != nil {
		response.MachineError(c, http.StatusBadRequest, codeIdentityUserPublicIDInvalid, "无效的用户 public_id")
		return "", false
	}
	return publicID.String(), true
}

type impersonationStatusError interface {
	error
	StatusCode() int
}

func respondAdminUserImpersonationError(c *gin.Context, err error) (int, string) {
	var statusErr impersonationStatusError
	if err == nil || !errors.As(err, &statusErr) {
		slog.Error("admin impersonation upstream failure", "err", err)
		response.MachineError(c, http.StatusBadGateway, codeIdentityUpstreamUnavailable, "认证服务伪装登录失败")
		return http.StatusBadGateway, "认证服务伪装登录失败"
	}

	switch statusErr.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		response.MachineError(c, statusErr.StatusCode(), codeIdentityUpstreamRejected, "认证服务拒绝伪装登录")
		return statusErr.StatusCode(), "认证服务拒绝伪装登录"
	case http.StatusBadRequest, http.StatusNotFound:
		response.MachineError(c, http.StatusBadRequest, codeIdentityRequestInvalid, "目标用户无法进行伪装登录")
		return http.StatusBadRequest, "目标用户无法进行伪装登录"
	default:
		slog.Error("admin impersonation upstream failure", "status", statusErr.StatusCode(), "err", err)
		response.MachineError(c, http.StatusBadGateway, codeIdentityUpstreamUnavailable, "认证服务伪装登录失败")
		return http.StatusBadGateway, "认证服务伪装登录失败"
	}
}

// recordImpersonationAttempt emits the structured slog trail for one
// impersonation attempt and persists it best-effort to the immutable
// admin-action store. Audit failures are logged and never fail the
// request; a nil receiver or unwired sink keeps the slog trail.
func (h *AdminHandler) recordImpersonationAttempt(ctx context.Context, attempt audit.RecordParams) {
	slog.InfoContext(ctx, "admin impersonation attempt",
		"actor_id", attempt.OperatorUserID,
		"target_public_id", attempt.TargetID,
		"auth_user_id", attempt.AuthUserID,
		"request_id", attempt.RequestID,
		"outcome", attempt.Outcome,
		"reason", attempt.ErrorMessage,
		"status", attempt.Status,
	)
	if h == nil || h.impersonationAuditor == nil {
		return
	}
	if err := h.impersonationAuditor.AppendRecord(ctx, attempt); err != nil {
		slog.WarnContext(ctx, "admin impersonation audit write failed",
			"actor_id", attempt.OperatorUserID,
			"target_public_id", attempt.TargetID,
			"request_id", attempt.RequestID,
			"err", err,
		)
	}
}

func RespondIdentityAdminCommandError(c *gin.Context, err error) {
	respondIdentityAdminCommandError(c, err)
}

func (h *AdminHandler) requireRuntimeSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if h == nil || h.runtimeSettings == nil {
		response.MachineError(c, http.StatusServiceUnavailable, codeIdentityServiceUnavailable, "系统设置服务未配置")
		return nil, false
	}
	return h.runtimeSettings, true
}

func (h *AdminHandler) requireIdentityAdminCommands(c *gin.Context) (ports.IdentityAdminCommands, bool) {
	if h == nil || h.identityCommands == nil {
		slog.Error("identity admin commands not configured")
		response.MachineError(c, http.StatusInternalServerError, codeIdentityOperationFailed, "身份管理命令未配置")
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
		writeIdentityRequestInvalid(c, identityValidationMessage(err, "参数错误"))
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
