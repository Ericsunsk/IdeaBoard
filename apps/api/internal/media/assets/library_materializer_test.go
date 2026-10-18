package assets

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/library/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
)

type stubLibraryFetcher struct {
	mu    sync.Mutex
	calls int
	data  []byte
	err   error
}

func (f *stubLibraryFetcher) FetchSourceBytes(context.Context, string, []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]byte(nil), f.data...), nil
}

func (f *stubLibraryFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeLibraryObjects struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (f *fakeLibraryObjects) key(profile, objectKey string) string {
	return profile + "\x00" + objectKey
}

func (f *fakeLibraryObjects) PutObjectAtProfile(_ context.Context, profileID, objectKey string, data []byte, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data == nil {
		f.data = make(map[string][]byte)
	}
	f.data[f.key(profileID, objectKey)] = append([]byte(nil), data...)
	return nil
}

func (f *fakeLibraryObjects) ReadObjectAtProfile(_ context.Context, profileID, objectKey string, _ int64) (*storageports.StoredUploadObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.data[f.key(profileID, objectKey)]
	if !ok {
		return nil, errors.New("library test object is missing")
	}
	return &storageports.StoredUploadObject{Data: append([]byte(nil), data...)}, nil
}

func (f *fakeLibraryObjects) storedBytes() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, 0, len(f.data))
	for _, data := range f.data {
		out = append(out, data)
	}
	return out
}

type fakeLibraryReplay struct {
	mu    sync.Mutex
	media map[string]string
	err   error
}

func (f *fakeLibraryReplay) FindCompletedMaterializationMedia(_ context.Context, workspacePublicID, assetPublicID string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", false, f.err
	}
	mediaID, ok := f.media[strings.TrimSpace(workspacePublicID)+"\x00"+strings.TrimSpace(assetPublicID)]
	return mediaID, ok, nil
}

func (f *fakeLibraryReplay) seed(workspacePublicID, assetPublicID, mediaID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.media == nil {
		f.media = make(map[string]string)
	}
	f.media[strings.TrimSpace(workspacePublicID)+"\x00"+strings.TrimSpace(assetPublicID)] = mediaID
}

type stubPolicyProvider struct {
	setting settings.OutboundPolicySetting
	err     error
}

func (p *stubPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return p.setting, p.err
}

func stubLibraryEgress(t *testing.T, hosts []string) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(&stubPolicyProvider{
		setting: settings.OutboundPolicySetting{AllowHosts: hosts},
	})
	t.Cleanup(restore)
}

func testLibraryPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func testLibraryAsset() *domain.V1Asset {
	const providerURL = "https://images.unsplash.com/photo-test-1"
	return &domain.V1Asset{
		WorkspacePublicID: "ws_library_test",
		PublicID:          "lasset_testmaterial01",
		Source:            domain.AssetSourceExternal,
		Provider:          domain.ProviderUnsplash,
		SourceID:          "photo-test-1",
		SourceURL:         providerURL,
		PreviewURL:        providerURL,
		OriginalURL:       providerURL,
		Title:             "dunes",
		Author:            "a",
		Width:             &testMaterialWidth,
		Height:            &testMaterialHeight,
	}
}

var (
	testMaterialWidth  int32 = 100
	testMaterialHeight int32 = 80
)

func newLibraryMaterializerForTest(t *testing.T, fetcher *stubLibraryFetcher, replay *fakeLibraryReplay) (*LibraryMaterializer, *workspacemedia.InMemoryWorkspaceMediaStore, *fakeLibraryObjects) {
	t.Helper()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeLibraryObjects{}
	deps := LibraryMaterializerDeps{Media: media, Objects: objects, Fetcher: fetcher}
	if replay != nil {
		deps.Replay = replay
	}
	materializer, err := NewLibraryMaterializer(deps)
	if err != nil {
		t.Fatalf("NewLibraryMaterializer: %v", err)
	}
	return materializer, media, objects
}

