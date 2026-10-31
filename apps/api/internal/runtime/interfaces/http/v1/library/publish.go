package library

import (
	"context"
	"strings"

	librarydomain "ideaboard/api/internal/library/domain"
	taskdomain "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tenancy"
)

// Library lineage is visible only to callers who may read the producing Task.
func (h *Handler) redactLibraryLineageForCopy(ctx context.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, asset *librarydomain.V1Asset) *librarydomain.V1Asset {
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
	var access *taskdomain.BoardAccess
	if h.deps.ResolveTaskBoardAccess != nil {
		access = h.deps.ResolveTaskBoardAccess(ctx, workspace, workspaceRole, userID, task)
	}
	if !taskdomain.AuthorizeTaskRead(task, userID, access) {
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
func (h *Handler) libraryCallerRole(ctx context.Context, workspace *tenancy.Workspace, userID int64) (string, bool) {
	if h.deps.Store == nil {
		return "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(ctx, workspace.PublicID, userID)
	if err != nil || !member {
		return "", false
	}
	return role, true
}
