package adapters

import (
	"context"

	taskdomain "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tenancy"
)

// ReadinessWorkspaceReader projects Tenancy facts without acquiring submission
// locks. Hidden workspaces and absent membership share the Task not-found error.
type ReadinessWorkspaceReader struct {
	Store interface {
		GetWorkspace(context.Context, string) (*tenancy.Workspace, error)
		GetWorkspaceRole(context.Context, string, int64) (string, bool, error)
	}
}

var _ taskports.ReadinessWorkspaceReader = (*ReadinessWorkspaceReader)(nil)

func (r *ReadinessWorkspaceReader) ResolveReadinessWorkspace(ctx context.Context, workspaceID string, userID int64) (taskports.ReadinessWorkspace, error) {
	if r == nil || r.Store == nil {
		return taskports.ReadinessWorkspace{}, taskdomain.ErrSettlementUnavailable
	}
	workspace, err := r.Store.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return taskports.ReadinessWorkspace{}, taskdomain.ErrNotFound
	}
	role, member, err := r.Store.GetWorkspaceRole(ctx, workspaceID, userID)
	if err != nil || !member {
		return taskports.ReadinessWorkspace{}, taskdomain.ErrNotFound
	}
	return taskports.ReadinessWorkspace{Role: role, Organization: !workspace.IsPersonal()}, nil
}
