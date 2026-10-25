package v1

// Organization member invitation confirmation endpoints (#957, ADR-0041).

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1CreateOrganizationInviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type v1OrganizationMemberInviteDTO struct {
	v1MemberInviteDTO
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name,omitempty"`
}

type v1OrganizationInviteAckDTO struct {
	Declined  bool `json:"declined,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
}

func renderOrganizationInviteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, tenancy.ErrOrganizationLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationLimitReached, runtimehttp.MsgOrganizationLimitReached)
	case errors.Is(err, tenancy.ErrOrganizationInviteConflict), errors.Is(err, tenancy.ErrConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "An active invitation already exists for this email")
	case errors.Is(err, tenancy.ErrOrganizationInviteExpired):
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

func (h *contractV1Handler) toOrganizationMemberInviteDTO(c *gin.Context, inv *tenancy.OrganizationMemberInvite, rawToken string) v1OrganizationMemberInviteDTO {
	if inv == nil {
		return v1OrganizationMemberInviteDTO{v1MemberInviteDTO: v1MemberInviteDTO{Token: rawToken}}
	}
	page := h.toOrganizationMemberInviteDTOs(c, []*tenancy.OrganizationMemberInvite{inv})
	if len(page) == 0 {
		return v1OrganizationMemberInviteDTO{v1MemberInviteDTO: v1MemberInviteDTO{Token: rawToken}}
	}
	dto := page[0]
	dto.Token = rawToken
	return dto
}

func (h *contractV1Handler) toOrganizationMemberInviteDTOs(c *gin.Context, invites []*tenancy.OrganizationMemberInvite) []v1OrganizationMemberInviteDTO {
	facts := make([]*tenancy.MemberInvite, 0, len(invites))
	for _, invite := range invites {
		facts = append(facts, tenancy.MemberInviteFromOrganization(invite))
	}
	views := h.deps.InvitationReader.Project(c.Request.Context(), facts)
	page := make([]v1OrganizationMemberInviteDTO, 0, len(views))
	for _, view := range views {
		page = append(page, v1OrganizationMemberInviteDTO{
			v1MemberInviteDTO: memberInviteDTO(view),
			OrganizationID:    view.ScopePublicID, OrganizationName: view.ScopeName,
		})
	}
	return page
}

// createOrganizationInvite creates a pending organization member invitation (POST /api/v1/organizations/:organization_id/invites).
func (h *contractV1Handler) createOrganizationInvite(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateOrganizationInviteBody
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
		if role == tenancy.RoleAdmin && octx.Role != tenancy.RoleOwner {
			contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, runtimehttp.MsgForbidden)
			c.Abort()
			return
		}

		type inviteResult struct {
			Invite *tenancy.OrganizationMemberInvite
			Token  string
		}

		userExternalID := tenancy.FormatUserPublicID(octx.Principal.PublicID())
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

// listOrganizationInvites lists all member invitations in an organization (GET /api/v1/organizations/:organization_id/invites).
func (h *contractV1Handler) listOrganizationInvites(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		invites, err := h.deps.Store.ListOrganizationMemberInvites(c.Request.Context(), octx.Organization.PublicID)
		if err != nil {
			renderOrganizationInviteError(c, err)
			return
		}
		page := h.toOrganizationMemberInviteDTOs(c, invites)
		contract.RenderSuccess(c, page)
	})
}

// cancelOrganizationInvite cancels a pending invitation (DELETE /api/v1/organizations/:organization_id/invites/:invite_id).
// DELETE cancellation is naturally idempotent (a repeated cancel converges to
// 409 conflict, never a duplicate side effect), so no Idempotency-Key is
// enforced here; see the cancelOrganizationMemberInvite operation description.
func (h *contractV1Handler) cancelOrganizationInvite(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !strings.HasPrefix(inviteID, tenancy.OrganizationInvitePrefix) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Invitation not found")
			c.Abort()
			return
		}
		err := h.deps.Invitations.CancelOrganizationMemberInvite(c.Request.Context(), octx.Organization.PublicID, inviteID, octx.Principal.UserID())
		if err != nil {
			renderOrganizationInviteError(c, err)
			return
		}
		contract.RenderSuccess(c, v1DeletedDTO{Deleted: true, ID: inviteID})
	})
}

// listUserOrganizationInvites lists pending organization invitations for current user (GET /api/v1/user/organization-invites).
func (h *contractV1Handler) listUserOrganizationInvites(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
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

// acceptUserOrganizationInvite accepts an in-app organization invitation (POST /api/v1/user/organization-invites/:invite_id/accept).
// See tenancy.OrganizationInviteStore for the authoritative post-decision contract.
func (h *contractV1Handler) acceptUserOrganizationInvite(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, tenancy.OrganizationInvitePrefix) {
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

// declineUserOrganizationInvite declines an in-app organization invitation (POST /api/v1/user/organization-invites/:invite_id/decline).
func (h *contractV1Handler) declineUserOrganizationInvite(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	inviteID := strings.TrimSpace(c.Param("invite_id"))
	if !strings.HasPrefix(inviteID, tenancy.OrganizationInvitePrefix) {
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
