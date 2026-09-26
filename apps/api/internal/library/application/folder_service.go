// Package application owns the per-aggregate orchestration entry points
// over the ports seam. The services hold no SQL and no transactions:
// folder/asset/materialization durability, tx boundaries (DeleteFolder,
// dedup replay convergence), and the materialization lifecycle machine
// stay behavior-identical inside infrastructure/postgres. Handlers keep
// calling the WorkspaceLibraryStore seam; these services give future
// orchestration (validation-first flows, cross-aggregate coordination) a
// home that does not reopen the postgres aggregate files.
package application

import (
	"context"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/ports"
)

// FolderService orchestrates folder collection management.
type FolderService struct {
	store ports.WorkspaceLibraryStore
}

// NewFolderService builds a FolderService over store.
func NewFolderService(store ports.WorkspaceLibraryStore) *FolderService {
	return &FolderService{store: store}
}

func (s *FolderService) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error) {
	return s.store.CreateFolder(ctx, workspacePublicID, name, publicID)
}

func (s *FolderService) ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error) {
	return s.store.ListFolders(ctx, workspacePublicID)
}

func (s *FolderService) RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error) {
	return s.store.RenameFolder(ctx, workspacePublicID, folderPublicID, name)
}

func (s *FolderService) DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error {
	return s.store.DeleteFolder(ctx, workspacePublicID, folderPublicID)
}
