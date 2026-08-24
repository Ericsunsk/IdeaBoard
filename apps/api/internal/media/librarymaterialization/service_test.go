package librarymaterialization

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
)

type materializationStoreStub struct {
	mu            sync.Mutex
	storeCalls    []string
	storeOptions  []storage.StorageStoreOptions
	storeErrors   map[string]error
	stored        storage.StoredUploadRef
	deleteCalls   []storage.StoredUploadLocation
	firstStarted  chan struct{}
	continueFirst chan struct{}
	firstCall     bool
}

func (s *materializationStoreStub) PrepareUploadRef(_ context.Context, _ string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	return &storage.StoredUploadRef{
		URL:            "/uploads/" + opts.DirPrefix + "/original.png",
		StorageKey:     "uploads/" + opts.DirPrefix + "/original.png",
		StorageProfile: "profile-library",
	}, nil
}

func (s *materializationStoreStub) StoreMediaRefAtProfile(_ context.Context, _ string, source string, opts storage.StorageStoreOptions) (*storage.StoredUploadRef, error) {
	s.mu.Lock()
	s.storeCalls = append(s.storeCalls, source)
	s.storeOptions = append(s.storeOptions, opts)
	shouldBlock := !s.firstCall && s.firstStarted != nil
	if shouldBlock {
		s.firstCall = true
	}
	err := s.storeErrors[source]
	stored := s.stored
	s.mu.Unlock()
	if shouldBlock {
		close(s.firstStarted)
		<-s.continueFirst
	}
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

func (s *materializationStoreStub) DeleteObjectByStorageProfile(_ context.Context, profileID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls = append(s.deleteCalls, storage.StoredUploadLocation{StorageProfileID: profileID, StorageKey: key})
	return nil
}

type materializationRegistryStub struct {
	mu          sync.Mutex
	ready       dbsqlc.MediaAsset
	readyFound  bool
	nextID      int64
	begin       []ingestion.Request
	completed   []ingestion.Handle
	completions []ingestion.Completion
	failed      []ingestion.Handle
	completeErr error
}

func (r *materializationRegistryStub) FindReadyLibraryMaterializedMedia(_ context.Context, _, _ int64) (dbsqlc.MediaAsset, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready, r.readyFound, nil
}

func (r *materializationRegistryStub) Begin(_ context.Context, request ingestion.Request) (ingestion.Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.begin = append(r.begin, request)
	return ingestion.Handle{
		AssetID: r.nextID, OwnerUserID: request.OwnerUserID, StorageProfile: request.StorageProfile,
		StorageKey: request.StorageKey, Origin: request.Origin,
	}, nil
}

func (r *materializationRegistryStub) Complete(_ context.Context, handle ingestion.Handle, completion ingestion.Completion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completed = append(r.completed, handle)
	r.completions = append(r.completions, completion)
	if r.completeErr != nil {
		return r.completeErr
	}
	r.ready = dbsqlc.MediaAsset{
		ID: handle.AssetID, AssetID: "asset-library", Url: completion.URL, SourceUrl: completion.SourceURL,
		Width: int32(completion.Width), Height: int32(completion.Height), MimeType: completion.MIMEType, Status: "ready",
	}
	r.readyFound = true
	return nil
}

func (r *materializationRegistryStub) Fail(_ context.Context, handle ingestion.Handle, _ error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = append(r.failed, handle)
	return nil
}

func (r *materializationRegistryStub) GetByID(_ context.Context, _ int64, _ int64) (dbsqlc.MediaAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready, nil
}

type serialLibraryLocker struct {
	slots    chan struct{}
	mu       sync.Mutex
	calls    int
	attempts atomic.Int32
}

func newSerialLibraryLocker() *serialLibraryLocker {
	locker := &serialLibraryLocker{slots: make(chan struct{}, 1)}
	locker.slots <- struct{}{}
	return locker
}

func (l *serialLibraryLocker) Lock(ctx context.Context, _ int64) (func(), error) {
	l.attempts.Add(1)
	select {
	case <-l.slots:
		l.mu.Lock()
		l.calls++
		l.mu.Unlock()
		return func() { l.slots <- struct{}{} }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type outboundPolicyStub struct{ hosts []string }

func (s outboundPolicyStub) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: s.hosts}, nil
}

func materializationInput() library.MaterializeInput {
	return library.MaterializeInput{
		OwnerUserID: 1, LibraryAssetID: 9, Provider: "unsplash", SourceID: "source-9",
		SourceURL:   "https://unsplash.com/photos/source-9",
		OriginalURL: "https://images.unsplash.com/original.png",
		PreviewURL:  "https://images.unsplash.com/preview.png", Title: "Source title", Author: "Source author",
	}
}

func TestMaterializeLibraryAssetFallsBackToPreviewAndPreservesProvenance(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(outboundPolicyStub{hosts: []string{"images.unsplash.com"}})
	t.Cleanup(restore)
	store := &materializationStoreStub{
		storeErrors: map[string]error{"https://images.unsplash.com/original.png": errors.New("original unavailable")},
		stored:      storage.StoredUploadRef{URL: "/uploads/library/1/9/original.png", StorageKey: "uploads/library/1/9/original.png", StorageProfile: "profile-library", MIMEType: "image/png", Width: 640, Height: 480},
	}
	registry := &materializationRegistryStub{nextID: 41}
	service := newService(store, registry, newSerialLibraryLocker(), newMaterializationGate())

	result, err := service.MaterializeLibraryAsset(t.Context(), materializationInput())
	if err != nil {
		t.Fatalf("MaterializeLibraryAsset() error = %v", err)
	}
	if result.MediaAssetID != 41 || result.URL != "/api/user/media-assets/asset-library/original/content" {
		t.Fatalf("materialized result = %#v", result)
	}
	if got := store.storeCalls; len(got) != 2 || got[0] != materializationInput().OriginalURL || got[1] != materializationInput().PreviewURL {
		t.Fatalf("source attempts = %#v", got)
	}
	if got := store.storeOptions[0].AllowHosts; len(got) != 1 || got[0] != "images.unsplash.com" {
		t.Fatalf("outbound allow hosts = %#v", got)
	}
	if got := registry.begin[0].Origin.(ingestion.LibraryMaterializationFacts).CanonicalSourceURL; got != materializationInput().SourceURL {
		t.Fatalf("canonical source URL = %q", got)
	}
	completed := registry.completed[0].Origin.(ingestion.LibraryMaterializationFacts)
	if completed.CanonicalSourceURL != materializationInput().SourceURL || registry.completions[0].SourceURL != materializationInput().PreviewURL {
		t.Fatalf("completed provenance = %#v completion=%#v", completed, registry.completions[0])
	}
}

func TestMaterializeLibraryAssetReusesReadyMappingAfterAdvisoryLock(t *testing.T) {
	store := &materializationStoreStub{
		stored:       storage.StoredUploadRef{URL: "/uploads/library/1/9/original.png", StorageKey: "uploads/library/1/9/original.png", StorageProfile: "profile-library", MIMEType: "image/png", Width: 640, Height: 480},
		firstStarted: make(chan struct{}), continueFirst: make(chan struct{}),
	}
	registry := &materializationRegistryStub{nextID: 52}
	locker := newSerialLibraryLocker()
	service := newService(store, registry, locker, newMaterializationGate())

	type outcome struct{ err error }
	first := make(chan outcome, 1)
	second := make(chan outcome, 1)
	go func() {
		_, err := service.MaterializeLibraryAsset(context.Background(), materializationInput())
		first <- outcome{err}
	}()
	select {
	case <-store.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first materialization did not reach storage")
	}
	go func() {
		_, err := service.MaterializeLibraryAsset(context.Background(), materializationInput())
		second <- outcome{err}
	}()
	deadline := time.After(time.Second)
	for locker.attempts.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("second materialization did not contend for advisory lock")
		case <-time.After(time.Millisecond):
		}
	}
	close(store.continueFirst)
	if got := (<-first).err; got != nil {
		t.Fatalf("first materialization error = %v", got)
	}
	if got := (<-second).err; got != nil {
		t.Fatalf("second materialization error = %v", got)
	}
	if len(store.storeCalls) != 1 {
		t.Fatalf("storage calls = %#v, want exactly one materialization", store.storeCalls)
	}
	if locker.calls != 2 {
		t.Fatalf("advisory lock calls = %d, want both callers serialized", locker.calls)
	}
}

