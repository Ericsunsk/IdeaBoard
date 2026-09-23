package workspaceuploads

import (
	"context"
	"sync"
	"time"
)

// SessionStore is the ephemeral upload-session persistence seam. The
// process-local in-memory implementation is the production backend too:
// sessions coordinate transfer only and must never become durable rows
// (completion-only durability: only complete creates a Media Asset row).
type SessionStore interface {
	CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error)
	// GetSession applies lazy lifecycle transitions (pending ->
	// transferring on first read, pending/transferring -> expired past
	// the TTL) and returns ErrNotFound for cross-workspace ids so
	// sessions stay invisible outside their workspace.
	GetSession(ctx context.Context, workspaceID, uploadID string) (*Session, error)
	// CompleteSession marks the session completed with its asset. It
	// fails with ErrStateConflict outside pending/transferring.
	CompleteSession(ctx context.Context, workspaceID, uploadID, assetID string) (*Session, error)
	// FailSession marks the session failed with a server-side reason.
	FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error)
}

// Asset is the workspace-bound media asset projection. InternalID,
// StorageProfileID, StorageKey, and ObjectGeneration stay server-side
// and are never serialized to clients.
type Asset struct {
	InternalID       int64
	ID               string
	WorkspaceID      string
	OwnerUserID      int64
	Status           string
	MediaKind        string
	MIMEType         string
	Width            int32
	Height           int32
	SizeBytes        int64
	ChecksumSHA256   string
	ObjectGeneration int64
	StorageProfileID string
	StorageKey       string
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Derivative is the server-side projection of one materialized
// derivative row used by the v1 derivative-bytes endpoint.
type Derivative struct {
	Tier                   string
	Format                 string
	Status                 string
	MIMEType               string
	Width                  int32
	Height                 int32
	SizeBytes              int64
	StorageProfileID       string
	StorageKey             string
	SourceObjectGeneration int64
}

// IsArchived reports whether the asset carries the archive lifecycle
// marker (metadata.archived_at in postgres).
func (a *Asset) IsArchived() bool {
	return a != nil && a.ArchivedAt != nil
}

// CreateAssetParams carries validated asset creation input.
// Repository methods take context.Context first.
type CreateAssetParams struct {
	WorkspaceID      string
	OwnerUserID      int64
	MediaKind        string
	MIMEType         string
	Width            int32
	Height           int32
	SizeBytes        int64
	ChecksumSHA256   string
	StorageProfileID string
	StorageKey       string
}

// AssetStore is the workspace-bound media asset persistence seam.
type AssetStore interface {
	CreateAsset(ctx context.Context, params CreateAssetParams) (*Asset, error)
	// GetAsset returns ErrNotFound for cross-workspace ids (invisible).
	GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error)
	// ListAssets returns creation-ordered (newest first) non-archived
	// assets; limit is clamped to [1,100] like the contract kernel.
	ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error)
	// ArchiveAsset stamps the archive lifecycle marker plus the
	// retention reclaim_after; it is idempotent per asset.
	ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error)
	// ListDerivatives returns materialized derivative rows for the
	// v1 derivative-bytes endpoint. The in-memory sibling carries
	// none (assets there never verify bytes); postgres reuses the
	// existing media_derivatives queries.
	ListDerivatives(ctx context.Context, ownerUserID, assetInternalID int64) ([]Derivative, error)
}

// InMemorySessionStore is a mutex-guarded SessionStore for tests and the
// process-local production backend. Sessions are ephemeral: no sweeper,
// expiry is lazy on read (see doc.go).
type InMemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	now      func() time.Time
}

// NewInMemorySessionStore builds an empty session store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{sessions: make(map[string]*Session), now: time.Now}
}

