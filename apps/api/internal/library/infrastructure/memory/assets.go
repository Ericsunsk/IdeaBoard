package memory

import (
	"context"
	"sort"
	"strings"

	"ideaboard/api/internal/library/domain"
)

// SaveAsset validates the provider reference (provider-allowlisted
// https URLs; never bytes) and dedups on
// (workspace, provider, source_id): re-saving returns the existing
// asset with created=false. An unknown folder binding 404s.
func (s *InMemoryWorkspaceLibraryStore) SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	normalized, err := domain.NormalizeAndValidateAsset(input.Asset)
	if err != nil {
		return nil, false, err
	}
	folderID := strings.TrimSpace(input.FolderPublicID)
	if folderID != "" && !domain.IsV1FolderID(folderID) {
		return nil, false, domain.ErrV1FolderNotFound
	}
	if strings.TrimSpace(workspacePublicID) == "" || !domain.IsV1AssetID(publicID) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if folderID != "" && s.folders[workspacePublicID][folderID] == nil {
		return nil, false, domain.ErrV1FolderNotFound
	}
	if existingID, ok := s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, normalized.Provider, normalized.SourceID)]; ok {
		if existing := s.assets[workspacePublicID][existingID]; existing != nil {
			return copyV1Asset(existing), false, nil
		}
	}
	now := s.clock()
	width, height := int32Ptr(normalized.Width), int32Ptr(normalized.Height)
	asset := &domain.V1Asset{
		WorkspacePublicID: workspacePublicID, PublicID: publicID, FolderPublicID: folderID,
		Source:   domain.AssetSourceExternal,
		Provider: normalized.Provider, SourceID: normalized.SourceID,
		SourceURL: normalized.SourceURL, PreviewURL: normalized.PreviewURL, OriginalURL: normalized.OriginalURL,
		Title: normalized.Title, Author: normalized.Author,
		Width: width, Height: height,
		CreatedAt: now, UpdatedAt: now,
	}
	if s.assets[workspacePublicID] == nil {
		s.assets[workspacePublicID] = make(map[string]*domain.V1Asset)
	}
	s.assets[workspacePublicID][publicID] = asset
	s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, normalized.Provider, normalized.SourceID)] = publicID
	return copyV1Asset(asset), true, nil
}

// SaveManagedMediaAsset publishes a test-registered readable Media row.
// Repeated saves preserve the existing Folder and metadata.
func (s *InMemoryWorkspaceLibraryStore) SaveManagedMediaAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	normalized, err := domain.NormalizeAndValidateManagedAsset(input)
	if err != nil {
		return nil, false, err
	}
	if input.OperatorUserID <= 0 {
		return nil, false, errManagedOperatorRequired
	}
	folderID := strings.TrimSpace(input.FolderPublicID)
	if folderID != "" && !domain.IsV1FolderID(folderID) {
		return nil, false, domain.ErrV1FolderNotFound
	}
	if strings.TrimSpace(workspacePublicID) == "" || !domain.IsV1AssetID(publicID) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	media := s.managedMedia[workspacePublicID+"\x00"+normalized.MediaAssetID]
	if media == nil || media.archived {
		return nil, false, domain.ErrV1AssetNotFound
	}
	if folderID != "" && s.folders[workspacePublicID][folderID] == nil {
		return nil, false, domain.ErrV1FolderNotFound
	}
	if existingID, ok := s.mediaIndex[v1LibraryManagedDedupKey(workspacePublicID, normalized.MediaAssetID)]; ok {
		if existing := s.assets[workspacePublicID][existingID]; existing != nil {
			return copyV1Asset(existing), false, nil
		}
	}
	now := s.clock()
	asset := &domain.V1Asset{
		WorkspacePublicID: workspacePublicID, PublicID: publicID, FolderPublicID: folderID,
		Source: domain.AssetSourceMedia, MediaAssetID: normalized.MediaAssetID,
		Title: normalized.Title, Author: normalized.Author,
		Width: media.width, Height: media.height,
		Lineage:   normalized.Lineage,
		CreatedAt: now, UpdatedAt: now,
	}
	if s.assets[workspacePublicID] == nil {
		s.assets[workspacePublicID] = make(map[string]*domain.V1Asset)
	}
	s.assets[workspacePublicID][publicID] = asset
	s.mediaIndex[v1LibraryManagedDedupKey(workspacePublicID, normalized.MediaAssetID)] = publicID
	return copyV1Asset(asset), true, nil
}