func libraryAssetCount(t *testing.T, media *workspacemedia.InMemoryWorkspaceMediaStore, workspace string) int {
	t.Helper()
	listed, err := media.ListAssets(context.Background(), workspace, 100, 0)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	return len(listed)
}

func TestNewLibraryMaterializerFailsFast(t *testing.T) {
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeLibraryObjects{}
	fetcher := &stubLibraryFetcher{}
	if _, err := NewLibraryMaterializer(LibraryMaterializerDeps{Objects: objects, Fetcher: fetcher}); err == nil {
		t.Fatal("nil media must fail fast")
	}
	if _, err := NewLibraryMaterializer(LibraryMaterializerDeps{Media: media, Fetcher: fetcher}); err == nil {
		t.Fatal("nil objects must fail fast")
	}
	if _, err := NewLibraryMaterializer(LibraryMaterializerDeps{Media: media, Objects: objects}); err == nil {
		t.Fatal("nil fetcher must fail fast")
	}
	if _, err := NewLibraryMaterializer(LibraryMaterializerDeps{Media: media, Objects: objects, Fetcher: fetcher}); err != nil {
		t.Fatalf("full deps: %v", err)
	}
}

func TestLibraryMaterializeHappyPath(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	pngBytes := testLibraryPNG(t)
	fetcher := &stubLibraryFetcher{data: pngBytes}
	materializer, media, objects := newLibraryMaterializerForTest(t, fetcher, nil)

	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	mediaID, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset())
	if err != nil {
		t.Fatalf("MaterializeWorkspaceAsset: %v", err)
	}
	if !strings.HasPrefix(mediaID, "asset_") {
		t.Fatalf("media id = %q, want asset_...", mediaID)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
	stored := objects.storedBytes()
	if len(stored) != 1 || !bytes.Equal(stored[0], pngBytes) {
		t.Fatalf("object store writes = %d, want 1 exact byte copy", len(stored))
	}
	stored_asset, err := media.GetAsset(context.Background(), "ws_library_test", mediaID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if stored_asset.MIMEType != "image/png" || stored_asset.Width != 2 || stored_asset.Height != 2 {
		t.Fatalf("asset facts = %+v, want image/png 2x2", stored_asset)
	}
	if stored_asset.ChecksumSHA256 == "" || stored_asset.SizeBytes != int64(len(pngBytes)) {
		t.Fatalf("asset integrity = %+v, want checksum + size", stored_asset)
	}
}

func TestLibraryMaterializeProviderDeniedFailsClosed(t *testing.T) {
	stubLibraryEgress(t, []string{"evil.example", "images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	materializer, media, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	asset := testLibraryAsset()
	asset.SourceURL = "https://evil.example/x.png"
	asset.PreviewURL = "https://evil.example/x.png"
	asset.OriginalURL = "https://evil.example/x.png"
	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	if _, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", asset); err == nil {
		t.Fatal("non-allowlisted provider URL must fail closed")
	} else if !strings.Contains(err.Error(), "no fetchable") {
		t.Fatalf("err = %v, want no-fetchable-URL failure", err)
	}
	if got := fetcher.callCount(); got != 0 {
		t.Fatalf("fetch calls = %d, want 0 (never fetched)", got)
	}
	if got := libraryAssetCount(t, media, "ws_library_test"); got != 0 {
		t.Fatalf("media assets = %d, want 0", got)
	}
}

func TestLibraryMaterializeMediaSourceDenied(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	materializer, _, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	asset := testLibraryAsset()
	asset.Source = domain.AssetSourceMedia
	asset.Provider = ""
	asset.MediaAssetID = "asset_managed0000000001"
	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	if _, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", asset); err == nil {
		t.Fatal("media source must fail closed")
	} else if !strings.Contains(err.Error(), "cannot be fetched") {
		t.Fatalf("err = %v, want cannot-be-fetched failure", err)
	}
	if got := fetcher.callCount(); got != 0 {
		t.Fatalf("fetch calls = %d, want 0", got)
	}
}

func TestLibraryMaterializeIgnoresOperatorPolicy(t *testing.T) {
	// plus.unsplash.com is in the code-owned provider world but absent
	// from the operator egress policy: the materializer must fetch anyway
	// (operator policy no longer consulted for provider fetches).
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	materializer, media, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	asset := testLibraryAsset()
	asset.SourceURL = "https://plus.unsplash.com/photo-test-1"
	asset.PreviewURL = "https://plus.unsplash.com/photo-test-1"
	asset.OriginalURL = "https://plus.unsplash.com/photo-test-1"
	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	mediaID, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", asset)
	if err != nil {
		t.Fatalf("code-owned provider host must fetch: %v", err)
	}
	if !strings.HasPrefix(mediaID, "asset_") {
		t.Fatalf("media id = %q, want asset_...", mediaID)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
	if got := libraryAssetCount(t, media, "ws_library_test"); got != 1 {
		t.Fatalf("media assets = %d, want 1", got)
	}
}

func TestLibraryMaterializeNeedsNoEgressPolicy(t *testing.T) {
	// No operator policy configured at all: provider fetches proceed on
	// the code-owned host list without touching the network policy.
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	materializer, _, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	mediaID, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset())
	if err != nil {
		t.Fatalf("empty operator policy must not block provider fetch: %v", err)
	}
	if !strings.HasPrefix(mediaID, "asset_") {
		t.Fatalf("media id = %q, want asset_...", mediaID)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
}

func TestLibraryMaterializeFetchFailureCreatesNoAsset(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{err: errors.New("boom")}
	materializer, media, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	if _, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset()); err == nil {
		t.Fatal("fetch failure must surface")
	} else if !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("err = %v, want download-failed failure", err)
	}
	if got := libraryAssetCount(t, media, "ws_library_test"); got != 0 {
		t.Fatalf("media assets = %d, want 0", got)
	}
}

