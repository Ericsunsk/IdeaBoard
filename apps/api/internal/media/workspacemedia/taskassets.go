package workspacemedia

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// TaskAssetAdapter supplies readiness facts for worker execution of accepted
// immutable inputs. Submission authorization belongs to TaskSubmissionAuthorizer.
type TaskAssetAdapter struct {
	Store AssetStore
}

// Compile-time convergence with the task seam.
var _ workspacetasks.AssetReader = (*TaskAssetAdapter)(nil)

// GetAsset projects one workspace-scoped asset to the minimal
// ready-gate facts tasks need. Unknown, malformed, and
// cross-workspace ids converge to ErrMediaNotFound (invisible, never
// a distinct status).
func (a *TaskAssetAdapter) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacetasks.AssetInfo, error) {
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("workspacemedia: task asset store is not configured: %w", workspacetasks.ErrMediaNotFound)
	}
	asset, err := a.Store.GetAsset(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(assetID))
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: task asset lookup: %w", workspacetasks.ErrMediaNotFound)
	}
	if asset.IsArchived() {
		return nil, fmt.Errorf("workspacemedia: task asset %s archived: %w", asset.ID, workspacetasks.ErrMediaArchived)
	}
	if strings.ToLower(strings.TrimSpace(asset.Status)) != AssetStatusReady {
		return nil, fmt.Errorf("workspacemedia: task asset %s not ready: %w", asset.ID, workspacetasks.ErrMediaNotReady)
	}
	return &workspacetasks.AssetInfo{
		ID:          asset.ID,
		WorkspaceID: asset.WorkspaceID,
		Status:      asset.Status,
		Archived:    false,
	}, nil
}