// ListAssets returns every workspace asset in creation order; the
// handler slices the stable page (same posture as boards listBoards).
func (s *InMemoryWorkspaceLibraryStore) ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*domain.V1Asset, 0, len(s.assets[workspacePublicID]))
	for _, asset := range s.assets[workspacePublicID] {
		all = append(all, copyV1Asset(asset))
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].PublicID < all[j].PublicID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all, nil
}

// GetAsset converges unknown ids to domain.ErrV1AssetNotFound (cross-workspace
// ids miss the workspace-keyed map by construction).
func (s *InMemoryWorkspaceLibraryStore) GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[workspacePublicID][assetPublicID]
	if asset == nil {
		return nil, domain.ErrV1AssetNotFound
	}
	return copyV1Asset(asset), nil
}

// MoveAsset rebinds the folder ("" unfiles); unknown assets or folders
// 404 with their own codes.
func (s *InMemoryWorkspaceLibraryStore) MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	folderID := strings.TrimSpace(folderPublicID)
	if folderID != "" && !domain.IsV1FolderID(folderID) {
		return nil, domain.ErrV1FolderNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[workspacePublicID][assetPublicID]
	if asset == nil {
		return nil, domain.ErrV1AssetNotFound
	}
	if folderID != "" && s.folders[workspacePublicID][folderID] == nil {
		return nil, domain.ErrV1FolderNotFound
	}
	asset.FolderPublicID = folderID
	asset.UpdatedAt = s.clock()
	return copyV1Asset(asset), nil
}

// DeleteAsset drops the saved reference (the provider original is
// untouched; materialized Media Assets are owned by Media, not here).
func (s *InMemoryWorkspaceLibraryStore) DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[workspacePublicID][assetPublicID]
	if asset == nil {
		return domain.ErrV1AssetNotFound
	}
	delete(s.assets[workspacePublicID], assetPublicID)
	delete(s.assetIndex, v1LibraryAssetDedupKey(workspacePublicID, asset.Provider, asset.SourceID))
	if asset.Source == domain.AssetSourceMedia && asset.MediaAssetID != "" {
		delete(s.mediaIndex, v1LibraryManagedDedupKey(workspacePublicID, asset.MediaAssetID))
	}
	return nil
}

// ResolveAssets performs the typed batch resolve (bounded to
// domain.V1MaxResolveItems): provider + source_id matches a saved workspace
// asset; a bare provider URL must be an allowlisted https URL and match
// a saved asset's known URL exactly. Nothing is fetched: unknown
// references report unresolved per item. NOT a general proxy.
func (s *InMemoryWorkspaceLibraryStore) ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(items) > domain.V1MaxResolveItems {
		return nil, domain.ErrV1ResolveTooMany
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make([]domain.V1ResolveResult, 0, len(items))
	for _, item := range items {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		sourceID := strings.TrimSpace(item.SourceID)
		sourceURL := strings.TrimSpace(item.SourceURL)
		result := domain.V1ResolveResult{Provider: provider, SourceID: sourceID, Status: domain.V1ResolveUnresolved}
		if provider != domain.ProviderUnsplash && provider != domain.ProviderPexels {
			results = append(results, result)
			continue
		}
		if sourceID != "" {
			if id, ok := s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, provider, sourceID)]; ok {
				if asset := s.assets[workspacePublicID][id]; asset != nil {
					result.Status = domain.V1ResolveResolved
					result.Asset = copyV1Asset(asset)
				}
			}
			results = append(results, result)
			continue
		}
		if sourceURL == "" || !domain.IsAllowedProviderURL(provider, sourceURL) {
			results = append(results, result)
			continue
		}
		for _, asset := range s.assets[workspacePublicID] {
			if asset.Provider != provider {
				continue
			}
			if asset.SourceURL == sourceURL || asset.PreviewURL == sourceURL || asset.OriginalURL == sourceURL {
				result.Status = domain.V1ResolveResolved
				result.SourceID = asset.SourceID
				result.Asset = copyV1Asset(asset)
				break
			}
		}
		results = append(results, result)
	}
	return results, nil
}
