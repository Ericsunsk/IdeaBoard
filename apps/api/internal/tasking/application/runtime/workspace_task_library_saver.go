package runtime

import (
	"context"
	"fmt"

	librarydomain "ideaboard/api/internal/library/domain"
	libraryports "ideaboard/api/internal/library/ports"
)

// workspaceTaskLibrarySaver persists Task outputs through the managed-save
// use case, keeping Task lineage separate from collection identity.
type workspaceTaskLibrarySaver struct {
	store libraryports.WorkspaceLibraryStore
}

var _ TaskOutputLibrarySaver = (*workspaceTaskLibrarySaver)(nil)

func NewWorkspaceTaskLibrarySaver(store libraryports.WorkspaceLibraryStore) TaskOutputLibrarySaver {
	if store == nil {
		return nil
	}
	return &workspaceTaskLibrarySaver{store: store}
}

func (s *workspaceTaskLibrarySaver) SaveTaskOutputAsset(ctx context.Context, workspacePublicID string, input TaskOutputSaveInput) (*librarydomain.V1Asset, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, fmt.Errorf("workspace task library saver is not configured")
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		return nil, false, err
	}
	return s.store.SaveManagedMediaAsset(ctx, workspacePublicID, librarydomain.V1SaveManagedAssetInput{
		MediaAssetID: input.MediaAssetID,
		Lineage: &librarydomain.TaskLineage{
			TaskID:      input.TaskID,
			ResultIndex: int32(input.ResultIndex),
		},
		OperatorUserID: input.OperatorUserID,
	}, publicID)
}
