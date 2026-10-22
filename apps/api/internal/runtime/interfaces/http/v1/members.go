package v1

import (
	"context"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type v1MemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	// Email is the workspace-visible member email for invite search and
	// display assistance only. It is omitted when unavailable and never
	// acts as a write identity (writes stay usr_).
	Email string `json:"email,omitempty"`
	// Avatar is the workspace-visible member avatar public URL
	// (runtimehttp.V1PublicAvatarURLForRow projection). It is omitted when the
	// member has no custom avatar or the provider is unavailable, and
	// never fails the request.
	Avatar string `json:"avatar,omitempty"`
}

type v1DeletedDTO struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id,omitempty"`
}

// toMemberDTO translates a member row to the API shape. Without a user
// directory (tests always provide one) the handler fails closed with 501
// instead of leaking numeric database ids.
func (h *contractV1Handler) toMemberDTO(c *gin.Context, m *tenancy.WorkspaceMember) (v1MemberDTO, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return v1MemberDTO{}, false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), m.UserID)
	if err != nil || publicID == uuid.Nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
		c.Abort()
		return v1MemberDTO{}, false
	}
	dto := v1MemberDTO{UserID: tenancy.FormatUserPublicID(publicID), Role: m.Role}
	if created := v1FormatTime(m.CreatedAt); created != "" {
		dto.CreatedAt = created
	}
	return dto, true
}

func (h *contractV1Handler) resolveMemberUserID(c *gin.Context, raw string) (int64, uuid.UUID, bool) {
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
	parsed, err := tenancy.ParseUserPublicID(trimmed)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
		return 0, uuid.Nil, false
	}
	userID, err := h.deps.Users.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		// Unknown users are invisible (404), never 403, per anti-enumeration.
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
		return 0, uuid.Nil, false
	}
	return userID, parsed, true
}

func renderMemberTransitionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached):
		contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceMemberLimitReached, "Workspace supports up to 5 members, including owners")
	case errors.Is(err, tenancy.ErrDefaultWorkspaceProtected):
		contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
			contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
	case errors.Is(err, tenancy.ErrLastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden2)
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden3)
	case errors.Is(err, tenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

func (h *contractV1Handler) listWorkspaceMembers(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		members, err := h.deps.Store.ListWorkspaceMembers(c.Request.Context(), wctx.Workspace.PublicID)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
			c.Abort()
			return
		}
		page := make([]v1MemberDTO, 0, len(members))
		for _, m := range members {
			dto, ok := h.toMemberDTO(c, m)
			if !ok {
				return
			}
			page = append(page, dto)
		}
		// Best-effort display enrichment (email + avatar): unknown or
		// missing facts stay omitted and never fail the list. Only
		// members of this workspace are enriched, so facts never leak
		// across workspaces.
		h.enrichMemberDisplay(c.Request.Context(), members, page)
		contract.RenderSuccess(c, page)
	})
}

