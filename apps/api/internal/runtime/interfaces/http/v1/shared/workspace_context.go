package shared

import (
	"context"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// WorkspaceCtx carries resolved workspace facts and caller capabilities.
type WorkspaceCtx struct {
	Workspace *tenancy.Workspace
	Principal principal.Principal
	Role      string
}

// WorkspaceOpts configures workspace context resolution preconditions.
type WorkspaceOpts struct {
	Require func(c *gin.Context) bool
	Resolve func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool)
	Allow   func(role string, visible bool) bool
}

// AllowOwnerOrAdmin restricts the capability gate to workspace owner or admin.
func AllowOwnerOrAdmin(role string, visible bool) bool {
	return visible && (role == tenancy.RoleOwner || role == tenancy.RoleAdmin)
}

// RequestPrincipal extracts the authenticated principal from context.
func RequestPrincipal(c *gin.Context) (principal.Principal, bool) {
	identityPrincipal, ok := principal.From(c)
	if !ok {
		contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, runtimehttp.MsgUnauthorized)
		c.Abort()
		return principal.Principal{}, false
	}
	c.Request = c.Request.WithContext(notifications.ContextWithActor(c.Request.Context(), tenancy.FormatUserPublicID(identityPrincipal.PublicID())))
	return identityPrincipal, true
}

// WorkspacePrelude executes the standardized tenancy auth/membership prelude.
func WorkspacePrelude(
	c *gin.Context,
	require func(c *gin.Context) bool,
	resolve func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool),
	allow func(role string, visible bool) bool,
	fn func(WorkspaceCtx),
) {
	p, ok := RequestPrincipal(c)
	if !ok {
		return
	}
	if !require(c) {
		return
	}
	workspace, role, visible := resolve(c, p)
	if workspace == nil {
		return
	}
	allowed := visible
	if allow != nil {
		allowed = allow(role, visible)
	}
	if !runtimehttp.RequireWorkspaceCapability(c, visible, allowed) {
		return
	}
	fn(WorkspaceCtx{Principal: p, Workspace: workspace, Role: role})
}

// WriteInputError writes a consistent 400 Bad Request response for invalid JSON payloads.
func WriteInputError(c *gin.Context) {
	contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
		runtimehttp.MsgRequireEmptyV1BillingActionBody, contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
	c.Abort()
}

// WithWorkspaceBody runs the shared prelude plus lenient JSON binding for content writes.
func WithWorkspaceBody[T any](c *gin.Context, require func(*gin.Context) bool, resolve func(*gin.Context, principal.Principal) (*tenancy.Workspace, string, bool), allow func(string, bool) bool, fn func(WorkspaceCtx, T)) {
	WorkspacePrelude(c, require, resolve, allow, func(wctx WorkspaceCtx) {
		var body T
		if err := contract.BindLenientJSON(c, &body); err != nil {
			WriteInputError(c)
			return
		}
		fn(wctx, body)
	})
}

// WithWorkspaceBodyStrict runs the shared prelude plus strict JSON binding for permission and funds writes.
func WithWorkspaceBodyStrict[T any](c *gin.Context, require func(*gin.Context) bool, resolve func(*gin.Context, principal.Principal) (*tenancy.Workspace, string, bool), allow func(string, bool) bool, fn func(WorkspaceCtx, T)) {
	WorkspacePrelude(c, require, resolve, allow, func(wctx WorkspaceCtx) {
		var body T
		if err := contract.BindStrictJSON(c, &body); err != nil {
			WriteInputError(c)
			return
		}
		fn(wctx, body)
	})
}

// RequireIdempotencyKey enforces the shared header-only idempotency contract
// for required v1 mutations.
func RequireIdempotencyKey(c *gin.Context) (string, bool) {
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
		c.Abort()
		return "", false
	}
	return key, true
}

// ResolveWorkspaceContext resolves the explicit workspace context.
func ResolveWorkspaceContext(c *gin.Context, store tenancy.Store, identityPrincipal principal.Principal) (*tenancy.Workspace, string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	workspace, err := store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	role, member, err := store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	if !member {
		return workspace, "", false
	}
	return workspace, role, true
}

// ResolveOrganizationContext resolves organization-scoped context.
func ResolveOrganizationContext(c *gin.Context, store tenancy.Store, identityPrincipal principal.Principal) (*tenancy.Organization, string, bool) {
	raw := strings.TrimSpace(c.Param("organization_id"))
	if !contract.IsOrganizationID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
		c.Abort()
		return nil, "", false
	}
	organization, err := store.GetOrganization(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
		c.Abort()
		return nil, "", false
	}
	role, member, err := store.GetOrganizationRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
		c.Abort()
		return nil, "", false
	}
	if !member {
		return organization, "", false
	}
	return organization, role, true
}

// MemberDisplay is the workspace-visible display projection for one member.
type MemberDisplay struct {
	Email  string
	Avatar string
}

// MemberDisplayProvider resolves workspace-visible display facts.
type MemberDisplayProvider interface {
	GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]MemberDisplay, error)
}

// OrganizationCtx carries the resolved shared organization facts.
type OrganizationCtx struct {
	Principal    principal.Principal
	Organization *tenancy.Organization
	Role         string
}

// OrganizationOpts configures organization context resolution.
type OrganizationOpts struct {
	Allow func(role string, visible bool) bool
}

// OrganizationPrelude executes the standardized organization auth/membership prelude.
func OrganizationPrelude(
	c *gin.Context,
	require func(c *gin.Context) bool,
	resolve func(c *gin.Context, p principal.Principal) (*tenancy.Organization, string, bool),
	allow func(role string, visible bool) bool,
	fn func(OrganizationCtx),
) {
	p, ok := RequestPrincipal(c)
	if !ok {
		return
	}
	if !require(c) {
		return
	}
	organization, role, visible := resolve(c, p)
	if organization == nil {
		return
	}
	allowed := visible
	if allow != nil {
		allowed = allow(role, visible)
	}
	if !runtimehttp.RequireOrganizationCapability(c, visible, allowed) {
		return
	}
	fn(OrganizationCtx{Principal: p, Organization: organization, Role: role})
}
