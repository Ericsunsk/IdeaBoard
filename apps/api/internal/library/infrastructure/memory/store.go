package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/ports"
)

// errManagedOperatorRequired fails managed saves without a server-side
// operator (a wiring bug, never a client input error).
var errManagedOperatorRequired = errors.New("library: managed save operator required")

// InMemoryWorkspaceLibraryStore is a mutex-guarded
// WorkspaceLibraryStore for tests ONLY. It must never be wired in
// production.
type InMemoryWorkspaceLibraryStore struct {
	mu               sync.Mutex
	folders          map[string]map[string]*domain.V1Folder
	assets           map[string]map[string]*domain.V1Asset
	assetIndex       map[string]string
	mediaIndex       map[string]string
	materializations map[string]map[string]*domain.V1Materialization
	// managedMedia is the test-only Media readability registry backing
	// the media branch of SaveManagedMediaAsset: tests register the
	// managed rows the store may link (workspace-scoped, archived rows
	// stay unlinkable). Production validation lives in postgres.
	managedMedia map[string]*managedMediaRecord
	// materializationOrder keeps per-workspace job ids in creation
	// order (postgres parity for ListMaterializations; map iteration
	// alone cannot reproduce it when CreatedAt ties).
	materializationOrder map[string][]string
	now                  func() time.Time
}

// managedMediaRecord is one linkable managed row for tests.
type managedMediaRecord struct {
	archived bool
	width    *int32
	height   *int32
}

// RegisterManagedMediaForTests registers a linkable managed row:
// dimensions pair like the read projection (both set or both nil),
// archived rows fail saves like production.
func (s *InMemoryWorkspaceLibraryStore) RegisterManagedMediaForTests(workspacePublicID, mediaPublicID string, width, height *int32, archived bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managedMedia == nil {
		s.managedMedia = make(map[string]*managedMediaRecord)
	}
	w, h := domain.PairDimensions(width, height)
	s.managedMedia[workspacePublicID+"\x00"+mediaPublicID] = &managedMediaRecord{
		archived: archived, width: w, height: h,
	}
}

var _ ports.V1Store = (*InMemoryWorkspaceLibraryStore)(nil)
var _ ports.MaterializationClaimer = (*InMemoryWorkspaceLibraryStore)(nil)

// NewInMemoryWorkspaceLibraryStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceLibraryStoreForTests() *InMemoryWorkspaceLibraryStore {
	return &InMemoryWorkspaceLibraryStore{
		folders:              make(map[string]map[string]*domain.V1Folder),
		assets:               make(map[string]map[string]*domain.V1Asset),
		assetIndex:           make(map[string]string),
		mediaIndex:           make(map[string]string),
		managedMedia:         make(map[string]*managedMediaRecord),
		materializations:     make(map[string]map[string]*domain.V1Materialization),
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

func v1LibraryManagedDedupKey(workspacePublicID, mediaPublicID string) string {
	return workspacePublicID + "\x00\x00" + mediaPublicID
}

func int32Ptr(v int32) *int32 { return &v }

func copyV1Folder(folder *domain.V1Folder) *domain.V1Folder {
	if folder == nil {
		return nil
	}
	out := *folder
	return &out
}

func copyV1Asset(asset *domain.V1Asset) *domain.V1Asset {
	if asset == nil {
		return nil
	}
	out := *asset
	if asset.Width != nil {
		w := *asset.Width
		out.Width = &w
	}
	if asset.Height != nil {
		h := *asset.Height
		out.Height = &h
	}
	if asset.Lineage != nil {
		lineage := *asset.Lineage
		out.Lineage = &lineage
	}
	return &out
}

func copyV1Materialization(record *domain.V1Materialization) *domain.V1Materialization {
	if record == nil {
		return nil
	}
	out := *record
	return &out
}

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

// SaveTaskOutput publishes a test-registered output with explicit Task lineage.
func (s *InMemoryWorkspaceLibraryStore) SaveTaskOutput(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	if input.Lineage == nil {
		return nil, false, domain.ErrV1AssetNotFound
	}
	return s.SaveManagedMediaAsset(ctx, workspacePublicID, input, publicID)
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

// ClaimMaterialization atomically converges internal triggers in the test
// store, matching the postgres advisory-lock implementation.
func (s *InMemoryWorkspaceLibraryStore) ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !domain.IsV1MaterializationID(publicID) {
		return nil, false, domain.ErrV1MaterializationNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[workspacePublicID][assetPublicID] == nil {
		return nil, false, domain.ErrV1AssetNotFound
	}
	for _, id := range s.materializationOrder[workspacePublicID] {
		record := s.materializations[workspacePublicID][id]
		if record == nil || record.AssetPublicID != assetPublicID {
			continue
		}
		if record.Status == domain.V1MaterializationPending || record.Status == domain.V1MaterializationRunning || record.Status == domain.V1MaterializationCompleted {
			return copyV1Materialization(record), false, nil
		}
	}
	now := s.clock()
	record := &domain.V1Materialization{WorkspacePublicID: workspacePublicID, PublicID: publicID, AssetPublicID: assetPublicID, Status: domain.V1MaterializationPending, CreatedAt: now, UpdatedAt: now}
	if s.materializations[workspacePublicID] == nil {
		s.materializations[workspacePublicID] = make(map[string]*domain.V1Materialization)
	}
	s.materializations[workspacePublicID][publicID] = record
	s.materializationOrder[workspacePublicID] = append(s.materializationOrder[workspacePublicID], publicID)
	return copyV1Materialization(record), true, nil
}

// ListMaterializations returns every workspace job in creation order
// (concrete-store parity for the read-through completed-job scan and
// trigger dedup; unknown workspaces list empty).
func (s *InMemoryWorkspaceLibraryStore) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order := s.materializationOrder[workspacePublicID]
	all := make([]*domain.V1Materialization, 0, len(order))
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
func (s *InMemoryWorkspaceLibraryStore) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.materializations[workspacePublicID][materializationPublicID]
	if record == nil {
		return nil, domain.ErrV1MaterializationNotFound
	}
	if record.Status == domain.V1MaterializationCompleted || record.Status == domain.V1MaterializationFailed {
		return copyV1Materialization(record), nil
	}
	switch status {
	case domain.V1MaterializationRunning, domain.V1MaterializationCompleted, domain.V1MaterializationFailed:
		record.Status = status
	default:
		return nil, domain.ErrV1MaterializationNotFound
	}
	record.MediaAssetID = strings.TrimSpace(mediaAssetID)
	record.ErrorCode = strings.TrimSpace(errorCode)
	record.ErrorMessage = strings.TrimSpace(errorMessage)
	record.UpdatedAt = s.clock()
	return copyV1Materialization(record), nil
}