// enrichMemberDisplay joins workspace-member display facts (Auth-owned
// email plus avatar public URL) onto the DTOs in place. Single code path
// for the list and single-member writes: writes pass their one member.
// It fails closed: a nil provider, lookup error, or missing fact leaves
// the corresponding field omitted while the request still renders 2xx.
// Facts resolve by member userID lookup, never by position; only the
// passed workspace members are resolved, so facts never leak across
// workspaces.
func (h *contractV1Handler) enrichMemberDisplay(ctx context.Context, members []*tenancy.WorkspaceMember, page []v1MemberDTO) {
	if h.deps.MemberDisplay == nil || len(members) == 0 || len(page) == 0 {
		return
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m == nil || m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	if len(ids) == 0 {
		return
	}
	display, err := h.deps.MemberDisplay.GetMemberDisplay(ctx, ids)
	if err != nil || len(display) == 0 {
		return
	}
	for i, m := range members {
		if m == nil || i >= len(page) {
			continue
		}
		fact, ok := display[m.UserID]
		if !ok {
			continue
		}
		if email := strings.TrimSpace(fact.Email); email != "" {
			page[i].Email = email
		}
		if avatar := strings.TrimSpace(fact.Avatar); avatar != "" {
			page[i].Avatar = avatar
		}
	}
}

type v1AddMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

// workspaceDo runs fn under the header-only Idempotency-Key scope
// workspace:<op>: missing keys render 400 idempotency_key_required,
// key reuse with a different request hash renders 409
// workspace_idempotency_conflict (aborted), same key+hash replays the
// first result. Domain errors from fn are returned for the caller to
// map (they must not collapse to 503 here). Mirrors boardDo.
func (h *contractV1Handler) workspaceDo(c *gin.Context, userExternalID string, userID int64, op, key, hash string, fn func() (any, error)) (any, error, bool) {
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

func (h *contractV1Handler) addWorkspaceMember(c *gin.Context) {
	// Key before bind (board add precedent): a missing key fails closed
	// with 400 even when the body is also malformed, so this handler
	// uses withWorkspace plus a manual bind instead of
	// withV1WorkspaceBody.
	h.withWorkspace(c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		// Personal Default is private (#952): fail closed before any
		// key or body handling beyond visibility. Non-Default Personal
		// workspaces invite freely with no per-invite holder approval;
		// absent collaborator quota reads as unlimited.
		if wctx.Workspace.IsDefaultWorkspace() {
			contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected),
				contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
			c.Abort()
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1AddMemberBody
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
		if err := tenancy.CheckMemberAdd(wctx.Role, newRole); err != nil {
			renderMemberTransitionError(c, err)
			return
		}
		targetUserID, _, ok := h.resolveMemberUserID(c, body.Data.UserID)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
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
		added, ok := result.(*tenancy.WorkspaceMember)
		if !ok || added == nil {
			renderMemberTransitionError(c, errors.New("tenancy: member-add type mismatch"))
			return
		}
		dto, ok := h.toMemberDTO(c, added)
		if !ok {
			return
		}
		displayPage := []v1MemberDTO{dto}
		h.enrichMemberDisplay(c.Request.Context(), []*tenancy.WorkspaceMember{added}, displayPage)
		dto = displayPage[0]
		contract.RenderCreated(c, dto)
	})
}

type v1UpdateMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) updateWorkspaceMember(c *gin.Context) {
	// Visibility first (hidden -> 404); the store enforces the
	// actor-role transition rules below (403 on violation).
	withV1WorkspaceBody(h, c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx, body v1UpdateMemberBody) {
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
		h.enrichMemberDisplay(c.Request.Context(), []*tenancy.WorkspaceMember{updated}, displayPage)
		dto = displayPage[0]
		contract.RenderSuccess(c, dto)
	})
}

func (h *contractV1Handler) removeWorkspaceMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		// Resolve before the key (board remove precedent): unknown
		// member ids stay invisible with 404 even when the key is
		// also missing.
		targetUserID, targetExternal, ok := h.resolveMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("workspace:member-remove", wctx.Workspace.PublicID, tenancy.FormatUserPublicID(targetExternal))
		result, fnErr, ok := h.workspaceDo(c, externalUser, wctx.Principal.UserID(), "member-remove", key, hash, func() (any, error) {
			cleaned, err := h.deps.Store.RemoveWorkspaceMember(
				c.Request.Context(), wctx.Workspace.PublicID, wctx.Principal.UserID(), targetUserID, h.deps.Boards)
			if err != nil {
				return nil, err
			}
			for _, board := range cleaned {
				h.deliverBoardAccessSignals(c.Request.Context(), board.AccessSignals)
			}

			return &v1DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderMemberTransitionError(c, fnErr)
			return
		}
		deleted, ok := result.(*v1DeletedDTO)
		if !ok || deleted == nil {
			renderMemberTransitionError(c, errors.New("tenancy: member-remove type mismatch"))
			return
		}
		contract.RenderSuccess(c, *deleted)
	})
}
