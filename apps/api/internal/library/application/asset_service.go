package application

import (
	"context"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/ports"
)

// ManagedAssetStore extends the seam with the concrete task-output
// reference operation. Only the postgres Service implements it; the
// seam itself stays frozen.
type ManagedAssetStore interface {
	ports.WorkspaceLibraryStore
	SaveManagedMediaAsset(ctx context.Context, workspacePublicID, sourceID, mediaAssetID, publicID string) (*domain.V1Asset, bool, error)
}

// AssetService orchestrates saved-reference management: idempotent
// saves, folder bindings, and typed batch resolves.
type AssetService struct {
	store ManagedAssetStore
}

// NewAssetService builds an AssetService over store.
func NewAssetService(store ManagedAssetStore) *AssetService {
	return &AssetService{store: store}
}

func (s *AssetService) SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	return s.store.SaveAsset(ctx, workspacePublicID, input, publicID)
}

func (s *AssetService) SaveManagedMediaAsset(ctx context.Context, workspacePublicID, sourceID, mediaAssetID, publicID string) (*domain.V1Asset, bool, error) {
	return s.store.SaveManagedMediaAsset(ctx, workspacePublicID, sourceID, mediaAssetID, publicID)
}

func (s *AssetService) ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error) {
	return s.store.ListAssets(ctx, workspacePublicID)
}

func (s *AssetService) GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error) {
	return s.store.GetAsset(ctx, workspacePublicID, assetPublicID)
}

func (s *AssetService) MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error) {
	return s.store.MoveAsset(ctx, workspacePublicID, assetPublicID, folderPublicID)
}

func (s *AssetService) DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error {
	return s.store.DeleteAsset(ctx, workspacePublicID, assetPublicID)
}

func (s *AssetService) ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error) {
	return s.store.ResolveAssets(ctx, workspacePublicID, items)
}
