package runtime

import (
	"context"
	"fmt"

	librarydomain "ideaboard/api/internal/library/domain"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
)

// workspaceTaskLibrarySaver is the production task-output adapter. It keeps
// task identity out of the public library request shape while delegating the
// durable workspace/media validation and Quick Saves placement to librarypostgres.Service.
type workspaceTaskLibrarySaver struct {
	store *librarypostgres.Service
}

var _ TaskOutputLibrarySaver = (*workspaceTaskLibrarySaver)(nil)

func NewWorkspaceTaskLibrarySaver(store *librarypostgres.Service) TaskOutputLibrarySaver {
	if store == nil {
		return nil
	}
	return &workspaceTaskLibrarySaver{store: store}
}

func (s *workspaceTaskLibrarySaver) SaveTaskOutputAsset(ctx context.Context, workspacePublicID, taskID string, resultIndex int, mediaAssetID string) (*librarydomain.V1Asset, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, fmt.Errorf("workspace task library saver is not configured")
	}
	if taskID == "" || resultIndex < 0 {
		return nil, false, fmt.Errorf("workspace task library saver: task output identity required")
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		return nil, false, err
	}
	sourceID := fmt.Sprintf("task:%s:%d", taskID, resultIndex)
	return s.store.SaveManagedMediaAsset(ctx, workspacePublicID, sourceID, mediaAssetID, publicID)
}
