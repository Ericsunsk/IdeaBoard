// Package ports owns the library persistence seam. Repository methods take
// context.Context first; postgres siblings use the dbWithContext pattern.
// Runtime composition, HTTP handlers, and Task auto-save consume this seam.
package ports

import (
	"context"

	"ideaboard/api/internal/library/domain"
)

// WorkspaceLibraryStore persists workspace-scoped library state.
// The postgres implementation lives in infrastructure/postgres;
// infrastructure/memory is tests-only and must never be wired in production.
type WorkspaceLibraryStore interface {
	CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error)
	ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error)
	RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error)
	DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error
	SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error)
	// SaveManagedMediaAsset is the single managed-save use case: manual
	// and task-auto entries converge here. Dedup identity is
	// (workspace, media asset); repeats return the existing row with
	// created=false without touching its folder or metadata.
	SaveManagedMediaAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error)
	ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error)
	GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error)
	MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error)
	DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error
	ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error)
	CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, error)
	GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*domain.V1Materialization, error)
	SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error)
}

// MaterializationClaimer atomically converges concurrent internal triggers
// for one workspace asset onto one pending/running/completed job.
type MaterializationClaimer interface {
	ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error)
}
