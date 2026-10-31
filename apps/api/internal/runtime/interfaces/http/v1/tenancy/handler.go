package tenancy

import (
	"context"
	"net/http"

	domainboards "ideaboard/api/internal/boards"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/gin-gonic/gin"
)

type MemberDeps struct {
	MemberDisplay shared.MemberDisplayProvider
	MemberLookup  V1MemberLookupProvider
}

type Deps struct {
	Store                     domaintenancy.Store
	Users                     runtimehttp.V1UserDirectory
	Idempotency               *contract.IdempotencyStore
	UserProfile               runtimehttp.V1UserProfileDirectory
	BillingAccounts           shared.AccountStore
	Members                   MemberDeps
	InvitationReader          invitations.Reader
	Invitations               *invitations.Service
	Boards                    domainboards.WorkspaceBoardStore
	DeliverBoardAccessSignals func(ctx context.Context, records []eventoutbox.BoardAccessOutboxRecord)
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	if deps.Idempotency == nil {
		deps.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	return &Handler{deps: deps}
}

func (h *Handler) MountPublic(group *gin.RouterGroup) {
	group.GET("/invites/:token", h.getWorkspaceInviteByToken)
}

func (h *Handler) Mount(group *gin.RouterGroup) {
	group.GET("/user", h.getUser)
	group.PATCH("/user", h.updateUser)
	group.GET("/workspaces", h.listWorkspaces)
	group.POST("/workspaces", h.createWorkspace)
	group.GET("/workspaces/:workspace_id", h.getWorkspace)
	group.PATCH("/workspaces/:workspace_id", h.updateWorkspace)
	group.DELETE("/workspaces/:workspace_id", h.deleteWorkspace)
	group.GET("/workspaces/:workspace_id/members", h.listWorkspaceMembers)
	group.POST("/workspaces/:workspace_id/members", h.grantOrganizationWorkspaceMember)
	group.GET("/workspaces/:workspace_id/members/lookup", h.lookupWorkspaceMemberByEmail)
	group.PATCH("/workspaces/:workspace_id/members/:member_id", h.updateWorkspaceMember)
	group.DELETE("/workspaces/:workspace_id/members/:member_id", h.removeWorkspaceMember)
	group.POST("/workspaces/:workspace_id/invites", h.createWorkspaceInvite)
	group.GET("/workspaces/:workspace_id/invites", h.listWorkspaceInvites)
	group.DELETE("/workspaces/:workspace_id/invites/:invite_id", h.cancelWorkspaceInvite)
	group.GET("/user/invites", h.listUserWorkspaceInvites)
	group.POST("/user/invites/:invite_id/accept", h.acceptUserWorkspaceInvite)
	group.POST("/user/invites/:invite_id/decline", h.declineUserWorkspaceInvite)
	group.POST("/invites/:token/accept", h.acceptWorkspaceInviteByToken)
	group.POST("/invites/:token/decline", h.declineWorkspaceInviteByToken)
	group.GET("/organizations", h.listOrganizations)
	group.POST("/organizations", h.createOrganization)
	group.GET("/organizations/:organization_id", h.getOrganization)
	group.PATCH("/organizations/:organization_id", h.updateOrganization)
	group.POST("/organizations/:organization_id/closure", h.closeOrganization)
	group.GET("/organizations/:organization_id/members", h.listOrganizationMembers)
	group.GET("/organizations/:organization_id/members/lookup", h.lookupOrganizationMemberByEmail)
	group.PATCH("/organizations/:organization_id/members/:member_id", h.updateOrganizationMember)
	group.DELETE("/organizations/:organization_id/members/:member_id", h.removeOrganizationMember)
	group.POST("/organizations/:organization_id/invites", h.createOrganizationInvite)
	group.GET("/organizations/:organization_id/invites", h.listOrganizationInvites)
	group.DELETE("/organizations/:organization_id/invites/:invite_id", h.cancelOrganizationInvite)
	group.GET("/user/organization-invites", h.listUserOrganizationInvites)
	group.POST("/user/organization-invites/:invite_id/accept", h.acceptUserOrganizationInvite)
	group.POST("/user/organization-invites/:invite_id/decline", h.declineUserOrganizationInvite)
	group.GET("/organizations/:organization_id/workspaces", h.listOrganizationWorkspaces)
	group.POST("/organizations/:organization_id/workspaces", h.createOrganizationWorkspace)
}

func (h *Handler) requireStore(c *gin.Context) bool {
	if h.deps.Store == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

func (h *Handler) withWorkspace(c *gin.Context, opts shared.WorkspaceOpts, fn func(shared.WorkspaceCtx)) {
	require := opts.Require
	if require == nil {
		require = h.requireStore
	}
	resolve := opts.Resolve
	if resolve == nil {
		resolve = func(c *gin.Context, p principal.Principal) (*domaintenancy.Workspace, string, bool) {
			return shared.ResolveWorkspaceContext(c, h.deps.Store, p)
		}
	}
	shared.WorkspacePrelude(c, require, resolve, opts.Allow, fn)
}

func (h *Handler) withOrganization(c *gin.Context, opts shared.OrganizationOpts, fn func(shared.OrganizationCtx)) {
	shared.OrganizationPrelude(c, h.requireStore, func(c *gin.Context, p principal.Principal) (*domaintenancy.Organization, string, bool) {
		return shared.ResolveOrganizationContext(c, h.deps.Store, p)
	}, opts.Allow, fn)
}
