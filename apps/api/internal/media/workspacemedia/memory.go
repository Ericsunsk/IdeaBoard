package workspacemedia

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryWorkspaceMediaStore is a mutex-guarded WorkspaceMediaStore
// for tests ONLY. It must never be wired in production. It mirrors the
// postgres semantics: workspace-keyed lookups (cross-workspace ids
// miss), lazy session expiry, atomic complete (asset insert +
// session flip), and idempotent archive.
type InMemoryWorkspaceMediaStore struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	assets      map[string]*Asset
	derivatives map[string][]*Derivative
	taskAssets  map[string][]*TaskAsset
	now         func() time.Time
}

// NewInMemoryWorkspaceMediaStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceMediaStoreForTests() *InMemoryWorkspaceMediaStore {
	return &InMemoryWorkspaceMediaStore{
		sessions:    make(map[string]*Session),
		assets:      make(map[string]*Asset),
		derivatives: make(map[string][]*Derivative),
		taskAssets:  make(map[string][]*TaskAsset),
		now:         time.Now,
	}
}

// Compile-time convergence with the v1 persistence seam.
var _ WorkspaceMediaStore = (*InMemoryWorkspaceMediaStore)(nil)

// SetClockForTests injects a deterministic clock (lazy-expiry tests).
func (s *InMemoryWorkspaceMediaStore) SetClockForTests(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *InMemoryWorkspaceMediaStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func mediaKey(workspaceID, publicID string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(publicID)
}

func copySession(in *Session) *Session {
	if in == nil {
		return nil
	}
	out := *in
	if in.CompletedAt != nil {
		completed := *in.CompletedAt
		out.CompletedAt = &completed
	}
	if in.ExpiredAt != nil {
		expired := *in.ExpiredAt
		out.ExpiredAt = &expired
	}
	return &out
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

func copyDerivative(in *Derivative) *Derivative {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

// applyLazyEffectsLocked flips pending -> transferring on first read
// and expires rows past their TTL. Expired rows report ErrExpired.
func (s *InMemoryWorkspaceMediaStore) applyLazyEffectsLocked(session *Session) error {
	switch session.Status {
	case StatusPending, StatusTransferring:
		now := s.clock()
		if !now.Before(sessionExpiryDeadline(session.CreatedAt, session.Status)) {
			session.Status = StatusExpired
			expired := now
			session.ExpiredAt = &expired
			session.UpdatedAt = now
			session.ExpiresAt = expired
			return ErrExpired
		}
		if session.Status == StatusPending {
			session.Status = StatusTransferring
			session.UpdatedAt = now
			session.ExpiresAt = sessionExpiryDeadline(session.CreatedAt, StatusTransferring)
		}
		return nil
	case StatusExpired:
		return ErrExpired
	default:
		return nil
	}
}

// CreateSession validates the declaration and reserves the slot.
func (s *InMemoryWorkspaceMediaStore) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filename := strings.TrimSpace(params.Filename)
	mimeType := strings.TrimSpace(params.MIMEType)
	checksum := NormalizeChecksum(params.ChecksumSHA256)
	kind := strings.TrimSpace(params.MediaKind)
	if kind == "" {
		kind = MediaKindImage
	}
	if validateErr := ValidateCreateInput(filename, mimeType, params.SizeBytes, checksum, kind); validateErr != nil {
		return nil, validateErr
	}
	if params.OwnerUserID <= 0 {
		return nil, NewFieldValidationError("owner")
	}
	if strings.TrimSpace(params.WorkspaceID) == "" {
		return nil, ErrNotFound
	}
	publicID, err := NewUploadPublicID()
	if err != nil {
		return nil, err
	}
	now := s.clock()
	session := &Session{
		ID:             publicID,
		WorkspaceID:    strings.TrimSpace(params.WorkspaceID),
		OwnerUserID:    params.OwnerUserID,
		Filename:       filename,
		MIMEType:       CanonicalMIME(mimeType),
		SizeBytes:      params.SizeBytes,
		ChecksumSHA256: checksum,
		MediaKind:      MediaKindForMIME(mimeType),
		Status:         StatusPending,
		StorageKey:     SlotKey(params.WorkspaceID, publicID, filename),
		CreatedAt:      now,
		ExpiresAt:      sessionExpiryDeadline(now, StatusPending),
		UpdatedAt:      now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[mediaKey(session.WorkspaceID, session.ID)] = session
	return copySession(session), nil
}

// GetSession resolves a session with lazy lifecycle effects.
func (s *InMemoryWorkspaceMediaStore) GetSession(ctx context.Context, workspaceID, uploadID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[mediaKey(workspaceID, uploadID)]
	if session == nil {
		return nil, ErrNotFound
	}
	if effectErr := s.applyLazyEffectsLocked(session); effectErr != nil {
		return copySession(session), effectErr
	}
	return copySession(session), nil
}

// CompleteSession atomically inserts the asset and flips the session.
func (s *InMemoryWorkspaceMediaStore) CompleteSession(ctx context.Context, workspaceID, uploadID string, facts CompleteFacts, asset CreateAssetParams) (*Session, *Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if facts.Width < 0 || facts.Height < 0 || facts.Width > 32768 || facts.Height > 32768 {
		return nil, nil, NewFieldValidationError("dimensions")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[mediaKey(workspaceID, uploadID)]
	if session == nil {
		return nil, nil, ErrNotFound
	}
	if effectErr := s.applyLazyEffectsLocked(session); effectErr != nil {
		return copySession(session), nil, effectErr
	}
	if !session.IsLive() {
		return nil, nil, ErrStateConflict
	}
	if mismatch := matchCompletionFacts(session, facts); mismatch != nil {
		session.Status = StatusFailed
		session.UpdatedAt = s.clock()
		return nil, nil, mismatch
	}
	publicID, err := NewAssetPublicID()
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	created := &Asset{
		InternalID:     int64(len(s.assets) + 1),
		ID:             publicID,
		WorkspaceID:    session.WorkspaceID,
		OwnerUserID:    session.OwnerUserID,
		Status:         AssetStatusReady,
		MediaKind:      session.MediaKind,
		MIMEType:       session.MIMEType,
		Width:          facts.Width,
		Height:         facts.Height,
		SizeBytes:      session.SizeBytes,
		ChecksumSHA256: session.ChecksumSHA256,
		StorageKey:     session.StorageKey,
		CreatedAt:      now,
	}
	s.assets[mediaKey(created.WorkspaceID, created.ID)] = created
	session.Status = StatusCompleted
	completed := now
	session.CompletedAt = &completed
	session.UpdatedAt = now
	session.AssetID = created.ID
	return copySession(session), copyAsset(created), nil
}

// FailSession marks a live session failed.
func (s *InMemoryWorkspaceMediaStore) FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = strings.TrimSpace(reason)
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[mediaKey(workspaceID, uploadID)]
	if session == nil {
		return nil, ErrNotFound
	}
	if effectErr := s.applyLazyEffectsLocked(session); effectErr != nil {
		return copySession(session), effectErr
	}
	if !session.IsLive() {
		if session.Status == StatusFailed {
			return copySession(session), nil
		}
		return nil, ErrStateConflict
	}
	session.Status = StatusFailed
	session.UpdatedAt = s.clock()
	return copySession(session), nil
}

// GetAsset resolves a workspace-scoped asset.
func (s *InMemoryWorkspaceMediaStore) GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[mediaKey(workspaceID, assetID)]
	if asset == nil {
		return nil, ErrNotFound
	}
	return copyAsset(asset), nil
}

// ListAssets pages live workspace assets newest-first.
func (s *InMemoryWorkspaceMediaStore) ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > int(maxMediaAssetRows) {
		limit = int(maxMediaAssetRows)
	}
	if offset < 0 {
		offset = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := strings.TrimSpace(workspaceID) + "\x00"
	all := make([]*Asset, 0)
	for key, asset := range s.assets {
		if !strings.HasPrefix(key, prefix) || asset.IsArchived() {
			continue
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

// ArchiveAsset stamps the archive marker (idempotent per asset).
func (s *InMemoryWorkspaceMediaStore) ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[mediaKey(workspaceID, assetID)]
	if asset == nil {
		return nil, ErrNotFound
	}
	if asset.ArchivedAt == nil {
		now := s.clock()
		asset.ArchivedAt = &now
	}
	return copyAsset(asset), nil
}

// UpdateAssetMetadata refreshes dimensions and merges metadata
// (metadata has no durable test projection beyond acceptance).
func (s *InMemoryWorkspaceMediaStore) UpdateAssetMetadata(ctx context.Context, workspaceID, assetID string, params UpdateMetadataParams) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	asset := s.assets[mediaKey(workspaceID, assetID)]
	if asset == nil {
		return nil, ErrNotFound
	}
	if params.HasDims {
		asset.Width = params.Width
		asset.Height = params.Height
	}
	return copyAsset(asset), nil
}

// RecordDerivative upserts one derivative variant.
func (s *InMemoryWorkspaceMediaStore) RecordDerivative(ctx context.Context, workspaceID, assetID string, derivative Derivative) (*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[mediaKey(workspaceID, assetID)] == nil {
		return nil, ErrNotFound
	}
	variant := strings.TrimSpace(derivative.VariantName)
	if variant == "" {
		return nil, NewFieldValidationError("variant")
	}
	if strings.TrimSpace(derivative.StorageKey) == "" {
		return nil, NewFieldValidationError("storage_key")
	}
	key := mediaKey(workspaceID, assetID)
	record := copyDerivative(&derivative)
	record.VariantName = variant
	record.CreatedAt = s.clock()
	replaced := false
	for i, existing := range s.derivatives[key] {
		if existing.VariantName == variant {
			s.derivatives[key][i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		s.derivatives[key] = append(s.derivatives[key], record)
	}
	return copyDerivative(record), nil
}

// ListDerivatives returns stored variants for one asset.
func (s *InMemoryWorkspaceMediaStore) ListDerivatives(ctx context.Context, workspaceID, assetID string) ([]*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[mediaKey(workspaceID, assetID)] == nil {
		return nil, ErrNotFound
	}
	rows := s.derivatives[mediaKey(workspaceID, assetID)]
	out := make([]*Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, copyDerivative(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VariantName < out[j].VariantName })
	return out, nil
}

// SeedTaskAssetForTests attaches an asset to a task for ListTaskAssets
// tests (the in-memory store carries no task tables).
func (s *InMemoryWorkspaceMediaStore) SeedTaskAssetForTests(workspaceID, taskID string, item *TaskAsset) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := mediaKey(workspaceID, taskID)
	entry := &TaskAsset{OutputIndex: item.OutputIndex, Role: item.Role}
	if item.Asset != nil {
		entry.Asset = copyAsset(item.Asset)
	}
	s.taskAssets[key] = append(s.taskAssets[key], entry)
}

// ListTaskAssets returns seeded task attachments (empty by default).
func (s *InMemoryWorkspaceMediaStore) ListTaskAssets(ctx context.Context, workspaceID, taskID string) ([]*TaskAsset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.taskAssets[mediaKey(workspaceID, taskID)]
	out := make([]*TaskAsset, 0, len(rows))
	for _, row := range rows {
		entry := &TaskAsset{OutputIndex: row.OutputIndex, Role: row.Role}
		if row.Asset != nil {
			entry.Asset = copyAsset(row.Asset)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OutputIndex < out[j].OutputIndex })
	return out, nil
}
