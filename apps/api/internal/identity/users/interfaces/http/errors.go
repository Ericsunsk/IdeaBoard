package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// Identity admin/user HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
// Identity admin/user HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
//
// The values below alias the kernel registry
// (platform/contract.CodeIdentity*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeIdentityRequestInvalid      = string(contract.CodeIdentityRequestInvalid)
	codeIdentityUnauthorized        = string(contract.CodeIdentityUnauthorized)
	codeIdentityUserPublicIDInvalid = string(contract.CodeIdentityUserPublicIDInvalid)
	codeIdentityUserNotFound        = string(contract.CodeIdentityUserNotFound)
	codeIdentityForbidden           = string(contract.CodeIdentityForbidden)
	codeIdentityConflict            = string(contract.CodeIdentityConflict)
	codeIdentityUpstreamRejected    = string(contract.CodeIdentityUpstreamRejected)
	codeIdentityUpstreamUnavailable = string(contract.CodeIdentityUpstreamUnavailable)
	codeIdentityOperationFailed     = string(contract.CodeIdentityOperationFailed)
	codeIdentityServiceUnavailable  = string(contract.CodeIdentityServiceUnavailable)
)

func writeIdentityRequestInvalid(c *gin.Context, message string) {
	if message == "" {
		message = errorrender.MsgRespondRuntimeSettingsWriteError
	}
	errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, message)
}

func identityValidationMessage(err error, fallback string) string {
	if errorrender.IsValidationError(err) && err.Error() != "" {
		return err.Error()
	}
	if fallback != "" {
		return fallback
	}
	return errorrender.MsgRespondRuntimeSettingsWriteError
}

// respondIdentityAdminCommandError renders typed admin command failures
// through the machine envelope. 4xx outcomes preserve the safe command
// message; 5xx outcomes log the cause and render the same safe message
// through the shared errorrender core so callers branch on stable codes.
func respondIdentityAdminCommandError(c *gin.Context, err error) {
	var commandErr *ports.IdentityAdminCommandError
	if err == nil {
		errorrender.Render(c, http.StatusInternalServerError, codeIdentityOperationFailed, errorrender.MsgIdentityOperationFailed3)
		return
	}
	if !errors.As(err, &commandErr) {
		errorrender.RenderDefault(c, "identity admin command failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, errorrender.MsgIdentityOperationFailed3, err)
		return
	}
	safeMessage := commandErr.Message
	if safeMessage == "" {
		safeMessage = errorrender.MsgIdentityOperationFailed3
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
			http.StatusInternalServerError, codeIdentityOperationFailed, errorrender.MsgIdentityOperationFailed3, err,
			"kind", string(commandErr.Kind))
	}
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.

// codeIdentityServiceUnavailable
// codeIdentityOperationFailed
// RespondRuntimeSettingsReadError @ admin.go
// RespondRuntimeSettingsWriteError @ admin.go
// RespondRuntimeSettingsWriteError @ admin.go
// codeIdentityUserPublicIDInvalid
// ImpersonateUser @ administration_impersonate.go
// codeIdentityOperationFailed
// codeIdentityUserNotFound
// ImpersonateUser @ administration_impersonate.go
// ImpersonateUser @ administration_impersonate.go
// ImpersonateUser @ administration_impersonate.go
// ImpersonateUser @ administration_impersonate.go
// codeIdentityUpstreamUnavailable
// codeIdentityUpstreamRejected
// codeIdentityRequestInvalid
// UpdateOutboundPolicy @ administration_settings.go
// UpdateOutboundPolicy @ administration_settings.go
// ListUsers @ administration_users.go
// BanUser @ administration_users.go
// codeIdentityOperationFailed
// UpdateLoginMetadata @ internal.go
