package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

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
