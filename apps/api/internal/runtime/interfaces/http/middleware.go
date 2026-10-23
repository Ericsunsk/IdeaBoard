package http

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// Contract kernel HTTP seam (issue #800; full authZ, per-surface rate
// limits, and audit land in #808).
//
// Additive only: these groups and middleware never replaced the global
// middleware chain; the legacy /api/user, /api/admin, /api/health
// routes were removed in #809.

const (
	// ContractV1GroupPath is the Public Data Plane prefix (Baseline 2.1).
	ContractV1GroupPath = "/api/v1"
	// ContractControlGroupPath is the Control Plane prefix (Baseline 2.2).
	ContractControlGroupPath = "/api/control/v1"
)

// ContractRequestIDMiddleware bridges the Baseline 5.7 Request-Id header
// with the existing X-Request-ID correlation ID set by the global
// RequestIDMiddleware: a client-supplied Request-Id is adopted, otherwise
// the existing correlation ID is echoed back as Request-Id so v1 clients
// see one stable header either way.
func ContractRequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(contract.RequestIDHeader))
		if requestID == "" {
			requestID = c.GetString(response.RequestIDHeader)
		}
		if requestID != "" {
			c.Set(contract.RequestIDHeader, requestID)
			c.Header(contract.RequestIDHeader, requestID)
			// Keep the applog scope in sync for engines mounted without
			// the global RequestIDMiddleware (tests, contract-only
			// mounts). Same-id re-seeds keep existing enrichment.
			if c.Request != nil {
				c.Request = c.Request.WithContext(applog.ContextWithRequestID(c.Request.Context(), requestID))
			}
		}
		c.Next()
	}
}

// ContractRateLimitHeadersMiddleware is a passthrough placeholder: #808
// wires per-surface limits and renders RateLimit-Limit /
// RateLimit-Remaining / RateLimit-Reset (plus Retry-After on 429) via
// contract.SetRateLimit. It exists so both chains share one seam today.
func ContractRateLimitHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// ContractETagMiddleware is a passthrough placeholder: resource handlers
// in #802+ own ETag/If-Match validation via contract.SetETag and
// contract.IfMatch. It exists so both chains share one seam today.
func ContractETagMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// ResolvePrincipal enriches the authenticated principal with the explicit
// workspace context (issue #801). It runs AFTER the cookie-session
// AuthMiddleware (or a test principal injection): when a principal is
// already present it resolves TenantScope from the {workspace_id} path
// param and continues. Cookie-session requests keep working because the
// upstream AuthMiddleware establishes the principal from the Better Auth
// session first; this middleware never validates sessions itself.
//
// TenantScope resolution never fails the request: a missing param leaves
// the scope empty, and a malformed ws_ shape also leaves it empty (the
// lookup seam then converges to 404 per Baseline 4 anti-enumeration
// instead of leaking shape feedback with a distinct status).
func ResolvePrincipal() gin.HandlerFunc {
	return func(c *gin.Context) {
		identityPrincipal, ok := principal.From(c)
		if !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, MsgUnauthorized)
			c.Abort()
			return
		}
		if raw := strings.TrimSpace(c.Param("workspace_id")); raw != "" && contract.IsWorkspaceID(raw) {
			scoped, err := identityPrincipal.WithTenantScope(raw)
			if err == nil {
				// WithTenantScope copies; re-set so downstream handlers see
				// the scoped principal. A set failure fails closed to the
				// unscoped principal rather than 500: visibility checks
				// still converge to 404/403 below.
				if setErr := principal.Set(c, scoped); setErr == nil {
					identityPrincipal = scoped
				}
			}
		}
		// The principal is resolved here (this middleware runs AFTER the
		// cookie-session AuthMiddleware, and aborts 401 above when absent),
		// so enrich the request scope with user_id/workspace_id. Request-ID
		// seeding happened upstream (global + contract middleware), so this
		// only adds identity fields — never clears the scope.
		if c.Request != nil {
			c.Request = c.Request.WithContext(applog.ContextWithPrincipal(
				c.Request.Context(), identityPrincipal.UserID(), identityPrincipal.TenantScope()))
		}
		c.Next()
	}
}

// RequireCapability renders the visibility-aware authorization outcome
// behind every explicit scope gate (Resolve -> LoadRole -> Require):
// hidden resources converge to the scope's 404 so existence is not
// leaked, while visible-but-forbidden callers get the scope's 403.
// It returns true when allowed (caller continues) and aborts otherwise.
func RequireCapability(c *gin.Context, visible, allowed bool, notFoundCode contract.Code, forbiddenCode contract.Code, notFoundMsg, forbiddenMsg string) bool {
	switch {
	case !visible:
		contract.RenderError(c, http.StatusNotFound, notFoundCode, notFoundMsg)
		c.Abort()
		return false
	case !allowed:
		contract.RenderError(c, http.StatusForbidden, forbiddenCode, forbiddenMsg)
		c.Abort()
		return false
	default:
		return true
	}
}

// RequireWorkspaceCapability renders the visibility-aware authorization
// outcome for an explicit workspace context: hidden resources (the caller
// cannot see the workspace at all) converge to 404 workspace_not_found so
// existence is not leaked, while visible-but-forbidden callers get 403.
// It returns true when allowed (caller continues) and aborts otherwise.
func RequireWorkspaceCapability(c *gin.Context, visible, allowed bool) bool {
	return RequireCapability(c, visible, allowed,
		contract.CodeWorkspaceNotFound, contract.CodeWorkspaceForbidden,
		MsgListControlBillingAccounts, MsgForbidden)
}

// RequireOrganizationCapability is the organization-scoped counterpart:
// hidden orgs converge to 404 organization_not_found, visible-but-
// forbidden to 403.
func RequireOrganizationCapability(c *gin.Context, visible, allowed bool) bool {
	return RequireCapability(c, visible, allowed,
		contract.CodeOrganizationNotFound, contract.CodeOrganizationForbidden,
		MsgOrganizationNotFound, MsgForbidden)
}

// PublicContractChain is the Public Data Plane middleware chain
// placeholder. #808 adds principal resolution, capability checks, and
// per-surface rate limits here; the chain must never grant Control
// Plane capabilities.
func PublicContractChain() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		ContractRequestIDMiddleware(),
		ContractRateLimitHeadersMiddleware(),
		ContractETagMiddleware(),
	}
}

// Note (plane split): the v1 engine mount
// (RegisterContractV1RoutesOnEngine) lives in v1/routes.go, the
// billing engine mount in v1/billing.go, and the control
// engine mount in control/routes.go. The kernel
// RegisterRoutes mounts kernel routes only; composition calls all four.
// This keeps the kernel import-free of v1/control (no import cycle).
