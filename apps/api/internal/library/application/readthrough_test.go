package application

// Library materialization orchestration seam tests (issue #908):
// claim/dedup, failure folding, and completed convergence through
// fake executor/materializer only (no real download, no external
// writes; the in-memory store is the only persistence).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	materializerassets "ideaboard/api/internal/media/assets"
)

type fakeOrchestratorMaterializer struct {
	mu      sync.Mutex
	calls   int
	mediaID string
	err     error
	release chan struct{}
}

func (f *fakeOrchestratorMaterializer) MaterializeWorkspaceAsset(ctx context.Context, _ string, _ *domain.V1Asset) (string, error) {
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.mediaID, f.err
}

var _ materializerassets.WorkspaceAssetMaterializer = (*fakeOrchestratorMaterializer)(nil)

func saveOrchestratorAsset(t *testing.T, store *librarymemory.InMemoryWorkspaceLibraryStore, ws, sourceID string) *domain.V1Asset {
	t.Helper()
	publicID, err := domain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	providerURL := "https://images.unsplash.com/" + sourceID
	saved, created, err := store.SaveAsset(context.Background(), ws, domain.V1SaveAssetInput{
		Asset: domain.AssetInput{
			Provider: domain.ProviderUnsplash, SourceID: sourceID,
			SourceURL: providerURL, PreviewURL: providerURL, OriginalURL: providerURL,
		},
	}, publicID)
	if err != nil || !created || saved == nil {
		t.Fatalf("SaveAsset = %+v %v %v, want created", saved, created, err)
	}
	return saved
}

func inlineExecutor() Executor { return SpawnFunc(func(fn func()) { fn() }) }

func orchestratorJobs(t *testing.T, store *librarymemory.InMemoryWorkspaceLibraryStore, ws string) []*domain.V1Materialization {
	t.Helper()
	all, err := store.ListMaterializations(context.Background(), ws)
	if err != nil {
		t.Fatalf("ListMaterializations: %v", err)
	}
	return all
}

// TestReadThroughClaimConverges proves the second request for an
// asset with a completed link performs no second fetch: the atomic
// claim returns the completed record and the run converges.
func TestReadThroughClaimConverges(t *testing.T) {
	ctx := context.Background()
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	saved := saveOrchestratorAsset(t, store, "ws_converge", "photo-converge-1")
	fetcher := &fakeOrchestratorMaterializer{mediaID: "asset_converge00000001"}
	svc := NewReadThroughService(store, fetcher, inlineExecutor(), 0)
	if svc == nil {
		t.Fatal("NewReadThroughService returned nil")
	}

	svc.Request(ctx, "ws_converge", saved, 42)
	all := orchestratorJobs(t, store, "ws_converge")
	if len(all) != 1 || all[0].Status != domain.V1MaterializationCompleted || all[0].MediaAssetID != "asset_converge00000001" {
		t.Fatalf("jobs = %+v, want one completed link", all)
	}

	// Repeat: completed history converges without refetching.
	svc.Request(ctx, "ws_converge", saved, 42)
	all = orchestratorJobs(t, store, "ws_converge")
	if len(all) != 1 {
		t.Fatalf("jobs = %d, want 1 (completed claim suppresses a duplicate job)", len(all))
	}
	fetcher.mu.Lock()
	calls := fetcher.calls
	fetcher.mu.Unlock()
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}

	// Sync resolve replays the same link.
	_, mediaID, found := svc.Resolve(ctx, "ws_converge", saved.PublicID)
	if !found || mediaID != "asset_converge00000001" {
		t.Fatalf("resolve = %q %v, want linked asset_", mediaID, found)
	}
}

