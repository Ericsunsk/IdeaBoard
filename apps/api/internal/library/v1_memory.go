package library

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/library/domain"
)

// InMemoryWorkspaceLibraryStore is a mutex-guarded
// WorkspaceLibraryStore for tests ONLY. It must never be wired in
// production.
type InMemoryWorkspaceLibraryStore struct {
	mu               sync.Mutex
	folders          map[string]map[string]*V1Folder
	assets           map[string]map[string]*V1Asset
	assetIndex       map[string]string
	materializations map[string]map[string]*V1Materialization
	// materializationOrder keeps per-workspace job ids in creation
	// order (postgres parity for ListMaterializations; map iteration
	// alone cannot reproduce it when CreatedAt ties).
	materializationOrder map[string][]string
	now                  func() time.Time
}

// NewInMemoryWorkspaceLibraryStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceLibraryStoreForTests() *InMemoryWorkspaceLibraryStore {
	return &InMemoryWorkspaceLibraryStore{
		folders:              make(map[string]map[string]*V1Folder),
		assets:               make(map[string]map[string]*V1Asset),
		assetIndex:           make(map[string]string),
		materializations:     make(map[string]map[string]*V1Materialization),
		materializationOrder: make(map[string][]string),
		now:                  time.Now,
	}
}

func (s *InMemoryWorkspaceLibraryStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func v1LibraryAssetDedupKey(workspacePublicID, provider, sourceID string) string {
	return workspacePublicID + "\x00" + provider + "\x00" + sourceID
}

func copyV1Folder(folder *V1Folder) *V1Folder {
	if folder == nil {
		return nil
	}
	out := *folder
	return &out
}

func copyV1Asset(asset *V1Asset) *V1Asset {
	if asset == nil {
		return nil
	}
	out := *asset
	return &out
}

func copyV1Materialization(record *V1Materialization) *V1Materialization {
	if record == nil {
		return nil
	}
	out := *record
	return &out
}

// CreateFolder validates the display name (1..100 runes, same bound as
// the legacy surface) and enforces per-workspace name uniqueness.
func (s *InMemoryWorkspaceLibraryStore) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed, err := normalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspacePublicID) == "" || !IsV1FolderID(publicID) {
		return nil, ErrV1FolderNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.folders[workspacePublicID] {
		if strings.EqualFold(existing.Name, trimmed) {
			return nil, ErrV1FolderNameConflict
		}
	}
	now := s.clock()
	folder := &V1Folder{WorkspacePublicID: workspacePublicID, PublicID: publicID, Name: trimmed, CreatedAt: now, UpdatedAt: now}
	if s.folders[workspacePublicID] == nil {
		s.folders[workspacePublicID] = make(map[string]*V1Folder)
	}
	s.folders[workspacePublicID][publicID] = folder
	return copyV1Folder(folder), nil
}

