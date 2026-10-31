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

type v1CreateOrganizationInviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type OrganizationMemberInviteDTO struct {
	MemberInviteDTO
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name,omitempty"`
}

type v1OrganizationMemberInviteDTO = OrganizationMemberInviteDTO

type OrganizationInviteAckDTO struct {
	Declined  bool `json:"declined,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
}

type v1OrganizationInviteAckDTO = OrganizationInviteAckDTO

func renderOrganizationInviteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domaintenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, domaintenancy.ErrOrganizationLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationLimitReached, runtimehttp.MsgOrganizationLimitReached)
	case errors.Is(err, domaintenancy.ErrOrganizationInviteConflict), errors.Is(err, domaintenancy.ErrConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "An active invitation already exists for this email")
	case errors.Is(err, domaintenancy.ErrOrganizationInviteExpired):
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

func (h *Handler) toOrganizationMemberInviteDTO(c *gin.Context, inv *domaintenancy.OrganizationMemberInvite, rawToken string) v1OrganizationMemberInviteDTO {
	if inv == nil {
		return v1OrganizationMemberInviteDTO{MemberInviteDTO: MemberInviteDTO{Token: rawToken}}
	}
	page := h.toOrganizationMemberInviteDTOs(c, []*domaintenancy.OrganizationMemberInvite{inv})
	if len(page) == 0 {
		return v1OrganizationMemberInviteDTO{MemberInviteDTO: MemberInviteDTO{Token: rawToken}}
	}
	dto := page[0]
	dto.Token = rawToken
	return dto
}

func (h *Handler) toOrganizationMemberInviteDTOs(c *gin.Context, invites []*domaintenancy.OrganizationMemberInvite) []v1OrganizationMemberInviteDTO {
	facts := make([]*domaintenancy.MemberInvite, 0, len(invites))
	for _, invite := range invites {
		facts = append(facts, domaintenancy.MemberInviteFromOrganization(invite))
	}
	views := h.deps.InvitationReader.Project(c.Request.Context(), facts)
	page := make([]v1OrganizationMemberInviteDTO, 0, len(views))
	for _, view := range views {
		page = append(page, v1OrganizationMemberInviteDTO{
			MemberInviteDTO: memberInviteDTO(view),
			OrganizationID:  view.ScopePublicID, OrganizationName: view.ScopeName,
		})
	}
	return page
}

func (h *Handler) createOrganizationInvite(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == domaintenancy.RoleOwner || role == domaintenancy.RoleAdmin
	}}, func(octx shared.OrganizationCtx) {
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateOrganizationInviteBody
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
		if role == domaintenancy.RoleAdmin && octx.Role != domaintenancy.RoleOwner {
			contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, runtimehttp.MsgForbidden)
			c.Abort()
			return
		}

		type inviteResult struct {
			Invite *domaintenancy.OrganizationMemberInvite
			Token  string
		}

		userExternalID := domaintenancy.FormatUserPublicID(octx.Principal.PublicID())
		scope := contract.IdempotencyScope(userExternalID, "organization:member-invite")
		hash := contract.HashIdempotencyRequest("organization:member-invite", octx.Organization.PublicID, email, role)

		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), scope, octx.Principal.UserID(), key, hash, func() (any, error) {
			inv, rawToken, createErr := h.deps.Invitations.CreateOrganizationMemberInvite(
				c.Request.Context(), octx.Organization.PublicID, octx.Principal.UserID(), email, role)
			if createErr != nil {
				return nil, createErr
			}
			return inviteResult{Invite: inv, Token: rawToken}, nil
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			renderOrganizationInviteError(c, doErr)
			return
		}
		res, ok := result.(inviteResult)
		if !ok || res.Invite == nil {
			renderOrganizationInviteError(c, errors.New("tenancy: invalid invite result"))
			return
		}
		dto := h.toOrganizationMemberInviteDTO(c, res.Invite, res.Token)
		contract.RenderCreated(c, dto)
	})
}

func (h *Handler) listOrganizationInvites(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == domaintenancy.RoleOwner || role == domaintenancy.RoleAdmin
	}}, func(octx shared.OrganizationCtx) {
		invites, err := h.deps.Store.ListOrganizationMemberInvites(c.Request.Context(), octx.Organization.PublicID)
		if err != nil {
			renderOrganizationInviteError(c, err)
			return
		}
		page := h.toOrganizationMemberInviteDTOs(c, invites)
		contract.RenderSuccess(c, page)
	})
}

func (h *Handler) cancelOrganizationInvite(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == domaintenancy.RoleOwner || role == domaintenancy.RoleAdmin
	}}, func(octx shared.OrganizationCtx) {
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !strings.HasPrefix(inviteID, domaintenancy.OrganizationInvitePrefix) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
			c.Abort()
			return
		}
		err := h.deps.Invitations.CancelOrganizationMemberInvite(c.Request.Context(), octx.Organization.PublicID, inviteID, octx.Principal.UserID())
		if err != nil {
			renderOrganizationInviteError(c, err)
			return
		}
		contract.RenderSuccess(c, shared.DeletedDTO{Deleted: true, ID: inviteID})
	})
}

func (h *Handler) listUserOrganizationInvites(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	invites, err := h.deps.Invitations.ListPendingOrganizationInvitesForUser(c.Request.Context(), principal.UserID())
	if err != nil {
		renderOrganizationInviteError(c, err)
		return
	}
	page := h.toOrganizationMemberInviteDTOs(c, invites)
	contract.RenderSuccess(c, page)
}

func (h *Handler) acceptUserOrganizationInvite(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, domaintenancy.OrganizationInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	member, _, err := h.deps.Invitations.AcceptOrganizationInvite(c.Request.Context(), inviteID, principal.UserID())
	if err != nil {
		renderOrganizationInviteError(c, err)
		return
	}
	dto, ok := h.toOrganizationMemberDTO(c, member)
	if !ok {
		return
	}
	contract.RenderSuccess(c, dto)
}

func (h *Handler) declineUserOrganizationInvite(c *gin.Context) {
	principal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, domaintenancy.OrganizationInvitePrefix) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
		c.Abort()
		return
	}
	_, err := h.deps.Invitations.DeclineOrganizationInvite(c.Request.Context(), inviteID, principal.UserID())
	if err != nil {
		renderOrganizationInviteError(c, err)
		return
	}
	contract.RenderSuccess(c, v1OrganizationInviteAckDTO{Declined: true})
}