// TestReadThroughFailureFoldsAndRetries proves a failed fetch folds
// into a failed job with the fixed message (never a usable media
// link) and a later request retries with a new job instead of
// resurrecting the terminal one.
func TestReadThroughFailureFoldsAndRetries(t *testing.T) {
	ctx := context.Background()
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	saved := saveOrchestratorAsset(t, store, "ws_fail", "photo-fail-1")
	fetcher := &fakeOrchestratorMaterializer{err: errors.New("provider down")}
	svc := NewReadThroughService(store, fetcher, inlineExecutor(), 0)
	if svc == nil {
		t.Fatal("NewReadThroughService returned nil")
	}

	svc.Request(ctx, "ws_fail", saved, 42)
	all := orchestratorJobs(t, store, "ws_fail")
	if len(all) != 1 || all[0].Status != domain.V1MaterializationFailed {
		t.Fatalf("jobs = %+v, want one failed job", all)
	}
	if all[0].ErrorMessage != ReadThroughFailureMessage {
		t.Fatalf("error message = %q, want fixed retry message", all[0].ErrorMessage)
	}
	if all[0].MediaAssetID != "" {
		t.Fatalf("failed link = %q, want no usable media", all[0].MediaAssetID)
	}
	if _, _, found := svc.Resolve(ctx, "ws_fail", saved.PublicID); found {
		t.Fatal("resolve found a failed job, want miss (failures never replay as media)")
	}

	svc.Request(ctx, "ws_fail", saved, 42)
	all = orchestratorJobs(t, store, "ws_fail")
	if len(all) != 2 {
		t.Fatalf("jobs = %d, want 2 (failed history retries with a new job)", len(all))
	}
	for _, job := range all {
		if job.Status != domain.V1MaterializationFailed {
			t.Fatalf("job %+v, want failed", job)
		}
	}
}