// ListFolders returns folders in creation order (stable for cursor
// pagination at the handler).
func (s *InMemoryWorkspaceLibraryStore) ListFolders(ctx context.Context, workspacePublicID string) ([]*V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*V1Folder, 0, len(s.folders[workspacePublicID]))
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
func (s *InMemoryWorkspaceLibraryStore) RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed, err := normalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	folder := s.folders[workspacePublicID][folderPublicID]
	if folder == nil {
		return nil, ErrV1FolderNotFound
	}
	for id, existing := range s.folders[workspacePublicID] {
		if id != folderPublicID && strings.EqualFold(existing.Name, trimmed) {
			return nil, ErrV1FolderNameConflict
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
		return ErrV1FolderNotFound
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

// SaveAsset validates the provider reference (provider-allowlisted
// https URLs; never bytes) and dedups on
// (workspace, provider, source_id): re-saving returns the existing
// asset with created=false. An unknown folder binding 404s.
func (s *InMemoryWorkspaceLibraryStore) SaveAsset(ctx context.Context, workspacePublicID string, input V1SaveAssetInput, publicID string) (*V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	normalized, err := normalizeAndValidateAsset(input.Asset)
	if err != nil {
		return nil, false, err
	}
	folderID := strings.TrimSpace(input.FolderPublicID)
	if folderID != "" && !IsV1FolderID(folderID) {
		return nil, false, ErrV1FolderNotFound
	}
	if strings.TrimSpace(workspacePublicID) == "" || !IsV1AssetID(publicID) {
		return nil, false, ErrV1AssetNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if folderID != "" && s.folders[workspacePublicID][folderID] == nil {
		return nil, false, ErrV1FolderNotFound
	}
	if existingID, ok := s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, normalized.Provider, normalized.SourceID)]; ok {
		if existing := s.assets[workspacePublicID][existingID]; existing != nil {
			return copyV1Asset(existing), false, nil
		}
	}
	now := s.clock()
	asset := &V1Asset{
		WorkspacePublicID: workspacePublicID, PublicID: publicID, FolderPublicID: folderID,
		Provider: normalized.Provider, SourceID: normalized.SourceID,
		SourceURL: normalized.SourceURL, PreviewURL: normalized.PreviewURL, OriginalURL: normalized.OriginalURL,
		Title: normalized.Title, Author: normalized.Author,
		Width: normalized.Width, Height: normalized.Height,
		CreatedAt: now, UpdatedAt: now,
	}
	if s.assets[workspacePublicID] == nil {
		s.assets[workspacePublicID] = make(map[string]*V1Asset)
	}
	s.assets[workspacePublicID][publicID] = asset
	s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, normalized.Provider, normalized.SourceID)] = publicID
	return copyV1Asset(asset), true, nil
}

// ListAssets returns every workspace asset in creation order; the
// handler slices the stable page (same posture as boards listBoards).
func (s *InMemoryWorkspaceLibraryStore) ListAssets(ctx context.Context, workspacePublicID string) ([]*V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*V1Asset, 0, len(s.assets[workspacePublicID]))
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

// GetAsset converges unknown ids to ErrV1AssetNotFound (cross-workspace
// ids miss the workspace-keyed map by construction).
func (s *InMemoryWorkspaceLibraryStore) GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[workspacePublicID][assetPublicID]
	if asset == nil {
		return nil, ErrV1AssetNotFound
	}
	return copyV1Asset(asset), nil
}

// MoveAsset rebinds the folder ("" unfiles); unknown assets or folders
// 404 with their own codes.
func (s *InMemoryWorkspaceLibraryStore) MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	folderID := strings.TrimSpace(folderPublicID)
	if folderID != "" && !IsV1FolderID(folderID) {
		return nil, ErrV1FolderNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[workspacePublicID][assetPublicID]
	if asset == nil {
		return nil, ErrV1AssetNotFound
	}
	if folderID != "" && s.folders[workspacePublicID][folderID] == nil {
		return nil, ErrV1FolderNotFound
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
		return ErrV1AssetNotFound
	}
	delete(s.assets[workspacePublicID], assetPublicID)
	delete(s.assetIndex, v1LibraryAssetDedupKey(workspacePublicID, asset.Provider, asset.SourceID))
	return nil
}

