package workspacemedia

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// TaskAssetAdapter adapts the workspace media store to the
// workspacetasks AssetReader seam (task input verification: invisible
// assets fail closed as media-not-found, archived as media-archived,
// non-ready as media-not-ready). It reports through the
// workspacetasks sentinels so settlement maps them without knowing
// Media internals. Wire it as workspacetasks.ServiceDeps.Assets once
// the task surface consumes media inputs (#821).
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
