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
	"github.com/google/uuid"
)

type OrganizationMemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	Email     string `json:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
}

type v1OrganizationMemberDTO = OrganizationMemberDTO

func (h *Handler) toOrganizationMemberDTO(c *gin.Context, m *domaintenancy.OrganizationMember) (v1OrganizationMemberDTO, bool) {
	if m == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
		c.Abort()
		return v1OrganizationMemberDTO{}, false
	}
	page, ok := h.toOrganizationMemberDTOs(c, []*domaintenancy.OrganizationMember{m})
	if !ok || len(page) == 0 {
		return v1OrganizationMemberDTO{}, false
	}
	return page[0], true
}

func renderOrganizationMemberError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domaintenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, domaintenancy.ErrOrganizationLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationLimitReached, runtimehttp.MsgOrganizationLimitReached)
	case errors.Is(err, domaintenancy.ErrOrganizationOwnerImmutable):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, domaintenancy.ErrLastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden2)
	case errors.Is(err, domaintenancy.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, domaintenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationMemberNotFound, runtimehttp.MsgOrganizationMemberNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
	}
	c.Abort()
}

func (h *Handler) listOrganizationMembers(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{}, func(octx shared.OrganizationCtx) {
		members, err := h.deps.Store.ListOrganizationMembers(c.Request.Context(), octx.Organization.PublicID)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		page, ok := h.toOrganizationMemberDTOs(c, members)
		if !ok {
			return
		}
		contract.RenderSuccess(c, page)
	})
}

func (h *Handler) lookupOrganizationMemberByEmail(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == domaintenancy.RoleOwner || role == domaintenancy.RoleAdmin
	}}, func(octx shared.OrganizationCtx) {
		normalized := NormalizeMemberLookupEmail(c.Query("email"))
		if !IsValidMemberLookupEmail(normalized) {
			contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
				runtimehttp.MsgBadRequest35, contract.FieldErrors{Fields: map[string]string{"email": "invalid"}})
			c.Abort()
			return
		}
		if h.deps.Members.MemberLookup == nil {
			contract.RenderError(c, http.StatusNotImplemented,
				contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
			c.Abort()
			return
		}
		publicID, err := h.deps.Members.MemberLookup.LookupUserPublicID(c.Request.Context(), normalized)
		if err == nil && publicID != uuid.Nil {
			contract.RenderSuccess(c, v1MemberLookupDTO{UserID: domaintenancy.FormatUserPublicID(publicID)})
			return
		}
		if err != nil && !errors.Is(err, domaintenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationMemberNotFound, runtimehttp.MsgOrganizationMemberNotFound)
		c.Abort()
	})
}

func (h *Handler) toOrganizationMemberDTOs(c *gin.Context, members []*domaintenancy.OrganizationMember) ([]v1OrganizationMemberDTO, bool) {
	userIDs := userIDsOfOrganizationMembers(members)
	publicIDs, ok := batchUserPublicIDs(c.Request.Context(), c, h.deps.Users, userIDs)
	if !ok {
		return nil, false
	}
	display := batchDisplayFacts(c.Request.Context(), h.deps.Members.MemberDisplay, userIDs)
	page := make([]v1OrganizationMemberDTO, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		dto := v1OrganizationMemberDTO{UserID: publicIDs[m.UserID], Role: m.Role}
		if created := shared.FormatTime(m.CreatedAt); created != "" {
			dto.CreatedAt = created
		}
		if email, avatar := applyDisplayFact(display, m.UserID); email != "" || avatar != "" {
			dto.Email, dto.Avatar = email, avatar
		}
		page = append(page, dto)
	}
	return page, true
}

type v1UpdateOrganizationMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *Handler) updateOrganizationMember(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{}, func(octx shared.OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		var body v1UpdateOrganizationMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			shared.WriteInputError(c)
			return
		}
		newRole := strings.TrimSpace(body.Data.Role)
		if !domaintenancy.IsValidRole(newRole) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgRenderBoardError2, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		targetUserID, _, ok := h.resolveMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		updated, err := h.deps.Store.UpdateOrganizationMemberRole(
			c.Request.Context(), organization.PublicID, identityPrincipal.UserID(), targetUserID, newRole)
		if err != nil {
			renderOrganizationMemberError(c, err)
			return
		}
		dto, ok := h.toOrganizationMemberDTO(c, updated)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

func (h *Handler) removeOrganizationMember(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{}, func(octx shared.OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		targetUserID, targetExternal, ok := h.resolveMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := domaintenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("organization:member-remove", organization.PublicID, domaintenancy.FormatUserPublicID(targetExternal))
		_, fnErr, ok := h.workspaceDo(c, externalUser, identityPrincipal.UserID(), "organization-member-remove", key, hash, func() (any, error) {
			if err := h.deps.Store.RemoveOrganizationMember(
				c.Request.Context(), organization.PublicID, identityPrincipal.UserID(), targetUserID); err != nil {
				return nil, err
			}
			return &shared.DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderOrganizationMemberError(c, fnErr)
			return
		}
		contract.RenderSuccess(c, shared.DeletedDTO{Deleted: true})
	})
}
