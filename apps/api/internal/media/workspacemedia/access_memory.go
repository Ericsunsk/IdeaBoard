package workspacemedia

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// In-memory asset authorization twin (#936, tests only). Mirrors
// authorizeLoadedAsset in access_postgres.go outcome-for-outcome: the same
// source order (owner, library, task, upload-target board, referencing
// boards), the same ErrNotFound denials, the same live narrow-port board
// checks. Two documented twin notes:
//
//   - Library links have no library tables in this store; tests seed them
//     via SeedLibraryLinkForTests (the postgres twin reads library_assets).
//   - CreateSession stores the board binding without existence validation
//     (the postgres twin resolves the board row at write). Forged bindings
//     still authorize nothing: the read-time role check fails closed on
//     unknown boards, and the HTTP layer verifies board write access
//     before creating bound sessions.

type taskVisibility struct {
	submitterUserID int64
	boardID         string
}

// SeedLibraryLinkForTests marks one asset as backing a published Workspace
// Library row (twin of the library_assets existence fact).
func (s *InMemoryWorkspaceMediaStore) SeedLibraryLinkForTests(workspaceID, assetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.libraryLinks[mediaKey(workspaceID, assetID)] = struct{}{}
}

// authorizeLoadedLocked authorizes one loaded asset.
// The caller holds s.mu.
func (s *InMemoryWorkspaceMediaStore) authorizeLoadedLocked(
	ctx context.Context,
	workspacePublicID string,
	asset *Asset,
	callerUserID int64,
	workspaceRole string,
	access BoardAccess,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if asset == nil {
		return ErrNotFound
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return err
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	assetKey := mediaKey(trimmedWorkspace, asset.ID)
	if callerUserID > 0 && asset.OwnerUserID == callerUserID {
		return nil
	}
	if _, ok := s.libraryLinks[assetKey]; ok {
		return nil
	}
	prefix := trimmedWorkspace + "\x00"
	for key, items := range s.taskAssets {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		visibility := s.taskVisibility[key]
		if visibility.submitterUserID != callerUserID && !authorizeBoardRead(ctx, access, trimmedWorkspace, visibility.boardID, callerUserID, workspaceRole) {
			continue
		}
		for _, item := range items {
			if item.Asset != nil && item.Asset.ID == asset.ID {
				return nil
			}
		}
	}
	if target := strings.TrimSpace(s.assetTargets[assetKey]); target != "" {
		if authorizeBoardRead(ctx, access, trimmedWorkspace, target, callerUserID, workspaceRole) {
			return nil
		}
	}
	if access != nil {
		referencing, err := access.ListBoardsReferencingAsset(ctx, trimmedWorkspace, asset.ID)
		if err != nil {
			return err
		}
		for _, boardPublicID := range referencing {
			if authorizeBoardRead(ctx, access, trimmedWorkspace, boardPublicID, callerUserID, workspaceRole) {
				return nil
			}
		}
	}
	return ErrNotFound
}

// AuthorizeAsset resolves one workspace-scoped asset for a caller (see
// access.go for the source order). Denials report ErrNotFound.
func (s *InMemoryWorkspaceMediaStore) AuthorizeAsset(ctx context.Context, workspaceID, assetID string, callerUserID int64, workspaceRole string, access BoardAccess) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[mediaKey(workspaceID, assetID)]
	if asset == nil {
		return nil, ErrNotFound
	}
	if err := s.authorizeLoadedLocked(ctx, workspaceID, asset, callerUserID, workspaceRole, access); err != nil {
		return nil, err
	}
	return copyAsset(asset), nil
}

// ListAuthorizedAssets pages the caller's readable assets newest-first.
// Offset counts AUTHORIZED items: filtering happens before pagination.
func (s *InMemoryWorkspaceMediaStore) ListAuthorizedAssets(ctx context.Context, workspaceID string, callerUserID int64, workspaceRole string, access BoardAccess, limit, offset int) ([]*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	prefix := trimmedWorkspace + "\x00"
	all := make([]*Asset, 0)
	for key, asset := range s.assets {
		if !strings.HasPrefix(key, prefix) || asset.IsArchived() {
			continue
		}
		if err := s.authorizeLoadedLocked(ctx, trimmedWorkspace, asset, callerUserID, workspaceRole, access); err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		all = append(all, copyAsset(asset))
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	if offset >= len(all) {
		return []*Asset{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}
