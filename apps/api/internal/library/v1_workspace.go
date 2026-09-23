package library

// Workspace-scoped library for the Public API v1 (issue #806).
//
// Additive only: the legacy user-scoped tables (library_folders,
// library_assets keyed by integer ids + owner_user_id) are untouched
// (removal is #809). This file owns a small additive
// WorkspaceLibraryStore interface with context.Context-first methods plus
// an in-memory implementation used by contract tests. A postgres
// implementation (workspace_library_folders/assets/materializations
// tables) is a documented #809 follow-up; until it lands, production
// handlers fail closed with 501 when the store is nil.
//
// Invariants owned here:
//   - External ids mint libf_/liba_/libm_ via contract.MintPublicID (local
//     prefix consts like boards' sht_ pattern since the kernel lacks
//     them); integer database ids are never exposed. v1 lookups reject
//     legacy integer ids as malformed (404, never a distinct status).
//   - Assets are saved external references ONLY: provider URL + source
//     metadata. No bytes are stored, fetched, or proxied here. Asset
//     resolution matches already-saved workspace assets by
//     (provider, source_id) or exact known URL; unknown references report
//     unresolved per item instead of fetching (provider lookup only, NOT
//     a general proxy; live provider fetch needs Control provider config
//     and is #808-owned).
//   - Save is idempotent on (workspace, provider, source_id): re-saving
//     the same reference returns the existing asset (created=false).
//   - Materializations are status-addressable records
//     (pending -> running -> completed|failed); terminal states are
//     immutable. The result links a media asset_... id produced by the
//     caller-owned materializer seam (bytes/storage stay in Media).
//   - No cross-workspace refs: every lookup is keyed by workspace first;
//     foreign-workspace ids miss and read as 404.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/contract"
)

const (
	// V1FolderPrefix mints libf_ folder ids. Exported so sibling
	// contexts (search vision refs) and handlers validate shape through
	// the kernel helper instead of copying the literal.
	V1FolderPrefix = "libf_"
	// V1AssetPrefix mints liba_ asset ids.
	V1AssetPrefix = "liba_"
	// V1MaterializationPrefix mints libm_ materialization ids.
	V1MaterializationPrefix = "libm_"
	// V1MaxResolveItems bounds one typed batch resolve (fail-closed 400
	// above it; the endpoint is a lookup, not a proxy).
	V1MaxResolveItems = 20
)

var (
	// ErrV1FolderNotFound reports an unknown folder within a visible
	// workspace (hidden/cross-workspace/malformed ids converge here).
	ErrV1FolderNotFound = errors.New("library v1 folder not found")
	// ErrV1AssetNotFound reports an unknown asset within a visible
	// workspace.
	ErrV1AssetNotFound = errors.New("library v1 asset not found")
	// ErrV1MaterializationNotFound reports an unknown materialization.
	ErrV1MaterializationNotFound = errors.New("library v1 materialization not found")
	// ErrV1FolderNameConflict reports a duplicate folder name in one
	// workspace.
	ErrV1FolderNameConflict = errors.New("library v1 folder name already exists")
	// ErrV1ResolveTooMany reports a resolve batch above V1MaxResolveItems.
	ErrV1ResolveTooMany = errors.New("library v1 resolve batch too large")
)

// NewV1FolderPublicID mints an opaque libf_ public identifier.
func NewV1FolderPublicID() (string, error) {
	return contract.MintPublicID(V1FolderPrefix, 12)
}

// NewV1AssetPublicID mints an opaque liba_ public identifier.
func NewV1AssetPublicID() (string, error) {
	return contract.MintPublicID(V1AssetPrefix, 12)
}

// NewV1MaterializationPublicID mints an opaque libm_ public identifier.
func NewV1MaterializationPublicID() (string, error) {
	return contract.MintPublicID(V1MaterializationPrefix, 12)
}

// IsV1FolderID reports libf_ shape (opaque URL segment for {folder_id}).
func IsV1FolderID(id string) bool { return contract.IsValidPublicID(V1FolderPrefix, id) }

// IsV1AssetID reports liba_ shape (opaque URL segment for {asset_id}).
func IsV1AssetID(id string) bool { return contract.IsValidPublicID(V1AssetPrefix, id) }

// IsV1MaterializationID reports libm_ shape.
func IsV1MaterializationID(id string) bool {
	return contract.IsValidPublicID(V1MaterializationPrefix, id)
}

