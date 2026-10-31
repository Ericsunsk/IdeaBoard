package tenancy

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1CreateWorkspaceInviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type WorkspaceMemberInviteDTO struct {
	MemberInviteDTO
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type v1WorkspaceMemberInviteDTO = WorkspaceMemberInviteDTO

type WorkspaceInvitePreviewDTO struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	InviterName   string `json:"inviter_name,omitempty"`
	InviterAvatar string `json:"inviter_avatar,omitempty"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	ExpiresAt     string `json:"expires_at"`
}

type v1WorkspaceInvitePreviewDTO = WorkspaceInvitePreviewDTO

type WorkspaceInviteAckDTO struct {
	Declined  bool `json:"declined,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
}

type v1WorkspaceInviteAckDTO = WorkspaceInviteAckDTO

func renderWorkspaceInviteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domaintenancy.ErrWorkspaceMemberLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceMemberLimitReached, "Workspace supports up to 5 members, including owners")
	case errors.Is(err, domaintenancy.ErrDefaultWorkspaceProtected):
		contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
			contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
	case errors.Is(err, domaintenancy.ErrWorkspaceInviteConflict), errors.Is(err, domaintenancy.ErrConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "An active invitation already exists for this email")
	case errors.Is(err, domaintenancy.ErrWorkspaceInviteExpired):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "The invitation has expired")
	case errors.Is(err, domaintenancy.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, domaintenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

func (h *Handler) toWorkspaceMemberInviteDTO(c *gin.Context, inv *domaintenancy.WorkspaceMemberInvite, rawToken string) v1WorkspaceMemberInviteDTO {
	if inv == nil {
		return v1WorkspaceMemberInviteDTO{MemberInviteDTO: MemberInviteDTO{Token: rawToken}}
	}
	page := h.toWorkspaceMemberInviteDTOs(c, []*domaintenancy.WorkspaceMemberInvite{inv})
	if len(page) == 0 {
		return v1WorkspaceMemberInviteDTO{MemberInviteDTO: MemberInviteDTO{Token: rawToken}}
	}
	dto := page[0]
	dto.Token = rawToken
	return dto
}

func (h *Handler) toWorkspaceMemberInviteDTOs(c *gin.Context, invites []*domaintenancy.WorkspaceMemberInvite) []v1WorkspaceMemberInviteDTO {
	facts := make([]*domaintenancy.MemberInvite, 0, len(invites))
	for _, invite := range invites {
		facts = append(facts, domaintenancy.MemberInviteFromWorkspace(invite))
	}
	views := h.deps.InvitationReader.Project(c.Request.Context(), facts)
	page := make([]v1WorkspaceMemberInviteDTO, 0, len(views))
	for _, view := range views {
		page = append(page, v1WorkspaceMemberInviteDTO{
			MemberInviteDTO: memberInviteDTO(view),
			WorkspaceID:     view.ScopePublicID, WorkspaceName: view.ScopeName,
		})
	}
	return page
}

func (h *Handler) createWorkspaceInvite(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Allow: shared.AllowOwnerOrAdmin}, func(wctx shared.WorkspaceCtx) {
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateWorkspaceInviteBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			shared.WriteInputError(c)
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" || !strings.Contains(email, "@") {
			shared.WriteInputError(c)
			return
		}
		role := strings.TrimSpace(body.Role)
		if role == "" {
			role = domaintenancy.RoleMember
		}
		if role != domaintenancy.RoleMember && role != domaintenancy.RoleAdmin {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Invalid member role")
			c.Abort()
			return
		}

		type inviteResult struct {
			Invite *domaintenancy.WorkspaceMemberInvite
			Token  string
		}

		userExternalID := domaintenancy.FormatUserPublicID(wctx.Principal.PublicID())
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

func (h *Handler) listWorkspaceInvites(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Allow: shared.AllowOwnerOrAdmin}, func(wctx shared.WorkspaceCtx) {
		invites, err := h.deps.Store.ListWorkspaceMemberInvites(c.Request.Context(), wctx.Workspace.PublicID)
		if err != nil {
			renderWorkspaceInviteError(c, err)
			return
		}
		page := h.toWorkspaceMemberInviteDTOs(c, invites)
		contract.RenderSuccess(c, page)
	})
}

func (h *Handler) cancelWorkspaceInvite(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Allow: shared.AllowOwnerOrAdmin}, func(wctx shared.WorkspaceCtx) {
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !strings.HasPrefix(inviteID, domaintenancy.WorkspaceInvitePrefix) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
			c.Abort()
			return
		}
		err := h.deps.Invitations.CancelWorkspaceMemberInvite(c.Request.Context(), wctx.Workspace.PublicID, inviteID, wctx.Principal.UserID())
		if err != nil {
			renderWorkspaceInviteError(c, err)
			return
		}
		contract.RenderSuccess(c, shared.DeletedDTO{Deleted: true, ID: inviteID})
	})
}

func (h *Handler) listUserWorkspaceInvites(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
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

func (h *Handler) GetWorkspaceInviteByToken(c *gin.Context) {
	h.getWorkspaceInviteByToken(c)
}

func (h *Handler) getWorkspaceInviteByToken(c *gin.Context) {
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
	view := h.deps.InvitationReader.Project(c.Request.Context(), []*domaintenancy.MemberInvite{domaintenancy.MemberInviteFromWorkspace(invite)})[0]
	preview := v1WorkspaceInvitePreviewDTO{
		WorkspaceID: view.ScopePublicID, WorkspaceName: view.ScopeName,
		InviterName: view.Inviter.Name, InviterAvatar: view.Inviter.Avatar,
		Role: view.Role, Status: view.Status, ExpiresAt: shared.FormatTime(view.ExpiresAt),
	}
	contract.RenderSuccess(c, preview)
}

func (h *Handler) acceptWorkspaceInviteByToken(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
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

func (h *Handler) declineWorkspaceInviteByToken(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
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

func (h *Handler) acceptUserWorkspaceInvite(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, domaintenancy.WorkspaceInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.acceptWorkspaceInvite(c, inviteID, principal.UserID())
}

func (h *Handler) declineUserWorkspaceInvite(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, domaintenancy.WorkspaceInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	h.declineWorkspaceInvite(c, inviteID, principal.UserID())
}

func (h *Handler) acceptWorkspaceInvite(c *gin.Context, ref string, userID int64) {
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

func (h *Handler) declineWorkspaceInvite(c *gin.Context, ref string, userID int64) {
	_, err := h.deps.Invitations.DeclineWorkspaceInvite(c.Request.Context(), ref, userID)
	if err != nil {
		renderWorkspaceInviteError(c, err)
		return
	}
	contract.RenderSuccess(c, v1WorkspaceInviteAckDTO{Declined: true})
}
