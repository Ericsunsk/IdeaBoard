package httpapi

import (
	"errors"
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "成员服务暂不可用")
		c.Abort()
		return v1MemberDTO{}, false
	}
	dto := v1MemberDTO{UserID: tenancy.FormatUserPublicID(publicID), Role: m.Role}
	if created := v1FormatTime(m.CreatedAt); created != "" {
		dto.CreatedAt = created
	}
	return dto, true
}

func (h *contractV1Handler) resolveMemberUserID(c *gin.Context, raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if !contract.IsUserID(trimmed) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, "成员不存在")
		c.Abort()
		return 0, false
	}
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return 0, false
	}
	parsed, err := tenancy.ParseUserPublicID(trimmed)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, "成员不存在")
		c.Abort()
		return 0, false
	}
	userID, err := h.deps.Users.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		// Unknown users are invisible (404), never 403, per anti-enumeration.
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, "成员不存在")
		c.Abort()
		return 0, false
	}
	return userID, true
}

func renderMemberTransitionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrLastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, "最后一个所有者不能被移除或降级")
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, "成员角色变更需要更高权限")
	case errors.Is(err, tenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, "成员不存在")
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "成员服务暂不可用")
	}
	c.Abort()
}

func (h *contractV1Handler) listWorkspaceMembers(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	workspace, _, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	members, err := h.deps.Store.ListWorkspaceMembers(c.Request.Context(), workspace.PublicID)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
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
	contract.RenderSuccess(c, page)
}

type v1AddMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) addWorkspaceMember(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	workspace, actorRole, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, actorRole == tenancy.RoleOwner || actorRole == tenancy.RoleAdmin) {
		return
	}
	var body v1AddMemberBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	newRole := strings.TrimSpace(body.Data.Role)
	if !tenancy.IsValidRole(newRole) {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"成员角色非法", contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
		c.Abort()
		return
	}
	if err := tenancy.CheckMemberAdd(actorRole, newRole); err != nil {
		renderMemberTransitionError(c, err)
		return
	}
	targetUserID, ok := h.resolveMemberUserID(c, body.Data.UserID)
	if !ok {
		return
	}
	added, err := h.deps.Store.AddWorkspaceMember(c.Request.Context(), workspace.PublicID, targetUserID, newRole)
	if err != nil {
		renderMemberTransitionError(c, err)
		return
	}
	dto, ok := h.toMemberDTO(c, added)
	if !ok {
		return
	}
	contract.RenderCreated(c, dto)
}

type v1UpdateMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) updateWorkspaceMember(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	workspace, _, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	// Visibility first (hidden -> 404); the store enforces the
	// actor-role transition rules below (403 on violation).
	if !RequireWorkspaceCapability(c, visible, true) {
		return
	}
	var body v1UpdateMemberBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	newRole := strings.TrimSpace(body.Data.Role)
	if !tenancy.IsValidRole(newRole) {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"成员角色非法", contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
		c.Abort()
		return
	}
	targetUserID, ok := h.resolveMemberUserID(c, c.Param("member_id"))
	if !ok {
		return
	}
	updated, err := h.deps.Store.UpdateWorkspaceMemberRole(
		c.Request.Context(), workspace.PublicID, identityPrincipal.UserID(), targetUserID, newRole)
	if err != nil {
		renderMemberTransitionError(c, err)
		return
	}
	dto, ok := h.toMemberDTO(c, updated)
	if !ok {
		return
	}
	contract.RenderSuccess(c, dto)
}

func (h *contractV1Handler) removeWorkspaceMember(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	workspace, _, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, true) {
		return
	}
	targetUserID, ok := h.resolveMemberUserID(c, c.Param("member_id"))
	if !ok {
		return
	}
	if err := h.deps.Store.RemoveWorkspaceMember(
		c.Request.Context(), workspace.PublicID, identityPrincipal.UserID(), targetUserID); err != nil {
		renderMemberTransitionError(c, err)
		return
	}
	contract.RenderSuccess(c, v1DeletedDTO{Deleted: true})
}