func TestLibraryMaterializeOwnerMissingFailsClosed(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	materializer, media, _ := newLibraryMaterializerForTest(t, fetcher, nil)

	if _, err := materializer.MaterializeWorkspaceAsset(context.Background(), "ws_library_test", testLibraryAsset()); err == nil {
		t.Fatal("missing owner identity must fail closed")
	} else if !strings.Contains(err.Error(), "owner identity") {
		t.Fatalf("err = %v, want owner-identity failure", err)
	}
	if got := fetcher.callCount(); got != 0 {
		t.Fatalf("fetch calls = %d, want 0", got)
	}
	if got := libraryAssetCount(t, media, "ws_library_test"); got != 0 {
		t.Fatalf("media assets = %d, want 0", got)
	}
}

func TestLibraryMaterializeReplayReturnsSameRefWithoutRefetch(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	replay := &fakeLibraryReplay{}
	materializer, _, _ := newLibraryMaterializerForTest(t, fetcher, replay)

	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	first, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset())
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	replay.seed("ws_library_test", "lasset_testmaterial01", first)
	second, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset())
	if err != nil {
		t.Fatalf("replay materialize: %v", err)
	}
	if second != first {
		t.Fatalf("replay id = %q, want %q", second, first)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1 (replay must not refetch)", got)
	}
}

func TestLibraryMaterializeReplayMalformedFailsClosed(t *testing.T) {
	stubLibraryEgress(t, []string{"images.unsplash.com"})
	fetcher := &stubLibraryFetcher{data: testLibraryPNG(t)}
	replay := &fakeLibraryReplay{}
	materializer, _, _ := newLibraryMaterializerForTest(t, fetcher, replay)
	replay.seed("ws_library_test", "lasset_testmaterial01", "not-an-asset-id")

	ctx := WithLibraryMaterializationOwner(context.Background(), 42)
	if _, err := materializer.MaterializeWorkspaceAsset(ctx, "ws_library_test", testLibraryAsset()); err == nil {
		t.Fatal("malformed replay hit must fail closed")
	} else if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("err = %v, want malformed-replay failure", err)
	}
	if got := fetcher.callCount(); got != 0 {
		t.Fatalf("fetch calls = %d, want 0", got)
	}
}
