// Package ports owns the library persistence seam. Repository methods take
// context.Context first; postgres siblings use the dbWithContext pattern.
// Signatures are frozen: runtime composition, the v1 handlers, and the
// tasking materializer depend on them, so renames need a migration of
// every caller first.
package ports

import (
	"context"

	"ideaboard/api/internal/library/domain"
)

// WorkspaceLibraryStore is the additive persistence seam for
// workspace-scoped library state. The postgres implementation lives in
// infrastructure/postgres; the in-memory sibling in the library root
// package is tests-only and must never be wired in production.
type WorkspaceLibraryStore interface {
	CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error)
	ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error)
	RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error)
	DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error
	SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error)
	ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error)
	GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error)
	MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error)
	DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error
	ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error)
	CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, error)
	GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*domain.V1Materialization, error)
	SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error)
}
