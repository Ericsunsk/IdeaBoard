package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/errorrender"
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
	impersonationAuditSuccess = audit.OutcomeSuccess
	impersonationAuditDenied  = audit.OutcomeDenied
	impersonationAuditFailure = audit.OutcomeFailure
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
		ErrorMessage:   msgImpersonateUser,
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
		attempt.ErrorMessage = msgIdentityUserPublicIDInvalid
		attempt.Status = http.StatusBadRequest
		return
	}
	attempt.TargetID = publicID
	if h == nil || h.userService == nil || h.impersonator == nil {
		applog.FromContext(c.Request.Context()).Error("admin impersonation dependencies not configured")
		errorrender.Render(c, http.StatusInternalServerError, codeIdentityOperationFailed, msgIdentityOperationFailed2)
		attempt.ErrorMessage = msgIdentityOperationFailed2
		attempt.Status = http.StatusInternalServerError
		return
	}

	target, err := h.userService.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if database.IsNotFound(err) {
			errorrender.Render(c, http.StatusNotFound, codeIdentityUserNotFound, msgIdentityUserNotFound)
			attempt.ErrorMessage = msgIdentityUserNotFound
			attempt.Status = http.StatusNotFound
			return
		}
		response.InternalError(c, 500, msgImpersonateUser2, err)
		attempt.ErrorMessage = msgImpersonateUser2
		attempt.Status = http.StatusInternalServerError
		return
	}
	if target == nil || target.ID <= 0 {
		errorrender.Render(c, http.StatusNotFound, codeIdentityUserNotFound, msgIdentityUserNotFound)
		attempt.ErrorMessage = msgIdentityUserNotFound
		attempt.Status = http.StatusNotFound
		return
	}
	if actor.UserID() == target.ID {
		writeIdentityRequestInvalid(c, msgImpersonateUser3)
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = msgImpersonateUser3
		attempt.Status = http.StatusBadRequest
		return
	}
	if strings.ToLower(strings.TrimSpace(target.Role)) == domain.RoleAdmin {
		writeIdentityRequestInvalid(c, msgImpersonateUser4)
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = msgImpersonateUser4
		attempt.Status = http.StatusBadRequest
		return
	}
	authUserID := strings.TrimSpace(derefString(target.AuthUserID))
	if authUserID == "" {
		writeIdentityRequestInvalid(c, msgImpersonateUser5)
		attempt.Outcome = impersonationAuditDenied
		attempt.ErrorMessage = msgImpersonateUser5
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
		applog.FromContext(c.Request.Context()).Error("admin impersonation upstream failure", "err", err)
		errorrender.Render(c, http.StatusBadGateway, codeIdentityUpstreamUnavailable, msgIdentityUpstreamUnavailable)
		return http.StatusBadGateway, msgIdentityUpstreamUnavailable
	}

	switch statusErr.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		errorrender.Render(c, statusErr.StatusCode(), codeIdentityUpstreamRejected, msgIdentityUpstreamRejected)
		return statusErr.StatusCode(), msgIdentityUpstreamRejected
	case http.StatusBadRequest, http.StatusNotFound:
		errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, msgIdentityRequestInvalid)
		return http.StatusBadRequest, msgIdentityRequestInvalid
	default:
		applog.FromContext(c.Request.Context()).Error("admin impersonation upstream failure", "status", statusErr.StatusCode(), "err", err)
		errorrender.Render(c, http.StatusBadGateway, codeIdentityUpstreamUnavailable, msgIdentityUpstreamUnavailable)
		return http.StatusBadGateway, msgIdentityUpstreamUnavailable
	}
}

// recordImpersonationAttempt emits the structured slog trail for one
// impersonation attempt and persists it best-effort to the immutable
// admin-action store. Audit failures are logged and never fail the
// request; a nil receiver or unwired sink keeps the slog trail.
// request_id rides the applog scope from ctx, so it is not plumbed
// manually here.
func (h *AdminHandler) recordImpersonationAttempt(ctx context.Context, attempt audit.RecordParams) {
	applog.FromContext(ctx).InfoContext(ctx, "admin impersonation attempt",
		"actor_id", attempt.OperatorUserID,
		"target_public_id", attempt.TargetID,
		"auth_user_id", attempt.AuthUserID,
		"outcome", attempt.Outcome,
		"reason", attempt.ErrorMessage,
		"status", attempt.Status,
	)
	if h == nil || h.impersonationAuditor == nil {
		return
	}
	if err := h.impersonationAuditor.AppendRecord(ctx, attempt); err != nil {
		applog.FromContext(ctx).WarnContext(ctx, "admin impersonation audit write failed",
			"actor_id", attempt.OperatorUserID,
			"target_public_id", attempt.TargetID,
			"err", err,
		)
	}
}
