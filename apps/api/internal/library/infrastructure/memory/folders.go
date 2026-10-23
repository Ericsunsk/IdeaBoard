package memory

import (
	"context"
	"sort"
	"strings"

	"ideaboard/api/internal/library/domain"
)

// CreateFolder validates the display name (1..100 runes) and enforces
// per-workspace name uniqueness.
func (s *InMemoryWorkspaceLibraryStore) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed, err := domain.NormalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspacePublicID) == "" || !domain.IsV1FolderID(publicID) {
		return nil, domain.ErrV1FolderNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.folders[workspacePublicID] {
		if strings.EqualFold(existing.Name, trimmed) {
			return nil, domain.ErrV1FolderNameConflict
		}
	}
	now := s.clock()
	folder := &domain.V1Folder{WorkspacePublicID: workspacePublicID, PublicID: publicID, Name: trimmed, CreatedAt: now, UpdatedAt: now}
	if s.folders[workspacePublicID] == nil {
		s.folders[workspacePublicID] = make(map[string]*domain.V1Folder)
	}
	s.folders[workspacePublicID][publicID] = folder
	return copyV1Folder(folder), nil
}

// ListFolders returns folders in creation order (stable for cursor
// pagination at the handler).
func (s *InMemoryWorkspaceLibraryStore) ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*domain.V1Folder, 0, len(s.folders[workspacePublicID]))
	for _, folder := range s.folders[workspacePublicID] {
		all = append(all, copyV1Folder(folder))
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].PublicID < all[j].PublicID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all, nil
}

// RenameFolder keeps name uniqueness; unknown folders 404.
func (s *InMemoryWorkspaceLibraryStore) RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed, err := domain.NormalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	folder := s.folders[workspacePublicID][folderPublicID]
	if folder == nil {
		return nil, domain.ErrV1FolderNotFound
	}
	for id, existing := range s.folders[workspacePublicID] {
		if id != folderPublicID && strings.EqualFold(existing.Name, trimmed) {
			return nil, domain.ErrV1FolderNameConflict
		}
	}
	folder.Name = trimmed
	folder.UpdatedAt = s.clock()
	return copyV1Folder(folder), nil
}

// DeleteFolder drops the folder; contained assets become unfiled rather
// than deleted (documented #809 postgres behavior as well).
func (s *InMemoryWorkspaceLibraryStore) DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.folders[workspacePublicID][folderPublicID] == nil {
		return domain.ErrV1FolderNotFound
	}
	delete(s.folders[workspacePublicID], folderPublicID)
	for _, asset := range s.assets[workspacePublicID] {
		if asset.FolderPublicID == folderPublicID {
			asset.FolderPublicID = ""
			asset.UpdatedAt = s.clock()
		}
	}
	return nil
}
