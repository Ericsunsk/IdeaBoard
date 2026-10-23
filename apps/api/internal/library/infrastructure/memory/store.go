package memory

import (
	"errors"
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
