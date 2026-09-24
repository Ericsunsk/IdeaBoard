// Package librarymaterialization implements Media's half of the Library import
// seam. Library supplies only facts from an already-owned asset; this package
// owns remote fetch, storage, Media Asset registration and derivatives.
package librarymaterialization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/outbound"

	"github.com/jackc/pgx/v5/pgxpool"
)

type mediaStore interface {
	PrepareUploadRef(context.Context, string, ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	StoreMediaRefAtProfile(context.Context, string, string, ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	DeleteObjectByStorageProfile(context.Context, string, string) error
}

type libraryAssetLocker interface {
	Lock(context.Context, int64) (func(), error)
}

// Service uses PostgreSQL advisory locks keyed by the globally unique Library
// asset ID. The lock spans source download and registration, so concurrent API
// processes converge on one managed Media asset rather than duplicate storage.
type Service struct {
	storage  mediaStore
	registry assets.LibraryMaterializationRegistry
	locker   libraryAssetLocker
	gate     *materializationGate
}

var processMaterializationGate = newMaterializationGate()

func NewService(db *pgxpool.Pool, storageService mediaStore, registry assets.LibraryMaterializationRegistry) *Service {
	return newService(storageService, registry, newPostgresLibraryAssetLocker(db), processMaterializationGate)
}

func newService(storageService mediaStore, registry assets.LibraryMaterializationRegistry, locker libraryAssetLocker, gate *materializationGate) *Service {
	return &Service{storage: storageService, registry: registry, locker: locker, gate: gate}
}

func (s *Service) MaterializeLibraryAsset(ctx context.Context, input library.MaterializeInput) (library.MaterializedAsset, error) {
	if s == nil || s.storage == nil || s.registry == nil || s.locker == nil || s.gate == nil || input.OwnerUserID <= 0 || input.LibraryAssetID <= 0 {
		return library.MaterializedAsset{}, fmt.Errorf("library media materializer is unavailable")
	}
	if existing, ok, err := s.registry.FindReadyLibraryMaterializedMedia(ctx, input.OwnerUserID, input.LibraryAssetID); err != nil {
		return library.MaterializedAsset{}, err
	} else if ok {
		return materializedResult(existing), nil
	}

	releaseGate, err := s.gate.Acquire(ctx, input.OwnerUserID)
	if err != nil {
		return library.MaterializedAsset{}, fmt.Errorf("acquire library materialization capacity: %w", err)
	}
	defer releaseGate()

	releaseLock, err := s.locker.Lock(ctx, input.LibraryAssetID)
	if err != nil {
		return library.MaterializedAsset{}, fmt.Errorf("acquire library materialization lock: %w", err)
	}
	defer releaseLock()

	if existing, ok, err := s.registry.FindReadyLibraryMaterializedMedia(ctx, input.OwnerUserID, input.LibraryAssetID); err != nil {
		return library.MaterializedAsset{}, err
	} else if ok {
		return materializedResult(existing), nil
	}
	return s.materializeLocked(ctx, input)
}

func (s *Service) materializeLocked(ctx context.Context, input library.MaterializeInput) (library.MaterializedAsset, error) {
	dirPrefix, err := domain.LibraryAssetUploadDirPrefix(input.OwnerUserID, input.LibraryAssetID)
	if err != nil {
		return library.MaterializedAsset{}, err
	}
	planned, err := s.storage.PrepareUploadRef(ctx, "original", ports.StorageStoreOptions{DirPrefix: dirPrefix})
	if err != nil {
		return library.MaterializedAsset{}, fmt.Errorf("prepare library media storage: %w", err)
	}
	request := ingestion.Request{
		OwnerUserID: input.OwnerUserID, StorageProfile: planned.StorageProfile,
		StorageKey: planned.StorageKey, URL: planned.URL,
		Origin: ingestion.LibraryMaterializationFacts{
			LibraryAssetID: input.LibraryAssetID, CanonicalSourceURL: input.SourceURL,
			Provider: input.Provider, SourceID: input.SourceID, Title: input.Title, Author: input.Author,
		},
	}
	handle, err := s.registry.Begin(ctx, request)
	if err != nil {
		return library.MaterializedAsset{}, err
	}

	stored, sourceURL, err := s.storeOriginalThenPreview(ctx, planned, input)
	if err != nil {
		_ = s.registry.Fail(context.WithoutCancel(ctx), handle, err)
		return library.MaterializedAsset{}, err
	}
	if err := s.registry.Complete(ctx, handle, stored.IngestionCompletion(sourceURL)); err != nil {
		_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), planned.StorageProfile, planned.StorageKey)
		_ = s.registry.Fail(context.WithoutCancel(ctx), handle, err)
		return library.MaterializedAsset{}, fmt.Errorf("complete library materialized media: %w", err)
	}
	asset, err := s.registry.GetByID(ctx, input.OwnerUserID, handle.AssetID)
	if err != nil {
		return library.MaterializedAsset{}, fmt.Errorf("load materialized library media: %w", err)
	}
	return materializedResult(asset), nil
}

