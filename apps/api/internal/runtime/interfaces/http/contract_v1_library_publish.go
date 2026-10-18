package httpapi

import (
	"context"
	"strings"

	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
)

// Library lineage is visible only to callers who may read the producing Task.
func (h *contractV1Handler) redactLibraryLineageForCopy(ctx context.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, asset *librarydomain.V1Asset) *librarydomain.V1Asset {
	if asset == nil || asset.Lineage == nil {
		return asset
	}
	if h.deps.Tasks == nil {
		return stripLibraryLineage(asset)
	}
	task, err := h.deps.Tasks.GetTask(ctx, workspace.PublicID, strings.TrimSpace(asset.Lineage.TaskID))
	if err != nil {
		return stripLibraryLineage(asset)
	}
	access := h.resolveTaskBoardAccess(ctx, workspace, workspaceRole, userID, task)
	if !workspacetasks.AuthorizeTaskRead(task, userID, access) {
		return stripLibraryLineage(asset)
	}
	return asset
}

func stripLibraryLineage(asset *librarydomain.V1Asset) *librarydomain.V1Asset {
	if asset == nil {
		return nil
	}
	out := *asset
	out.Lineage = nil
	return &out
}

// libraryCallerRole returns the workspace role for Library publish and
// redaction decisions. Library routes resolve membership without a role,
// so it is fetched here; unknown callers read as not found.
func (h *contractV1Handler) libraryCallerRole(ctx context.Context, workspace *tenancy.Workspace, userID int64) (string, bool) {
	return h.taskWorkspaceRoleString(ctx, workspace, userID)
}
