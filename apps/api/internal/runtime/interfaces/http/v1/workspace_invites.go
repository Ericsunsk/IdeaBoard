package v1

// Workspace member invitation confirmation endpoints (#957, ADR-0039).

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1CreateWorkspaceInviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type v1WorkspaceMemberInviteDTO struct {
	v1MemberInviteDTO
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type v1WorkspaceInvitePreviewDTO struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	InviterName   string `json:"inviter_name,omitempty"`
	InviterAvatar string `json:"inviter_avatar,omitempty"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	ExpiresAt     string `json:"expires_at"`
	// InviteeEmail is intentionally omitted from the public preview: the raw
	// token is unguessable but the preview is unauthenticated, so the invitee
	// address never leaves the authenticated invite DTOs.
}

type v1WorkspaceInviteAckDTO struct {
	Declined  bool `json:"declined,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
}

func renderWorkspaceInviteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceMemberLimitReached, "Workspace supports up to 5 members, including owners")
	case errors.Is(err, tenancy.ErrDefaultWorkspaceProtected):
		contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
			contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
	case errors.Is(err, tenancy.ErrWorkspaceInviteConflict), errors.Is(err, tenancy.ErrConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "An active invitation already exists for this email")
	case errors.Is(err, tenancy.ErrWorkspaceInviteExpired):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "The invitation has expired")
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, tenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

func (h *contractV1Handler) toWorkspaceMemberInviteDTO(c *gin.Context, inv *tenancy.WorkspaceMemberInvite, rawToken string) v1WorkspaceMemberInviteDTO {
	if inv == nil {
		return v1WorkspaceMemberInviteDTO{v1MemberInviteDTO: v1MemberInviteDTO{Token: rawToken}}
	}
	page := h.toWorkspaceMemberInviteDTOs(c, []*tenancy.WorkspaceMemberInvite{inv})
	if len(page) == 0 {
		return v1WorkspaceMemberInviteDTO{v1MemberInviteDTO: v1MemberInviteDTO{Token: rawToken}}
	}
	dto := page[0]
	dto.Token = rawToken
	return dto
}

func (h *contractV1Handler) toWorkspaceMemberInviteDTOs(c *gin.Context, invites []*tenancy.WorkspaceMemberInvite) []v1WorkspaceMemberInviteDTO {
	facts := make([]*tenancy.MemberInvite, 0, len(invites))
	for _, invite := range invites {
		facts = append(facts, tenancy.MemberInviteFromWorkspace(invite))
	}
	views := h.deps.InvitationReader.Project(c.Request.Context(), facts)
	page := make([]v1WorkspaceMemberInviteDTO, 0, len(views))
	for _, view := range views {
		page = append(page, v1WorkspaceMemberInviteDTO{
			v1MemberInviteDTO: memberInviteDTO(view),
			WorkspaceID:       view.ScopePublicID, WorkspaceName: view.ScopeName,
		})
	}
	return page
}

// createWorkspaceInvite creates a pending workspace member invitation (POST /api/v1/workspaces/:workspace_id/invites).
func (h *contractV1Handler) createWorkspaceInvite(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateWorkspaceInviteBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" || !strings.Contains(email, "@") {
			writeV1InputError(c)
			return
		}
		role := strings.TrimSpace(body.Role)
		if role == "" {
			role = tenancy.RoleMember
		}
		if role != tenancy.RoleMember && role != tenancy.RoleAdmin {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Invalid member role")
			c.Abort()
			return
		}

		type inviteResult struct {
			Invite *tenancy.WorkspaceMemberInvite
			Token  string
		}

		userExternalID := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("workspace:member-invite", wctx.Workspace.PublicID, email, role)

		result, doErr, handled := h.workspaceDo(c, userExternalID, wctx.Principal.UserID(), "member-invite", key, hash, func() (any, error) {
			inv, rawToken, createErr := h.deps.Invitations.CreateWorkspaceMemberInvite(
				c.Request.Context(), wctx.Workspace.PublicID, wctx.Principal.UserID(), email, role)
			if createErr != nil {
				return nil, createErr
			}
			return inviteResult{Invite: inv, Token: rawToken}, nil
		})
		if !handled {
			return
		}
		if doErr != nil {
			renderWorkspaceInviteError(c, doErr)
			return
		}
		res, ok := result.(inviteResult)
		if !ok || res.Invite == nil {
			renderWorkspaceInviteError(c, errors.New("tenancy: invalid invite result"))
			return
		}
		dto := h.toWorkspaceMemberInviteDTO(c, res.Invite, res.Token)
		contract.RenderCreated(c, dto)
	})
}

