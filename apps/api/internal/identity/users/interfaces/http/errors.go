package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

// Identity admin/user HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
const (
	codeIdentityRequestInvalid      = "identity_request_invalid"
	codeIdentityUserPublicIDInvalid = "identity_user_public_id_invalid"
	codeIdentityUserNotFound        = "identity_user_not_found"
	codeIdentityForbidden           = "identity_forbidden"
	codeIdentityConflict            = "identity_conflict"
	codeIdentityUpstreamRejected    = "identity_upstream_rejected"
	codeIdentityUpstreamUnavailable = "identity_upstream_unavailable"
	codeIdentityOperationFailed     = "identity_operation_failed"
	codeIdentityServiceUnavailable  = "identity_service_unavailable"
)

func writeIdentityRequestInvalid(c *gin.Context, message string) {
	if message == "" {
		message = "参数错误"
	}
	errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, message)
}

func identityValidationMessage(err error, fallback string) string {
	if validation.Is(err) && err.Error() != "" {
		return err.Error()
	}
	if fallback != "" {
		return fallback
	}
	return "参数错误"
}

// respondIdentityAdminCommandError renders typed admin command failures
// through the machine envelope. 4xx outcomes preserve the safe command
// message; 5xx outcomes log the cause and render the same safe message
// through the shared errorrender core so callers branch on stable codes.
func respondIdentityAdminCommandError(c *gin.Context, err error) {
	var commandErr *ports.IdentityAdminCommandError
	if err == nil {
		errorrender.Render(c, http.StatusInternalServerError, codeIdentityOperationFailed, "管理员操作失败")
		return
	}
	if !errors.As(err, &commandErr) {
		errorrender.RenderDefault(c, "identity admin command failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, "管理员操作失败", err)
		return
	}
	safeMessage := commandErr.Message
	if safeMessage == "" {
		safeMessage = "管理员操作失败"
	}
	switch commandErr.Kind {
	case ports.IdentityAdminInvalid, ports.IdentityAdminTargetNotFound, ports.IdentityAdminAuthIdentityNotFound:
		errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, safeMessage)
	case ports.IdentityAdminConflict:
		errorrender.Render(c, http.StatusConflict, codeIdentityConflict, safeMessage)
	case ports.IdentityAdminForbidden:
		errorrender.Render(c, http.StatusForbidden, codeIdentityForbidden, safeMessage)
	case ports.IdentityAdminAuthFailure:
		errorrender.RenderDefault(c, "identity admin Auth failure",
			http.StatusBadGateway, codeIdentityUpstreamUnavailable, safeMessage, err)
	case ports.IdentityAdminBusinessFailure, ports.IdentityAdminReconciliationRequired, ports.IdentityAdminUnavailable:
		errorrender.RenderDefault(c, "identity admin operation failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, safeMessage, err,
			"kind", string(commandErr.Kind))
	default:
		errorrender.RenderDefault(c, "identity admin operation failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, "管理员操作失败", err,
			"kind", string(commandErr.Kind))
	}
}
