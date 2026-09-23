package httpapi

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// Contract kernel HTTP seam (issue #800; full authZ, per-surface rate
// limits, and audit land in #808).
//
// Additive only: these groups and middleware never replace the global
// middleware chain or the legacy /api/user, /api/admin, /api/health
// routes (removal is #809).

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
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, "未登录")
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
				_ = principal.Set(c, scoped)
			}
		}
		c.Next()
	}
}

// RequireWorkspaceCapability renders the visibility-aware authorization
// outcome for an explicit workspace context: hidden resources (the caller
// cannot see the workspace at all) converge to 404 workspace_not_found so
// existence is not leaked, while visible-but-forbidden callers get 403.
// It returns true when allowed (caller continues) and aborts otherwise.
func RequireWorkspaceCapability(c *gin.Context, visible, allowed bool) bool {
	switch {
	case !visible:
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
		c.Abort()
		return false
	case !allowed:
		contract.RenderError(c, http.StatusForbidden, contract.CodeWorkspaceForbidden, "权限不足")
		c.Abort()
		return false
	default:
		return true
	}
}

// RequireOrganizationCapability is the organization-scoped counterpart:
// hidden orgs converge to 404 organization_not_found, visible-but-
// forbidden to 403.
func RequireOrganizationCapability(c *gin.Context, visible, allowed bool) bool {
	switch {
	case !visible:
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
		c.Abort()
		return false
	case !allowed:
		contract.RenderError(c, http.StatusForbidden, contract.CodeOrganizationForbidden, "权限不足")
		c.Abort()
		return false
	default:
		return true
	}
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

// ControlContractChain is the separate Control Plane middleware chain
// (issue #808): Request-Id bridging plus the dedicated control
// rate-limit bucket. Capability auth (operator grant), tenant scope,
// per-route audit, and header-only idempotency are wired by
// RegisterContractControlRoutes so tests can inject principals and
// sinks per engine.
func ControlContractChain(limiter *ControlRateLimiter) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		ContractRequestIDMiddleware(),
		ControlRateLimitMiddleware(limiter),
		ContractETagMiddleware(),
	}
}

// registerContractV1RoutesOnEngine mounts the additive v1 group: real
// handlers for the workspace/organization/member lifecycle (issue #801),
// boards + collaboration (#802), uploads + media (#803) with the shared
// PublicContractChain. Workspace billing (#804) mounts on its own group
// via RegisterContractV1BillingRoutesOnEngine, and the Control Plane
// (#808) via RegisterContractControlRoutesOnEngine. Legacy routes are
// untouched.
func registerContractV1RoutesOnEngine(r *gin.Engine, authDeps AuthDeps, deps ContractV1Deps) {
	v1 := r.Group(ContractV1GroupPath)
	v1.Use(PublicContractChain()...)
	RegisterContractV1Routes(v1, authDeps, deps)
}

func contractNotImplemented(c *gin.Context) {
	contract.RenderError(c, http.StatusNotImplemented,
		contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
}
