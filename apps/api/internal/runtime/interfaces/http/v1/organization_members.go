package v1

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type v1OrganizationMemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	Email     string `json:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
}

func (h *contractV1Handler) toOrganizationMemberDTO(c *gin.Context, m *tenancy.OrganizationMember) (v1OrganizationMemberDTO, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return v1OrganizationMemberDTO{}, false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), m.UserID)
	if err != nil || publicID == uuid.Nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
		c.Abort()
		return v1OrganizationMemberDTO{}, false
	}
	dto := v1OrganizationMemberDTO{UserID: tenancy.FormatUserPublicID(publicID), Role: m.Role}
	if created := v1FormatTime(m.CreatedAt); created != "" {
		dto.CreatedAt = created
	}
	return dto, true
}

func renderOrganizationMemberError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, tenancy.ErrOrganizationLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationLimitReached, runtimehttp.MsgOrganizationLimitReached)
	case errors.Is(err, tenancy.ErrOrganizationOwnerImmutable):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, tenancy.ErrLastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden2)
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, tenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationMemberNotFound, runtimehttp.MsgOrganizationMemberNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
	}
	c.Abort()
}

func (h *contractV1Handler) listOrganizationMembers(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{}, func(octx v1OrganizationCtx) {
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

// toOrganizationMemberDTOs builds the organization member list in three
// round trips total: the store list (caller) plus one identity batch and
// one display batch, replacing the per-member GetUserPublicID N+1 loop and
// covering organization members with the same email/avatar enrichment the
// workspace list carries. Unknown identities fail closed with 503,
// matching the single-row toOrganizationMemberDTO.
func (h *contractV1Handler) toOrganizationMemberDTOs(c *gin.Context, members []*tenancy.OrganizationMember) ([]v1OrganizationMemberDTO, bool) {
	userIDs := userIDsOfOrganizationMembers(members)
	publicIDs, ok := batchUserPublicIDs(c.Request.Context(), c, h.deps.Users, userIDs)
	if !ok {
		return nil, false
	}
	display := batchDisplayFacts(c.Request.Context(), h.deps.MemberDisplay, userIDs)
	page := make([]v1OrganizationMemberDTO, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		dto := v1OrganizationMemberDTO{UserID: publicIDs[m.UserID], Role: m.Role}
		if created := v1FormatTime(m.CreatedAt); created != "" {
			dto.CreatedAt = created
		}
		if email, avatar := applyDisplayFact(display, m.UserID); email != "" || avatar != "" {
			dto.Email, dto.Avatar = email, avatar
		}
		page = append(page, dto)
	}
	return page, true
}

type v1AddOrganizationMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) addOrganizationMember(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		role := octx.Role
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1AddOrganizationMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newRole := strings.TrimSpace(body.Data.Role)
		if !tenancy.IsValidRole(newRole) || newRole == tenancy.RoleOwner {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgRenderBoardError2, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		if err := tenancy.CheckOrganizationMemberAdd(role, newRole); err != nil {
			renderOrganizationMemberError(c, err)
			return
		}
		targetUserID, _, ok := h.resolveMemberUserID(c, body.Data.UserID)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		scope := contract.IdempotencyScope(externalUser, "organization:member-add")
		hash := contract.HashIdempotencyRequest("organization:member-add", organization.PublicID, body.Data.UserID, newRole)
		result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			return h.deps.Store.AddOrganizationMember(ctx, organization.PublicID, targetUserID, newRole)
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			renderOrganizationMemberError(c, doErr)
			return
		}
		added, ok := result.(*tenancy.OrganizationMember)
		if !ok || added == nil {
			renderOrganizationMemberError(c, errors.New("tenancy: organization member-add type mismatch"))
			return
		}
		dto, ok := h.toOrganizationMemberDTO(c, added)
		if !ok {
			return
		}
		contract.RenderCreated(c, dto)
	})
}

type v1UpdateOrganizationMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) updateOrganizationMember(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{}, func(octx v1OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		var body v1UpdateOrganizationMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newRole := strings.TrimSpace(body.Data.Role)
		if !tenancy.IsValidRole(newRole) {
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

func (h *contractV1Handler) removeOrganizationMember(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{}, func(octx v1OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		targetUserID, targetExternal, ok := h.resolveMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("organization:member-remove", organization.PublicID, tenancy.FormatUserPublicID(targetExternal))
		_, fnErr, ok := h.workspaceDo(c, externalUser, identityPrincipal.UserID(), "organization-member-remove", key, hash, func() (any, error) {
			if err := h.deps.Store.RemoveOrganizationMember(
				c.Request.Context(), organization.PublicID, identityPrincipal.UserID(), targetUserID); err != nil {
				return nil, err
			}
			return &v1DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderOrganizationMemberError(c, fnErr)
			return
		}
		contract.RenderSuccess(c, v1DeletedDTO{Deleted: true})
	})
}
