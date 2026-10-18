package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// taskBoardsAuthorizer returns the Board authorizer for the task surface,
// or nil when the Boards store is unwired (bound submits and bound reads
// then fail closed inside the domain).
func (h *contractV1Handler) taskBoardsAuthorizer() workspacetasks.BoardAuthorizer {
	if h.deps.Boards == nil {
		return nil
	}
	return &boardtaskaccess.Authorizer{Store: h.deps.Boards}
}

// taskWorkspaceRoleString returns the caller's workspace role string for
// Board posture resolution. Task routes resolve membership without a role
// (resolveTaskWorkspace), so Board-bound operations fetch it here; unknown
// callers read as not found (never 403).
func (h *contractV1Handler) taskWorkspaceRoleString(ctx context.Context, workspace *tenancy.Workspace, userID int64) (string, bool) {
	role, member, err := h.deps.Store.GetWorkspaceRole(ctx, workspace.PublicID, userID)
	if err != nil || !member {
		return "", false
	}
	return role, true
}

// resolveTaskBoardAccess resolves the caller's live Board posture for one
// task. Board-less tasks report nil (submitter-only visibility needs no
// Boards facts); invisible Boards report nil so callers converge to 404.
func (h *contractV1Handler) resolveTaskBoardAccess(ctx context.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, task *workspacetasks.WorkspaceTask) *workspacetasks.BoardAccess {
	if task == nil || strings.TrimSpace(task.BoardID) == "" {
		return nil
	}
	authorizer := h.taskBoardsAuthorizer()
	if authorizer == nil {
		return nil
	}
	access, err := authorizer.ResolveBoardAccess(ctx, workspace.PublicID, task.BoardID, userID, workspaceRole)
	if err != nil {
		return nil
	}
	return access
}

// requireVisibleWorkspaceTask gates one task read (detail, outputs, or
// cancel prelude) on the #937 visibility rule: the submitter, or a current
// Board reader on the linked Board. Anything else converges to 404
// task_not_found (never leaking whether the task exists).
func (h *contractV1Handler) requireVisibleWorkspaceTask(c *gin.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, task *workspacetasks.WorkspaceTask) bool {
	access := h.resolveTaskBoardAccess(c.Request.Context(), workspace, workspaceRole, userID, task)
	if !workspacetasks.AuthorizeTaskRead(task, userID, access) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, msgControlNotFound8)
		c.Abort()
		return false
	}
	return true
}

// isTaskContentEvent reports task lifecycle progress envelopes plus
// task-referencing billing settlement (#937 TaskBoardScope): the only
// workspace-broadcast envelopes whose delivery narrows to submitter or
// current Board readers. All other types keep their existing (#940)
// delivery rule.
func isTaskContentEvent(envelope events.V1Envelope) bool {
	switch strings.TrimSpace(envelope.Type) {
	case events.EventV1TaskCreated, events.EventV1TaskRunning, events.EventV1TaskCompleted,
		events.EventV1TaskFailed, events.EventV1TaskCancelled:
		return true
	case events.EventV1BillingSettled:
		var settled events.V1BillingSettledData
		if err := json.Unmarshal(envelope.Data, &settled); err != nil {
			return false
		}
		return strings.TrimSpace(settled.ReferenceType) == events.V1ResourceTask &&
			strings.TrimSpace(settled.ReferenceID) != ""
	default:
		return false
	}
}

// taskContentTaskID extracts the task identity from a task-content
// envelope: the resource id for task resources, the billing reference
// for task-referencing settlement.
func taskContentTaskID(envelope events.V1Envelope) string {
	if strings.TrimSpace(envelope.Type) == events.EventV1BillingSettled {
		var settled events.V1BillingSettledData
		if err := json.Unmarshal(envelope.Data, &settled); err != nil {
			return ""
		}
		if strings.TrimSpace(settled.ReferenceType) != events.V1ResourceTask {
			return ""
		}
		return strings.TrimSpace(settled.ReferenceID)
	}
	if strings.TrimSpace(envelope.Resource.Type) != events.V1ResourceTask {
		return ""
	}
	return strings.TrimSpace(envelope.Resource.ID)
}

// taskEventVisibleToViewer resolves one task-content envelope against the
// visibility rule at delivery time: the submitter, or a current Board
// reader on the linked Board. Unknown tasks and missing stores deny delivery.
func (h *contractV1Handler) taskEventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer workspaceEventViewer, envelope events.V1Envelope) bool {
	taskID := taskContentTaskID(envelope)
	if taskID == "" {
		return false
	}
	if h.deps.Tasks == nil {
		return false
	}
	task, err := h.deps.Tasks.GetTask(ctx, strings.TrimSpace(workspacePublicID), taskID)
	if err != nil {
		return false
	}
	if viewer.userID > 0 && task.OwnerUserID == viewer.userID {
		return true
	}
	if strings.TrimSpace(task.BoardID) == "" {
		return false
	}
	roles := h.boardRoles()
	if roles == nil {
		return false
	}
	_, hasRole, err := roles.GetBoardRole(ctx, strings.TrimSpace(workspacePublicID), strings.TrimSpace(task.BoardID), viewer.userID, viewer.workspaceRole)
	if err != nil {
		return false
	}
	return hasRole
}
