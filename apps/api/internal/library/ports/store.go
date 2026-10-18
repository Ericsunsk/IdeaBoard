// Package ports owns the library persistence seam. Repository methods take
// context.Context first; postgres siblings use the dbWithContext pattern.
// Runtime composition, HTTP handlers, and Task auto-save consume this seam.
package ports

import (
	"context"

	"ideaboard/api/internal/library/domain"

	"github.com/jackc/pgx/v5"
)

// WorkspaceLibraryStore persists workspace-scoped library state.
// The postgres implementation lives in infrastructure/postgres;
// infrastructure/memory is tests-only and must never be wired in production.
// The v1 handler does NOT depend on this interface directly (see V1Store
// below); postgres keeps implementing the full seam.
type WorkspaceLibraryStore interface {
	CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error)
	ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error)
	RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error)
	DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error
	SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error)
	// SaveManagedMediaAsset explicitly publishes readable Media into Workspace Library.
	SaveManagedMediaAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error)
	// SaveTaskOutput accepts an automatic publication of a persisted Task output.
	SaveTaskOutput(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error)
	ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error)
	GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error)
	MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error)
	DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error
	ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error)
	CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, error)
	GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*domain.V1Materialization, error)
	SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error)
}

// V1Store is the narrow library seam for the v1 handler
// (ContractV1LibraryDeps.Store and ContractV1SearchDeps.Library). It lists
// exactly the methods the v1 library/search handlers call (verified
// against internal/runtime/interfaces/http/v1/library.go + search.go):
// folder CRUD, SaveAsset/SaveManagedMediaAsset, List/Get/Move/Delete
// assets, ResolveAssets. Search only needs GetAsset (subset, covered).
//
// Excluded with reason:
//   - SaveTaskOutput: production caller is the task auto-saver
//     (tasking/application/runtime), retained on the in-memory double.
//   - CreateMaterialization/GetMaterialization: NO production caller
//     (production claims via ClaimMaterialization); deleted from the
//     in-memory double, retained on postgres (big seam unchanged).
//   - SetMaterializationResult/ClaimMaterialization/ListMaterializations:
//     production callers are the read-through orchestrator + source
//     resolver; retained on the in-memory double.
type V1Store interface {
	CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error)
	ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error)
	RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error)
	DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error
	SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error)
	SaveManagedMediaAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error)
	ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error)
	GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error)
	MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error)
	DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error
	ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error)
}

// MaterializationClaimer atomically converges concurrent internal triggers
// for one workspace asset onto one pending/running/completed job.
type MaterializationClaimer interface {
	ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error)
}

// PublicationAuthorizer evaluates publication on the Library accepting transaction.
type PublicationAuthorizer interface {
	AuthorizeManual(context.Context, pgx.Tx, string, domain.V1SaveManagedAssetInput) error
	AuthorizeAutomatic(context.Context, pgx.Tx, string, domain.V1SaveManagedAssetInput) (bool, error)
}
