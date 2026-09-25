package composition

import (
	"context"
	"fmt"

	"ideaboard/api/internal/library"
)

// workspaceTaskLibrarySaver is the production task-output adapter. It keeps
// task identity out of the public library request shape while delegating the
// durable workspace/media validation and Quick Saves placement to library.Service.
type workspaceTaskLibrarySaver struct {
	store *library.Service
}

var _ TaskOutputLibrarySaver = (*workspaceTaskLibrarySaver)(nil)

func newWorkspaceTaskLibrarySaver(store *library.Service) TaskOutputLibrarySaver {
	if store == nil {
		return nil
	}
	return &workspaceTaskLibrarySaver{store: store}
}

func (s *workspaceTaskLibrarySaver) SaveTaskOutputAsset(ctx context.Context, workspacePublicID, taskID string, resultIndex int, mediaAssetID string) (*library.V1Asset, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, fmt.Errorf("workspace task library saver is not configured")
	}
	if taskID == "" || resultIndex < 0 {
		return nil, false, fmt.Errorf("workspace task library saver: task output identity required")
	}
	publicID, err := library.NewV1AssetPublicID()
	if err != nil {
		return nil, false, err
	}
	sourceID := fmt.Sprintf("task:%s:%d", taskID, resultIndex)
	return s.store.SaveManagedMediaAsset(ctx, workspacePublicID, sourceID, mediaAssetID, publicID)
}