// SetClockForTests injects a deterministic clock.
func (s *InMemorySessionStore) SetClockForTests(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *InMemorySessionStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func copySession(in *Session) *Session {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

// applyLazyTransitions moves pending -> transferring on first read and
// expires past-TTL sessions. It returns ErrExpired for expired sessions.
func applyLazyTransitions(session *Session, now time.Time) error {
	if session.IsTerminal() {
		if session.Status == StatusExpired {
			return ErrExpired
		}
		return nil
	}
	if !now.Before(session.ExpiresAt) {
		session.Status = StatusExpired
		session.UpdatedAt = now
		return ErrExpired
	}
	if session.Status == StatusPending {
		session.Status = StatusTransferring
		session.UpdatedAt = now
		session.ExpiresAt = now.Add(UploadTransferTTL)
	}
	return nil
}

func (s *InMemorySessionStore) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	if err := ValidateCreateInput(params.Filename, params.MIMEType, params.SizeBytes, params.ChecksumSHA256, params.MediaKind); err != nil {
		return nil, err
	}
	id, err := NewUploadPublicID()
	if err != nil {
		return nil, err
	}
	now := s.clock()
	session := &Session{
		ID:             id,
		WorkspaceID:    params.WorkspaceID,
		OwnerUserID:    params.OwnerUserID,
		Filename:       params.Filename,
		MIMEType:       CanonicalMIME(params.MIMEType),
		SizeBytes:      params.SizeBytes,
		ChecksumSHA256: NormalizeChecksum(params.ChecksumSHA256),
		MediaKind:      MediaKindImage,
		Status:         StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(UploadPendingTTL),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[id]; exists {
		return nil, ErrStateConflict
	}
	s.sessions[id] = session
	return copySession(session), nil
}

func (s *InMemorySessionStore) GetSession(ctx context.Context, workspaceID, uploadID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[uploadID]
	if !ok || session.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	if err := applyLazyTransitions(session, s.clock()); err != nil {
		return nil, err
	}
	return copySession(session), nil
}

func (s *InMemorySessionStore) CompleteSession(ctx context.Context, workspaceID, uploadID, assetID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[uploadID]
	if !ok || session.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	if err := applyLazyTransitions(session, s.clock()); err != nil {
		return nil, err
	}
	if session.Status != StatusPending && session.Status != StatusTransferring {
		return nil, ErrStateConflict
	}
	now := s.clock()
	session.Status = StatusCompleted
	session.AssetID = assetID
	session.UpdatedAt = now
	return copySession(session), nil
}

func (s *InMemorySessionStore) FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[uploadID]
	if !ok || session.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	if session.IsTerminal() {
		return nil, ErrStateConflict
	}
	now := s.clock()
	session.Status = StatusFailed
	session.FailReason = reason
	session.UpdatedAt = now
	return copySession(session), nil
}

// InMemoryAssetStore is a mutex-guarded AssetStore for tests. Production
// uses the postgres sibling (same interface, dbWithContext pattern).
type InMemoryAssetStore struct {
	mu     sync.Mutex
	assets map[string]*Asset
	order  []string
	nextID int64
	now    func() time.Time
}

// NewInMemoryAssetStore builds an empty asset store.
func NewInMemoryAssetStore() *InMemoryAssetStore {
	return &InMemoryAssetStore{assets: make(map[string]*Asset), now: time.Now}
}

// SetClockForTests injects a deterministic clock.
func (s *InMemoryAssetStore) SetClockForTests(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *InMemoryAssetStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func copyAsset(in *Asset) *Asset {
	if in == nil {
		return nil
	}
	out := *in
	if in.ArchivedAt != nil {
		archived := *in.ArchivedAt
		out.ArchivedAt = &archived
	}
	return &out
}

func clampAssetPage(limit, offset int) (int, int) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *InMemoryAssetStore) CreateAsset(ctx context.Context, params CreateAssetParams) (*Asset, error) {
	if err := ValidateCreateInput("asset", params.MIMEType, params.SizeBytes, params.ChecksumSHA256, params.MediaKind); err != nil {
		return nil, err
	}
	id, err := NewAssetPublicID()
	if err != nil {
		return nil, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := &Asset{
		InternalID:       s.nextInternalIDLocked(),
		ID:               id,
		WorkspaceID:      params.WorkspaceID,
		OwnerUserID:      params.OwnerUserID,
		Status:           StatusPending,
		MediaKind:        MediaKindImage,
		MIMEType:         CanonicalMIME(params.MIMEType),
		Width:            params.Width,
		Height:           params.Height,
		SizeBytes:        params.SizeBytes,
		ChecksumSHA256:   NormalizeChecksum(params.ChecksumSHA256),
		ObjectGeneration: 1,
		StorageProfileID: params.StorageProfileID,
		StorageKey:       params.StorageKey,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, exists := s.assets[id]; exists {
		return nil, ErrStateConflict
	}
	s.assets[id] = asset
	s.order = append(s.order, id)
	return copyAsset(asset), nil
}

func (s *InMemoryAssetStore) GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[assetID]
	if !ok || asset.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	return copyAsset(asset), nil
}

func (s *InMemoryAssetStore) ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error) {
	limit, offset = clampAssetPage(limit, offset)
	s.mu.Lock()
	defer s.mu.Unlock()
	var filtered []*Asset
	for i := len(s.order) - 1; i >= 0; i-- {
		asset := s.assets[s.order[i]]
		if asset.WorkspaceID != workspaceID || asset.IsArchived() {
			continue
		}
		filtered = append(filtered, copyAsset(asset))
	}
	if offset >= len(filtered) {
		return []*Asset{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

func (s *InMemoryAssetStore) ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[assetID]
	if !ok || asset.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	if asset.ArchivedAt == nil {
		now := s.clock()
		asset.ArchivedAt = &now
		asset.UpdatedAt = now
	}
	return copyAsset(asset), nil
}

// nextInternalIDLocked mints a process-local stand-in for the
// database-internal numeric key (never serialized).
func (s *InMemoryAssetStore) nextInternalIDLocked() int64 {
	s.nextID++
	return s.nextID
}

// ListDerivatives reports no materialized rows: in-memory assets never
// verify bytes, so every tier stays not-ready (409) in tests.
func (s *InMemoryAssetStore) ListDerivatives(ctx context.Context, ownerUserID, assetInternalID int64) ([]Derivative, error) {
	return []Derivative{}, nil
}