// TestReadThroughDedupsInFlight proves a second request while the
// fetch is still running records no duplicate job and performs no
// second fetch; the link lands once the fetch completes.
func TestReadThroughDedupsInFlight(t *testing.T) {
	ctx := context.Background()
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	saved := saveOrchestratorAsset(t, store, "ws_inflight", "photo-inflight-1")
	fetcher := &fakeOrchestratorMaterializer{mediaID: "asset_inflight0000001", release: make(chan struct{})}
	svc := NewReadThroughService(store, fetcher, GoroutineExecutor{}, 0)
	if svc == nil {
		t.Fatal("NewReadThroughService returned nil")
	}

	svc.Request(ctx, "ws_inflight", saved, 42)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if all := orchestratorJobs(t, store, "ws_inflight"); len(all) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the in-flight job")
		}
		time.Sleep(10 * time.Millisecond)
	}

	svc.Request(ctx, "ws_inflight", saved, 42)
	time.Sleep(50 * time.Millisecond)
	if all := orchestratorJobs(t, store, "ws_inflight"); len(all) != 1 {
		t.Fatalf("jobs = %d, want 1 (in-flight job suppresses a duplicate)", len(all))
	}

	close(fetcher.release)
	deadline = time.Now().Add(5 * time.Second)
	for {
		all := orchestratorJobs(t, store, "ws_inflight")
		if len(all) == 1 && all[0].Status == domain.V1MaterializationCompleted {
			if all[0].MediaAssetID != "asset_inflight0000001" {
				t.Fatalf("completed link = %q, want asset_ shape", all[0].MediaAssetID)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for completion, have %+v", all)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fetcher.mu.Lock()
	calls := fetcher.calls
	fetcher.mu.Unlock()
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
}

// TestReadThroughRequestGuards proves owner, provider, and
// construction guards fail closed with no job and no fetch.
func TestReadThroughRequestGuards(t *testing.T) {
	ctx := context.Background()
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	saved := saveOrchestratorAsset(t, store, "ws_guards", "photo-guards-1")
	fetcher := &fakeOrchestratorMaterializer{mediaID: "asset_guards000000001"}
	svc := NewReadThroughService(store, fetcher, inlineExecutor(), 0)
	if svc == nil {
		t.Fatal("NewReadThroughService returned nil")
	}

	svc.Request(ctx, "ws_guards", nil, 42)
	svc.Request(ctx, "ws_guards", saved, 0)
	svc.Request(ctx, "ws_guards", saved, -1)
	workspaceMedia := *saved
	workspaceMedia.Source = domain.AssetSourceMedia
	svc.Request(ctx, "ws_guards", &workspaceMedia, 42)
	if all := orchestratorJobs(t, store, "ws_guards"); len(all) != 0 {
		t.Fatalf("jobs = %d, want 0 (guards fail closed)", len(all))
	}

	if got := NewReadThroughService(nil, fetcher, inlineExecutor(), 0); got != nil {
		t.Fatal("nil store built a service, want fail-closed nil")
	}
	if got := NewReadThroughService(store, nil, inlineExecutor(), 0); got != nil {
		t.Fatal("nil materializer built a service, want fail-closed nil")
	}
	if got := NewReadThroughService(store, fetcher, nil, 0); got != nil {
		t.Fatal("nil executor built a service, want fail-closed nil")
	}
	var nilSvc *ReadThroughService
	if _, _, found := nilSvc.Resolve(ctx, "ws_guards", saved.PublicID); found {
		t.Fatal("nil service resolved, want miss")
	}
	nilSvc.Request(ctx, "ws_guards", saved, 42)
	if all := orchestratorJobs(t, store, "ws_guards"); len(all) != 0 {
		t.Fatalf("jobs = %d after nil request, want 0", len(all))
	}
}

// TestReadThroughTimeoutFoldsToDefault proves a fetch that outlives
// the detached timeout folds into a failed job with the fixed retry
// copy (never a usable media link), so the next Canvas retry rebuilds
// instead of wedging.
func TestReadThroughTimeoutFoldsToDefault(t *testing.T) {
	ctx := context.Background()
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	saved := saveOrchestratorAsset(t, store, "ws_timeout", "photo-timeout-1")
	fetcher := &fakeOrchestratorMaterializer{mediaID: "asset_timeout00000001", release: make(chan struct{})}
	svc := NewReadThroughService(store, fetcher, inlineExecutor(), 100*time.Millisecond)
	if svc == nil {
		t.Fatal("NewReadThroughService returned nil")
	}

	svc.Request(ctx, "ws_timeout", saved, 42)
	all := orchestratorJobs(t, store, "ws_timeout")
	if len(all) != 1 || all[0].Status != domain.V1MaterializationFailed {
		t.Fatalf("jobs = %+v, want one failed job", all)
	}
	if all[0].ErrorMessage != ReadThroughFailureMessage {
		t.Fatalf("error message = %q, want fixed retry message", all[0].ErrorMessage)
	}
	if all[0].MediaAssetID != "" {
		t.Fatalf("failed link = %q, want no usable media", all[0].MediaAssetID)
	}
	if _, _, found := svc.Resolve(ctx, "ws_timeout", saved.PublicID); found {
		t.Fatal("resolve found a timed-out job, want miss (failures never replay as media)")
	}
}

// TestFindCompletedMediaPinsNewest proves the replay scan converges
// on the newest completed link and never hands malformed links out.
func TestFindCompletedMediaPinsNewest(t *testing.T) {
	jobs := []*domain.V1Materialization{
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_old00000000001"},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationFailed},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "not-an-asset"},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_new00000000001"},
		{AssetPublicID: "lasset_y", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_other000000001"},
	}
	mediaID, found := FindCompletedMedia(jobs, "lasset_x")
	if !found || mediaID != "asset_new00000000001" {
		t.Fatalf("completed = %q %v, want newest asset_ link", mediaID, found)
	}
	if _, found := FindCompletedMedia(jobs, "lasset_missing"); found {
		t.Fatal("unknown asset replayed, want miss")
	}
	malformed := []*domain.V1Materialization{
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "broken"},
	}
	if _, found := FindCompletedMedia(malformed, "lasset_x"); found {
		t.Fatal("malformed link replayed, want miss")
	}
}

// workspaceReadThroughStore exposes a direct reference without a writable fixture row.
type workspaceReadThroughStore struct {
	ReadThroughStore
	asset *domain.V1Asset
	t     *testing.T
}

func (s workspaceReadThroughStore) GetAsset(context.Context, string, string) (*domain.V1Asset, error) {
	return s.asset, nil
}

func (s workspaceReadThroughStore) ListMaterializations(context.Context, string) ([]*domain.V1Materialization, error) {
	s.t.Fatal("media read-through consulted remote materializations")
	return nil, nil
}

func TestReadThroughMediaResolvesWithoutTrigger(t *testing.T) {
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	asset := &domain.V1Asset{PublicID: "lasset_workspace", Source: domain.AssetSourceMedia, MediaAssetID: "asset_workspace0000001"}
	fetcher := &fakeOrchestratorMaterializer{mediaID: "asset_unused000000001"}
	svc := NewReadThroughService(workspaceReadThroughStore{ReadThroughStore: store, asset: asset, t: t}, fetcher, inlineExecutor(), 0)
	stored, mediaID, found := svc.Resolve(context.Background(), "ws_workspace", asset.PublicID)
	if stored != asset || mediaID != asset.MediaAssetID || !found {
		t.Fatal("media reference did not resolve its existing managed link")
	}
	svc.Request(context.Background(), "ws_workspace", stored, 42)
	if len(orchestratorJobs(t, store, "ws_workspace")) != 0 || fetcher.calls != 0 {
		t.Fatal("media reference triggered materialization")
	}
}
