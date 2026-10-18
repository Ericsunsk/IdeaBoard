package control

// Control workspaces family (issue #808): workspace reads and membership
// overrides behind control:workspaces.
//
// Reads and membership mutations go through the tenancy.Store seam;
// member identifiers are usr_ external ids resolved via the shared
// runtimehttp.V1UserDirectory. Operators act with their own user id as actor.
// Hidden or malformed workspace shapes converge to workspace_not_found
// per Baseline 4 anti-enumeration. Legacy admin task/statistics routes
// are untouched.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type controlWorkspace struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	OrganizationID *string    `json:"organization_id,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func newControlWorkspace(workspace *tenancy.Workspace) controlWorkspace {
	out := controlWorkspace{}
	if workspace == nil {
		return out
	}
	out.ID = workspace.PublicID
	out.Name = workspace.Name
	out.OrganizationID = workspace.OrganizationID
	out.ArchivedAt = workspace.ArchivedAt
	out.CreatedAt = workspace.CreatedAt
	out.UpdatedAt = workspace.UpdatedAt
	return out
}

type controlWorkspaceMember struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func newControlWorkspaceMember(member *tenancy.WorkspaceMember) controlWorkspaceMember {
	out := controlWorkspaceMember{Role: member.Role, CreatedAt: member.CreatedAt}
	if member == nil {
		return out
	}
	out.UserID = strings.TrimSpace(member.UserExternalID)
	return out
}

func parseControlWorkspaceID(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		return "", false
	}
	return raw, true
}

// resolveControlMemberUserID maps a usr_ {member_id} to the internal user
// id for the tenancy seam.
func (d *ContractControlDeps) resolveControlMemberUserID(c *gin.Context) (int64, bool) {
	parsed, ok := parseControlUserPublicID(c, c.Param("member_id"))
	if !ok {
		return 0, false
	}
	if d == nil || d.UserDirectory == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable36)
		return 0, false
	}
	userID, err := d.UserDirectory.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		return 0, false
	}
	return userID, true
}

func writeControlTenancyError(c *gin.Context, err error, notFoundMessage string) {
	switch {
	case errors.Is(err, tenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, notFoundMessage)
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeWorkspaceForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, tenancy.ErrLastOwner):
		contract.RenderError(c, http.StatusForbidden, contract.CodeMembershipTransitionForbidden, runtimehttp.MsgMembershipTransitionForbidden)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable37)
	}
}

func (d *ContractControlDeps) getControlWorkspace(c *gin.Context, _ controlContext) {
	if d == nil || d.Tenancy == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		return
	}
	id, ok := parseControlWorkspaceID(c)
	if !ok {
		return
	}
	workspace, err := d.Tenancy.GetWorkspace(c.Request.Context(), id)
	if err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}
	contract.RenderSuccess(c, newControlWorkspace(workspace))
}

func (d *ContractControlDeps) listControlWorkspaceMembers(c *gin.Context, _ controlContext) {
	if d == nil || d.Tenancy == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		return
	}
	id, ok := parseControlWorkspaceID(c)
	if !ok {
		return
	}
	members, err := d.Tenancy.ListWorkspaceMembers(c.Request.Context(), id)
	if err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}
	items := make([]controlWorkspaceMember, 0, len(members))
	for _, member := range members {
		items = append(items, newControlWorkspaceMember(member))
	}
	contract.RenderPaginated(c, items, contract.NewPageMeta("", false))
}

type controlWorkspaceMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (d *ContractControlDeps) addControlWorkspaceMember(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tenancy == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		return
	}
	id, ok := parseControlWorkspaceID(c)
	if !ok {
		return
	}
	var req controlWorkspaceMemberRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	targetUUID, ok := parseControlUserPublicID(c, req.UserID)
	if !ok {
		return
	}
	if d.UserDirectory == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable36)
		return
	}
	targetID, err := d.UserDirectory.ResolveUserID(c.Request.Context(), targetUUID)
	if err != nil || targetID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		return
	}
	role := strings.TrimSpace(req.Role)
	if !tenancy.IsValidRole(role) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest30)
		return
	}
	member, err := d.Tenancy.AddWorkspaceMember(c.Request.Context(), id, targetID, role)
	if err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}
	contract.RenderCreated(c, newControlWorkspaceMember(member))
}

func (d *ContractControlDeps) updateControlWorkspaceMember(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tenancy == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		return
	}
	id, ok := parseControlWorkspaceID(c)
	if !ok {
		return
	}
	targetID, ok := d.resolveControlMemberUserID(c)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	role := strings.TrimSpace(req.Role)
	if !tenancy.IsValidRole(role) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest30)
		return
	}
	member, err := d.Tenancy.UpdateWorkspaceMemberRole(c.Request.Context(), id, ctx.Operator.UserID(), targetID, role)
	if err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}
	contract.RenderSuccess(c, newControlWorkspaceMember(member))
}

func (d *ContractControlDeps) removeControlWorkspaceMember(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tenancy == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		return
	}
	id, ok := parseControlWorkspaceID(c)
	if !ok {
		return
	}
	targetID, ok := d.resolveControlMemberUserID(c)
	if !ok {
		return
	}
	if _, err := d.Tenancy.RemoveWorkspaceMember(c.Request.Context(), id, ctx.Operator.UserID(), targetID, d.Boards); err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}

	contract.RenderSuccess(c, struct {
		Removed bool `json:"removed"`
	}{Removed: true})
}
