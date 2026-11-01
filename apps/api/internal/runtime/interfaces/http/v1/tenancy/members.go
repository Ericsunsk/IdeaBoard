package tenancy

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	Email     string `json:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
}

type v1MemberDTO = MemberDTO

func (h *Handler) toMemberDTO(c *gin.Context, m *domaintenancy.WorkspaceMember) (v1MemberDTO, bool) {
	if m == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
		c.Abort()
		return v1MemberDTO{}, false
	}
	page, ok := h.toWorkspaceMemberDTOs(c, []*domaintenancy.WorkspaceMember{m})
	if !ok || len(page) == 0 {
		return v1MemberDTO{}, false
	}
	return page[0], true
}

func (h *Handler) resolveMemberUserID(c *gin.Context, raw string) (int64, uuid.UUID, bool) {
	trimmed := strings.TrimSpace(raw)
	if !contract.IsUserID(trimmed) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
		return 0, uuid.Nil, false
	}
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return 0, uuid.Nil, false
	}
	parsed, err := domaintenancy.ParseUserPublicID(trimmed)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
		return 0, uuid.Nil, false
	}
	userID, err := h.deps.Users.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
		return 0, uuid.Nil, false
	}
	return userID, parsed, true
}

func renderMemberTransitionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domaintenancy.ErrWorkspaceMemberLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceMemberLimitReached, "Workspace supports up to 5 members, including owners")
	case errors.Is(err, domaintenancy.ErrDefaultWorkspaceProtected):
		contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
			contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
	case errors.Is(err, domaintenancy.ErrLastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden2)
	case errors.Is(err, domaintenancy.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, domaintenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

func (h *Handler) listWorkspaceMembers(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{}, func(wctx shared.WorkspaceCtx) {
		members, err := h.deps.Store.ListWorkspaceMembers(c.Request.Context(), wctx.Workspace.PublicID)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
			c.Abort()
			return
		}
		page, ok := h.toWorkspaceMemberDTOs(c, members)
		if !ok {
			return
		}
		contract.RenderSuccess(c, page)
	})
}

func (h *Handler) toWorkspaceMemberDTOs(c *gin.Context, members []*domaintenancy.WorkspaceMember) ([]v1MemberDTO, bool) {
	userIDs := userIDsOfWorkspaceMembers(members)
	publicIDs, ok := batchUserPublicIDs(c.Request.Context(), c, h.deps.Users, userIDs)
	if !ok {
		return nil, false
	}
	display := batchDisplayFacts(c.Request.Context(), h.deps.Members.MemberDisplay, userIDs)
	page := make([]v1MemberDTO, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		dto := v1MemberDTO{UserID: publicIDs[m.UserID], Role: m.Role}
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

func (h *Handler) enrichMemberDisplay(ctx context.Context, members []*domaintenancy.WorkspaceMember, page []v1MemberDTO) {
	if len(members) == 0 || len(page) == 0 {
		return
	}
	display := batchDisplayFacts(ctx, h.deps.Members.MemberDisplay, userIDsOfWorkspaceMembers(members))
	if len(display) == 0 {
		return
	}
	for i, m := range members {
		if m == nil || i >= len(page) {
			continue
		}
		if email, avatar := applyDisplayFact(display, m.UserID); email != "" || avatar != "" {
			page[i].Email, page[i].Avatar = email, avatar
		}
	}
}

type v1AddMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

func (h *Handler) workspaceDo(c *gin.Context, userExternalID string, userID int64, op, key, hash string, fn func() (any, error)) (any, error, bool) {
	scope := contract.IdempotencyScope(userExternalID, "workspace:"+op)
	result, _, err := h.deps.Idempotency.Do(c.Request.Context(), scope, userID, key, hash, fn)
	if err != nil {
		if contract.IsIdempotencyConflict(err) {
			contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
			c.Abort()
			return nil, nil, false
		}
		return nil, err, true
	}
	return result, nil, true
}

func (h *Handler) grantOrganizationWorkspaceMember(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Allow: shared.AllowOwnerOrAdmin}, func(wctx shared.WorkspaceCtx) {
		if wctx.Workspace.IsDefaultWorkspace() {
			contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
				contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
			c.Abort()
			return
		}
		if wctx.Workspace.OrganizationID == nil {
			renderMemberTransitionError(c, domaintenancy.ErrForbidden)
			return
		}
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		var body v1AddMemberBody
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
		if err := domaintenancy.CheckMemberAdd(wctx.Role, newRole); err != nil {
			renderMemberTransitionError(c, err)
			return
		}
		targetUserID, _, ok := h.resolveMemberUserID(c, body.Data.UserID)
		if !ok {
			return
		}
		externalUser := domaintenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("workspace:member-add", wctx.Workspace.PublicID, body.Data.UserID, newRole)
		result, fnErr, ok := h.workspaceDo(c, externalUser, wctx.Principal.UserID(), "member-add", key, hash, func() (any, error) {
			added, err := h.deps.Store.AddWorkspaceMember(c.Request.Context(), wctx.Workspace.PublicID, targetUserID, newRole)
			if err != nil {
				return nil, err
			}
			return added, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderMemberTransitionError(c, fnErr)
			return
		}
		added, ok := result.(*domaintenancy.WorkspaceMember)
		if !ok || added == nil {
			renderMemberTransitionError(c, errors.New("tenancy: member-add type mismatch"))
			return
		}
		dto, ok := h.toMemberDTO(c, added)
		if !ok {
			return
		}
		displayPage := []v1MemberDTO{dto}
		h.enrichMemberDisplay(c.Request.Context(), []*domaintenancy.WorkspaceMember{added}, displayPage)
		dto = displayPage[0]
		contract.RenderCreated(c, dto)
	})
}

type v1UpdateMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *Handler) updateWorkspaceMember(c *gin.Context) {
	shared.WithWorkspaceBodyStrict(c, h.requireStore, func(c *gin.Context, p principal.Principal) (*domaintenancy.Workspace, string, bool) {
		return shared.ResolveWorkspaceContext(c, h.deps.Store, p)
	}, nil, func(wctx shared.WorkspaceCtx, body v1UpdateMemberBody) {
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
		updated, err := h.deps.Store.UpdateWorkspaceMemberRole(
			c.Request.Context(), wctx.Workspace.PublicID, wctx.Principal.UserID(), targetUserID, newRole)
		if err != nil {
			renderMemberTransitionError(c, err)
			return
		}
		dto, ok := h.toMemberDTO(c, updated)
		if !ok {
			return
		}
		displayPage := []v1MemberDTO{dto}
		h.enrichMemberDisplay(c.Request.Context(), []*domaintenancy.WorkspaceMember{updated}, displayPage)
		dto = displayPage[0]
		contract.RenderSuccess(c, dto)
	})
}

func (h *Handler) removeWorkspaceMember(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{}, func(wctx shared.WorkspaceCtx) {
		targetUserID, targetExternal, ok := h.resolveMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := domaintenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("workspace:member-remove", wctx.Workspace.PublicID, domaintenancy.FormatUserPublicID(targetExternal))
		result, fnErr, ok := h.workspaceDo(c, externalUser, wctx.Principal.UserID(), "member-remove", key, hash, func() (any, error) {
			cleaned, err := h.deps.Store.RemoveWorkspaceMember(
				c.Request.Context(), wctx.Workspace.PublicID, wctx.Principal.UserID(), targetUserID, h.deps.Boards)
			if err != nil {
				return nil, err
			}
			for _, board := range cleaned {
				if h.deps.DeliverBoardAccessSignals != nil {
					h.deps.DeliverBoardAccessSignals(c.Request.Context(), board.AccessSignals)
				}
			}

			return &shared.DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderMemberTransitionError(c, fnErr)
			return
		}
		deleted, ok := result.(*shared.DeletedDTO)
		if !ok || deleted == nil {
			renderMemberTransitionError(c, errors.New("tenancy: member-remove type mismatch"))
			return
		}
		contract.RenderSuccess(c, *deleted)
	})
}