type postgresLibraryAssetLocker struct{ db *pgxpool.Pool }

func newPostgresLibraryAssetLocker(db *pgxpool.Pool) *postgresLibraryAssetLocker {
	return &postgresLibraryAssetLocker{db: db}
}

func (l *postgresLibraryAssetLocker) Lock(ctx context.Context, libraryAssetID int64) (func(), error) {
	if l == nil || l.db == nil {
		return nil, errors.New("library materialization lock database is unavailable")
	}
	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", libraryAssetID); err != nil {
		conn.Release()
		return nil, err
	}
	return func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", libraryAssetID)
		conn.Release()
	}, nil
}

const (
	maxConcurrentMaterializationsPerProcess = 4
	maxConcurrentMaterializationsPerUser    = 2
)

// materializationGate bounds expensive remote fetches. Unlike the HTTP upload
// limiter, callers wait with their request context and one user cannot occupy
// the entire process-wide capacity.
type materializationGate struct {
	process chan struct{}
	mu      sync.Mutex
	users   map[int64]*userMaterializationGate
}

type userMaterializationGate struct {
	slots  chan struct{}
	leases int
}

func newMaterializationGate() *materializationGate {
	return &materializationGate{
		process: make(chan struct{}, maxConcurrentMaterializationsPerProcess),
		users:   make(map[int64]*userMaterializationGate),
	}
}

func (g *materializationGate) Acquire(ctx context.Context, userID int64) (func(), error) {
	if g == nil || userID <= 0 {
		return nil, errors.New("library materialization gate is unavailable")
	}
	user := g.retainUser(userID)
	select {
	case user.slots <- struct{}{}:
	case <-ctx.Done():
		g.releaseUser(userID, user)
		return nil, ctx.Err()
	}
	select {
	case g.process <- struct{}{}:
	case <-ctx.Done():
		<-user.slots
		g.releaseUser(userID, user)
		return nil, ctx.Err()
	}
	return func() {
		<-g.process
		<-user.slots
		g.releaseUser(userID, user)
	}, nil
}

func (g *materializationGate) retainUser(userID int64) *userMaterializationGate {
	g.mu.Lock()
	defer g.mu.Unlock()
	user := g.users[userID]
	if user == nil {
		user = &userMaterializationGate{slots: make(chan struct{}, maxConcurrentMaterializationsPerUser)}
		g.users[userID] = user
	}
	user.leases++
	return user
}

func (g *materializationGate) releaseUser(userID int64, user *userMaterializationGate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.users[userID] != user {
		return
	}
	user.leases--
	if user.leases == 0 {
		delete(g.users, userID)
	}
}

func (s *Service) storeOriginalThenPreview(ctx context.Context, planned *ports.StoredUploadRef, input library.MaterializeInput) (*ports.StoredUploadRef, string, error) {
	if planned == nil {
		return nil, "", fmt.Errorf("library materialization storage plan is missing")
	}
	options := ports.StorageStoreOptions{ObjectKey: planned.StorageKey, AllowHosts: outbound.AllowHosts(ctx)}
	urls := []string{strings.TrimSpace(input.OriginalURL)}
	if preview := strings.TrimSpace(input.PreviewURL); preview != "" && preview != urls[0] {
		urls = append(urls, preview)
	}
	var sourceErr error
	for _, sourceURL := range urls {
		if sourceURL == "" {
			continue
		}
		stored, err := s.storage.StoreMediaRefAtProfile(ctx, planned.StorageProfile, sourceURL, options)
		if err == nil {
			return stored, sourceURL, nil
		}
		sourceErr = err
	}
	if sourceErr == nil {
		sourceErr = errors.New("library asset has no materializable image URL")
	}
	return nil, "", fmt.Errorf("materialize library image: %w", sourceErr)
}

// sqlc's generated struct stays inside Media; only the Library-owned result
// crosses the context boundary.
//
// #817 (Phase 3b-08): the legacy user-scoped media_assets row is gone; this
// maps the workspace-scoped row (public_id asset_%). The materialization
// path itself is fail-closed through the assets registry stubs until the
// workspace-scoped store lands in Phase 4, so this mapping never executes
// today — it only keeps the orphaned service compiling.
func materializedResult(asset dbsqlc.MediaAsset) library.MaterializedAsset {
	width, height := int32(0), int32(0)
	if asset.Width.Valid {
		width = asset.Width.Int32
	}
	if asset.Height.Valid {
		height = asset.Height.Int32
	}
	return library.MaterializedAsset{
		MediaAssetID: asset.InternalID, AssetID: asset.PublicID,
		URL:   "/api/user/media-assets/" + asset.PublicID + "/original/content",
		Width: width, Height: height, MIMEType: asset.MimeType,
		Extension: imageformat.ImageExtensionFromMIME(asset.MimeType),
	}
}