// ResolveAssets performs the typed batch resolve (bounded to
// V1MaxResolveItems): provider + source_id matches a saved workspace
// asset; a bare provider URL must be an allowlisted https URL and match
// a saved asset's known URL exactly. Nothing is fetched: unknown
// references report unresolved per item. NOT a general proxy.
func (s *InMemoryWorkspaceLibraryStore) ResolveAssets(ctx context.Context, workspacePublicID string, items []V1ResolveItem) ([]V1ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(items) > V1MaxResolveItems {
		return nil, ErrV1ResolveTooMany
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make([]V1ResolveResult, 0, len(items))
	for _, item := range items {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		sourceID := strings.TrimSpace(item.SourceID)
		sourceURL := strings.TrimSpace(item.SourceURL)
		result := V1ResolveResult{Provider: provider, SourceID: sourceID, Status: domain.V1ResolveUnresolved}
		if provider != ProviderUnsplash && provider != ProviderPexels {
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
		if sourceURL == "" || !isAllowedProviderURL(provider, sourceURL) {
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

// CreateMaterialization opens a pending job for an owned asset. The
// caller drives pending -> running -> completed|failed through
// SetMaterializationResult as the separate durable job settles.
func (s *InMemoryWorkspaceLibraryStore) CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !IsV1MaterializationID(publicID) {
		return nil, ErrV1MaterializationNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[workspacePublicID][assetPublicID] == nil {
		return nil, ErrV1AssetNotFound
	}
	now := s.clock()
	record := &V1Materialization{
		WorkspacePublicID: workspacePublicID, PublicID: publicID,
		AssetPublicID: assetPublicID, Status: V1MaterializationPending,
		CreatedAt: now, UpdatedAt: now,
	}
	if s.materializations[workspacePublicID] == nil {
		s.materializations[workspacePublicID] = make(map[string]*V1Materialization)
	}
	s.materializations[workspacePublicID][publicID] = record
	s.materializationOrder[workspacePublicID] = append(s.materializationOrder[workspacePublicID], publicID)
	return copyV1Materialization(record), nil
}

// ClaimMaterialization atomically converges internal triggers in the test
// store, matching the postgres advisory-lock implementation.
func (s *InMemoryWorkspaceLibraryStore) ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*V1Materialization, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !IsV1MaterializationID(publicID) {
		return nil, false, ErrV1MaterializationNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[workspacePublicID][assetPublicID] == nil {
		return nil, false, ErrV1AssetNotFound
	}
	for _, id := range s.materializationOrder[workspacePublicID] {
		record := s.materializations[workspacePublicID][id]
		if record == nil || record.AssetPublicID != assetPublicID {
			continue
		}
		if record.Status == V1MaterializationPending || record.Status == V1MaterializationRunning || record.Status == V1MaterializationCompleted {
			return copyV1Materialization(record), false, nil
		}
	}
	now := s.clock()
	record := &V1Materialization{WorkspacePublicID: workspacePublicID, PublicID: publicID, AssetPublicID: assetPublicID, Status: V1MaterializationPending, CreatedAt: now, UpdatedAt: now}
	if s.materializations[workspacePublicID] == nil {
		s.materializations[workspacePublicID] = make(map[string]*V1Materialization)
	}
	s.materializations[workspacePublicID][publicID] = record
	s.materializationOrder[workspacePublicID] = append(s.materializationOrder[workspacePublicID], publicID)
	return copyV1Materialization(record), true, nil
}

// GetMaterialization returns the status-addressable job snapshot.
func (s *InMemoryWorkspaceLibraryStore) GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.materializations[workspacePublicID][materializationPublicID]
	if record == nil {
		return nil, ErrV1MaterializationNotFound
	}
	return copyV1Materialization(record), nil
}

// ListMaterializations returns every workspace job in creation order
// (concrete-store parity for the read-through completed-job scan and
// trigger dedup; unknown workspaces list empty).
func (s *InMemoryWorkspaceLibraryStore) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order := s.materializationOrder[workspacePublicID]
	all := make([]*V1Materialization, 0, len(order))
	for _, publicID := range order {
		if record := s.materializations[workspacePublicID][publicID]; record != nil {
			all = append(all, copyV1Materialization(record))
		}
	}
	return all, nil
}

// SetMaterializationResult advances pending|running jobs. Terminal
// records are immutable: repeat calls return the stored snapshot so
// idempotent replays converge instead of resurrecting jobs.
func (s *InMemoryWorkspaceLibraryStore) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.materializations[workspacePublicID][materializationPublicID]
	if record == nil {
		return nil, ErrV1MaterializationNotFound
	}
	if record.Status == V1MaterializationCompleted || record.Status == V1MaterializationFailed {
		return copyV1Materialization(record), nil
	}
	switch status {
	case V1MaterializationRunning, V1MaterializationCompleted, V1MaterializationFailed:
		record.Status = status
	default:
		return nil, ErrV1MaterializationNotFound
	}
	record.MediaAssetID = strings.TrimSpace(mediaAssetID)
	record.ErrorCode = strings.TrimSpace(errorCode)
	record.ErrorMessage = strings.TrimSpace(errorMessage)
	record.UpdatedAt = s.clock()
	return copyV1Materialization(record), nil
}