// V1Folder is one workspace-scoped collection. Assets without a folder
// are unfiled (FolderPublicID "") rather than forced into a system
// folder; the legacy quick-saves concept stays on the old surface.
type V1Folder struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1Asset is one saved external reference: provider URL + source
// metadata only. It never carries bytes.
type V1Asset struct {
	WorkspacePublicID string
	PublicID          string
	FolderPublicID    string
	Provider          string
	SourceID          string
	SourceURL         string
	PreviewURL        string
	OriginalURL       string
	Title             string
	Author            string
	Width             int32
	Height            int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1MaterializationStatus is the durable-job lifecycle. Records only
// move forward; completed/failed are terminal and immutable.
type V1MaterializationStatus string

const (
	V1MaterializationPending   V1MaterializationStatus = "pending"
	V1MaterializationRunning   V1MaterializationStatus = "running"
	V1MaterializationCompleted V1MaterializationStatus = "completed"
	V1MaterializationFailed    V1MaterializationStatus = "failed"
)

// V1Materialization is one separate durable materialization job: an
// owned library reference turned into a managed Media Asset. Result
// links asset_... on success; failures carry a machine code + message
// for the status poller.
type V1Materialization struct {
	WorkspacePublicID string
	PublicID          string
	AssetPublicID     string
	Status            V1MaterializationStatus
	MediaAssetID      string
	ErrorCode         string
	ErrorMessage      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1SaveAssetInput carries one save: optional folder binding plus the
// provider reference payload (validated like the legacy surface,
// including the provider-allowlisted https URL gate).
type V1SaveAssetInput struct {
	FolderPublicID string
	Asset          AssetInput
}

// V1ResolveItem is one typed batch-resolve entry: provider plus either
// a source_id or an exact known provider URL. Anything else resolves
// to unresolved (never fetched).
type V1ResolveItem struct {
	Provider  string
	SourceID  string
	SourceURL string
}

// V1ResolveResult is one resolved/unresolved outcome. Asset is set only
// for resolved items.
type V1ResolveResult struct {
	Provider string
	SourceID string
	Status   string
	Asset    *V1Asset
}

// WorkspaceLibraryStore is the additive persistence seam for
// workspace-scoped library state. Repository methods take
// context.Context first and use the dbWithContext pattern in the
// postgres sibling (#809); the in-memory sibling below is tests-only.
type WorkspaceLibraryStore interface {
	CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*V1Folder, error)
	ListFolders(ctx context.Context, workspacePublicID string) ([]*V1Folder, error)
	RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*V1Folder, error)
	DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error
	SaveAsset(ctx context.Context, workspacePublicID string, input V1SaveAssetInput, publicID string) (*V1Asset, bool, error)
	ListAssets(ctx context.Context, workspacePublicID string) ([]*V1Asset, error)
	GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*V1Asset, error)
	MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*V1Asset, error)
	DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error
	ResolveAssets(ctx context.Context, workspacePublicID string, items []V1ResolveItem) ([]V1ResolveResult, error)
	CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*V1Materialization, error)
	GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*V1Materialization, error)
	SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*V1Materialization, error)
}

// InMemoryWorkspaceLibraryStore is a mutex-guarded
// WorkspaceLibraryStore for tests ONLY. It must never be wired in
// production.
type InMemoryWorkspaceLibraryStore struct {
	mu               sync.Mutex
	folders          map[string]map[string]*V1Folder
	assets           map[string]map[string]*V1Asset
	assetIndex       map[string]string
	materializations map[string]map[string]*V1Materialization
	now              func() time.Time
}

// NewInMemoryWorkspaceLibraryStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceLibraryStoreForTests() *InMemoryWorkspaceLibraryStore {
	return &InMemoryWorkspaceLibraryStore{
		folders:          make(map[string]map[string]*V1Folder),
		assets:           make(map[string]map[string]*V1Asset),
		assetIndex:       make(map[string]string),
		materializations: make(map[string]map[string]*V1Materialization),
		now:              time.Now,
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
		result := V1ResolveResult{Provider: provider, SourceID: sourceID, Status: "unresolved"}
		if provider != ProviderUnsplash && provider != ProviderPexels {
			results = append(results, result)
			continue
		}
		if sourceID != "" {
			if id, ok := s.assetIndex[v1LibraryAssetDedupKey(workspacePublicID, provider, sourceID)]; ok {
				if asset := s.assets[workspacePublicID][id]; asset != nil {
					result.Status = "resolved"
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
				result.Status = "resolved"
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
	return copyV1Materialization(record), nil
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