func TestMaterializeLibraryAssetCleansObjectWhenCompletionFails(t *testing.T) {
	store := &materializationStoreStub{stored: storage.StoredUploadRef{URL: "/uploads/library/1/9/original.png", StorageKey: "uploads/library/1/9/original.png", StorageProfile: "profile-library", MIMEType: "image/png"}}
	registry := &materializationRegistryStub{nextID: 68, completeErr: errors.New("completion failed")}
	service := newService(store, registry, newSerialLibraryLocker(), newMaterializationGate())

	if _, err := service.MaterializeLibraryAsset(t.Context(), materializationInput()); err == nil {
		t.Fatal("expected completion error")
	}
	if len(store.deleteCalls) != 1 || store.deleteCalls[0].StorageProfileID != "profile-library" || store.deleteCalls[0].StorageKey != "uploads/library/1/9/original.png" {
		t.Fatalf("cleanup calls = %#v", store.deleteCalls)
	}
	if len(registry.failed) != 1 || registry.failed[0].AssetID != 68 {
		t.Fatalf("failed registrations = %#v", registry.failed)
	}
}

func TestMaterializationGateRespectsPerUserLimitAndContext(t *testing.T) {
	gate := newMaterializationGate()
	first, err := gate.Acquire(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := gate.Acquire(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := gate.Acquire(ctx, 7); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() error = %v, want context deadline exceeded", err)
	}
}