// listWorkspaceInvites lists all member invitations in a workspace (GET /api/v1/workspaces/:workspace_id/invites).
func (h *contractV1Handler) listWorkspaceInvites(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		invites, err := h.deps.Store.ListWorkspaceMemberInvites(c.Request.Context(), wctx.Workspace.PublicID)
		if err != nil {
			renderWorkspaceInviteError(c, err)
			return
		}
		page := h.toWorkspaceMemberInviteDTOs(c, invites)
		contract.RenderSuccess(c, page)
	})
}

// cancelWorkspaceInvite cancels a pending workspace invitation (DELETE /api/v1/workspaces/:workspace_id/invites/:invite_id).
// DELETE cancellation is naturally idempotent (a repeated cancel converges to
// 409 conflict, never a duplicate side effect), so no Idempotency-Key is
// enforced here; see the cancelWorkspaceMemberInvite operation description.
func (h *contractV1Handler) cancelWorkspaceInvite(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !strings.HasPrefix(inviteID, tenancy.WorkspaceInvitePrefix) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
			c.Abort()
			return
		}
		err := h.deps.Invitations.CancelWorkspaceMemberInvite(c.Request.Context(), wctx.Workspace.PublicID, inviteID, wctx.Principal.UserID())
		if err != nil {
			renderWorkspaceInviteError(c, err)
			return
		}
		contract.RenderSuccess(c, v1DeletedDTO{Deleted: true, ID: inviteID})
	})
}

// listUserWorkspaceInvites lists pending invitations for the authenticated user (GET /api/v1/user/invites).
func (h *contractV1Handler) listUserWorkspaceInvites(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	invites, err := h.deps.Invitations.ListPendingWorkspaceInvitesForUser(c.Request.Context(), principal.UserID())
	if err != nil {
		renderWorkspaceInviteError(c, err)
		return
	}
	page := h.toWorkspaceMemberInviteDTOs(c, invites)
	contract.RenderSuccess(c, page)
}

// getWorkspaceInviteByToken previews an invitation by raw token (GET /api/v1/invites/:token).
func (h *contractV1Handler) getWorkspaceInviteByToken(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	invite, err := h.deps.Store.GetWorkspaceInviteByToken(c.Request.Context(), token)
	if err != nil {
		renderWorkspaceInviteError(c, err)
		return
	}
	view := h.deps.InvitationReader.Project(c.Request.Context(), []*tenancy.MemberInvite{tenancy.MemberInviteFromWorkspace(invite)})[0]
	preview := v1WorkspaceInvitePreviewDTO{
		WorkspaceID: view.ScopePublicID, WorkspaceName: view.ScopeName,
		InviterName: view.Inviter.Name, InviterAvatar: view.Inviter.Avatar,
		Role: view.Role, Status: view.Status, ExpiresAt: v1FormatTime(view.ExpiresAt),
	}
	contract.RenderSuccess(c, preview)
}

// acceptWorkspaceInviteByToken accepts an invitation by raw token (POST /api/v1/invites/:token/accept).
// See tenancy.WorkspaceInviteStore for the authoritative post-decision contract.
func (h *contractV1Handler) acceptWorkspaceInviteByToken(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.acceptWorkspaceInvite(c, token, principal.UserID())
}

// declineWorkspaceInviteByToken declines an invitation by raw token (POST /api/v1/invites/:token/decline).
func (h *contractV1Handler) declineWorkspaceInviteByToken(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.declineWorkspaceInvite(c, token, principal.UserID())
}

// acceptUserWorkspaceInvite accepts an in-app invitation by invite ID (POST /api/v1/user/invites/:invite_id/accept).
func (h *contractV1Handler) acceptUserWorkspaceInvite(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, tenancy.WorkspaceInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.acceptWorkspaceInvite(c, inviteID, principal.UserID())
}

// declineUserWorkspaceInvite declines an in-app invitation by invite ID (POST /api/v1/user/invites/:invite_id/decline).
func (h *contractV1Handler) declineUserWorkspaceInvite(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, tenancy.WorkspaceInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.declineWorkspaceInvite(c, inviteID, principal.UserID())
}

func (h *contractV1Handler) acceptWorkspaceInvite(c *gin.Context, ref string, userID int64) {
	member, _, err := h.deps.Invitations.AcceptWorkspaceInvite(c.Request.Context(), ref, userID)
	if err != nil {
		renderWorkspaceInviteError(c, err)
		return
	}
	dto, ok := h.toMemberDTO(c, member)
	if !ok {
		return
	}
	contract.RenderSuccess(c, dto)
}

func (h *contractV1Handler) declineWorkspaceInvite(c *gin.Context, ref string, userID int64) {
	_, err := h.deps.Invitations.DeclineWorkspaceInvite(c.Request.Context(), ref, userID)
	if err != nil {
		renderWorkspaceInviteError(c, err)
		return
	}
	contract.RenderSuccess(c, v1WorkspaceInviteAckDTO{Declined: true})
}
