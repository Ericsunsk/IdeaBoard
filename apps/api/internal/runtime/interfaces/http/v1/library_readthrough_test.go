package v1

// Library materialization read-through tests: miss -> trigger ->
// found over GET media-assets/:asset_id with an lasset_ id, plus
// in-flight dedup, unknown-asset convergence, and failure records.
// The trigger runs inline here (deterministic); one test keeps the
// production async spawn to prove no duplicate job fires while a
// fetch is in flight.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	libraryapplication "ideaboard/api/internal/library/application"
	librarydomain "ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	materializerassets "ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type readThroughStubFetcher struct {
	mu    sync.Mutex
	calls int
	data  []byte
	err   error
}

func (f *readThroughStubFetcher) FetchSourceBytes(context.Context, string, []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]byte(nil), f.data...), nil
}

func (f *readThroughStubFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type readThroughBlockingFetcher struct {
	mu      sync.Mutex
	calls   int
	data    []byte
	release chan struct{}
}

func (f *readThroughBlockingFetcher) FetchSourceBytes(context.Context, string, []string) ([]byte, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	<-f.release
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.data...), nil
}

func (f *readThroughBlockingFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type readThroughPolicyProvider struct {
	hosts []string
}

func (p *readThroughPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func stubReadThroughEgress(t *testing.T, hosts []string) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(&readThroughPolicyProvider{hosts: hosts})
	t.Cleanup(restore)
}

func readThroughPNGBytes(t *testing.T) []byte {
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

type v1ReadThroughFixture struct {
	engine  *gin.Engine
	lib     *librarymemory.InMemoryWorkspaceLibraryStore
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeV1MediaObjects
	ws      *tenancy.Workspace
	owner   int64
	member  int64
}

func newV1ReadThroughFixture(t *testing.T, fetcher interface {
	FetchSourceBytes(context.Context, string, []string) ([]byte, error)
}, inlineSpawn bool) *v1ReadThroughFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	const owner, member = 11, 12
	dir.ensure(owner)
	dir.ensure(member)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "readthrough", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	// Composition-equivalent wiring: the typed read-through service
	// owns lifecycle + replay; Media owns download + writes. The stub
	// fetcher rides a real LibraryMaterializer so the tests keep
	// covering the production download->ingest path with fake bytes.
	replay := &libraryapplication.SourceResolver{Store: lib}
	rawMaterializer, err := materializerassets.NewLibraryMaterializer(materializerassets.LibraryMaterializerDeps{
		Media: media, Objects: objects, Fetcher: fetcher, Replay: replay,
	})
	if err != nil {
		t.Fatalf("NewLibraryMaterializer: %v", err)
	}
	var executor libraryapplication.Executor = libraryapplication.GoroutineExecutor{}
	if inlineSpawn {
		executor = libraryapplication.SpawnFunc(func(fn func()) { fn() })
	}
	readThrough := libraryapplication.NewReadThroughService(lib, rawMaterializer, executor, 0)
	if readThrough == nil {
		t.Fatal("NewReadThroughService returned nil for test deps")
	}
	deps := ContractV1Deps{Media: media, MediaObjects: objects, Library: ContractV1LibraryDeps{Store: lib, ReadThrough: readThrough}}
	engine := newV1MediaTestEngine(store, dir, deps)
	return &v1ReadThroughFixture{engine: engine, lib: lib, media: media, objects: objects, ws: ws, owner: owner, member: member}
}

func (f *v1ReadThroughFixture) saveRemoteAsset(t *testing.T, sourceID string) string {
	t.Helper()
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	providerURL := "https://images.unsplash.com/" + sourceID
	saved, created, err := f.lib.SaveAsset(context.Background(), f.ws.PublicID, librarydomain.V1SaveAssetInput{
		Asset: librarydomain.AssetInput{
			Provider: librarydomain.ProviderUnsplash, SourceID: sourceID,
			SourceURL: providerURL, PreviewURL: providerURL, OriginalURL: providerURL,
			Width: 100, Height: 80,
		},
	}, publicID)
	if err != nil || !created || saved == nil {
		t.Fatalf("SaveAsset = %+v %v %v, want created remote ref", saved, created, err)
	}
	return saved.PublicID
}

func (f *v1ReadThroughFixture) mediaPath(assetID string) string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets/" + assetID
}

func (f *v1ReadThroughFixture) jobs(t *testing.T) []*librarydomain.V1Materialization {
	t.Helper()
	all, err := f.lib.ListMaterializations(context.Background(), f.ws.PublicID)
	if err != nil {
		t.Fatalf("ListMaterializations: %v", err)
	}
	return all
}

func (f *v1ReadThroughFixture) pollJobs(t *testing.T, want func([]*librarydomain.V1Materialization) bool) []*librarydomain.V1Materialization {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		all := f.jobs(t)
		if want(all) {
			return all
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for jobs, have %d", len(all))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestV1LibraryUploadedMediaImport(t *testing.T) {
	fetcher := &readThroughStubFetcher{}
	f := newV1ReadThroughFixture(t, fetcher, true)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 240, 360))); err != nil {
		t.Fatal(err)
	}
	staged := imageBytes.Bytes()
	uploadsPath := "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
	uploadID := v1GapsDeclare(t, f.engine, uploadsPath, f.owner, "library-import-declare", "本地图片.png", staged)
	v1GapsStage(t, f.engine, uploadsPath+"/"+uploadID+"/bytes", staged, f.owner, "library-import-stage")
	status, _, payload := doV1(t, f.engine, v1Call{
		method: "POST", path: uploadsPath + "/" + uploadID + "/complete", userID: f.owner,
		body:    fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"mime_type":"image/png","media_kind":"image"}}`, v1GapsChecksum(staged), len(staged)),
		headers: map[string]string{"Idempotency-Key": "library-import-complete"},
	})
	if status != http.StatusOK {
		t.Fatalf("complete image upload = %d %v, want 200", status, payload)
	}
	mediaID := v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])
	f.lib.RegisterManagedMediaForTests(f.ws.PublicID, mediaID, nil, nil, false)
	status, _, payload = doV1(t, f.engine, v1Call{
		method: "POST", path: "/api/v1/workspaces/" + f.ws.PublicID + "/library/assets", userID: f.owner,
		body:    fmt.Sprintf(`{"data":{"source":"media","media_asset_id":%q}}`, mediaID),
		headers: map[string]string{"Idempotency-Key": "library-import-save"},
	})
	if status != http.StatusCreated {
		t.Fatalf("save uploaded media = %d %v, want 201", status, payload)
	}
	// #936: the managed save publishes the asset to workspace members; seed
	// the existence fact on the media seam (twin of the library_assets row).
	f.media.SeedLibraryLinkForTests(f.ws.PublicID, mediaID)
	libraryID := v1Data(t, payload).(map[string]any)["id"].(string)
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(libraryID), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("import uploaded Library image = %d %v, want 200", status, payload)
	}
	dto := v1Data(t, payload).(map[string]any)
	if dto["id"] != mediaID {
		t.Fatalf("imported Media ID = %v, want existing %s", dto["id"], mediaID)
	}
	if dto["width"] != float64(240) || dto["height"] != float64(360) {
		t.Fatalf("imported image dimensions = %v x %v, want 240 x 360", dto["width"], dto["height"])
	}
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: dto["content_url"].(string), userID: f.member}, "library-import-")
	if status != http.StatusOK || body != string(staged) {
		t.Fatalf("imported image content = %d len %d, want uploaded bytes", status, len(body))
	}
	if len(f.jobs(t)) != 0 || fetcher.callCount() != 0 {
		t.Fatal("managed import triggered external materialization")
	}
}

// TestV1LibraryReadThroughMissTriggerFound proves the end-to-end
// wiring: a freshly saved remote ref 404s on the media read (miss),
// the trigger fetches inline and records a completed job, and the
// next read serves the managed asset (found) with asset_ shape.
func TestV1LibraryReadThroughMissTriggerFound(t *testing.T) {
	stubReadThroughEgress(t, []string{"images.unsplash.com"})
	fetcher := &readThroughStubFetcher{data: readThroughPNGBytes(t)}
	f := newV1ReadThroughFixture(t, fetcher, true)
	lassetID := f.saveRemoteAsset(t, "photo-readthrough-1")

	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("first read = %d %v, want 404 media_not_found (miss triggers async fetch)", status, payload)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1 (miss triggers exactly one fetch)", got)
	}
	all := f.jobs(t)
	if len(all) != 1 || all[0].Status != librarydomain.V1MaterializationCompleted || !contract.IsAssetID(all[0].MediaAssetID) {
		t.Fatalf("jobs = %+v, want one completed job linked to an asset_ id", all)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("second read = %d %v, want 200 (found)", status, payload)
	}
	dto, ok := v1Data(t, payload).(map[string]any)
	if !ok {
		t.Fatalf("dto = %T, want object", v1Data(t, payload))
	}
	mediaID, _ := dto["id"].(string)
	if mediaID != all[0].MediaAssetID {
		t.Fatalf("dto id = %q, want linked %q", mediaID, all[0].MediaAssetID)
	}
	contentURL, _ := dto["content_url"].(string)
	if !strings.Contains(contentURL, mediaID+"/content") {
		t.Fatalf("content_url = %q, want asset_-based delivery URL", contentURL)
	}
	if width, _ := dto["width"].(float64); width != 2 {
		t.Fatalf("width = %v, want 2 (fetched bytes ingested)", width)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1 (completed link replays without refetch)", got)
	}
}

// TestV1LibraryReadThroughUnknownAsset proves unknown and malformed
// ids 404 without creating jobs or fetching.
func TestV1LibraryReadThroughUnknownAsset(t *testing.T) {
	stubReadThroughEgress(t, []string{"images.unsplash.com"})
	fetcher := &readThroughStubFetcher{data: readThroughPNGBytes(t)}
	f := newV1ReadThroughFixture(t, fetcher, true)

	missing, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(missing), userID: f.member})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("unknown lasset read = %d %v, want 404 media_not_found", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath("nope"), userID: f.member})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("malformed read = %d %v, want 404 media_not_found", status, payload)
	}
	if all := f.jobs(t); len(all) != 0 {
		t.Fatalf("jobs = %d, want 0 (no trigger without an owned asset)", len(all))
	}
	if got := fetcher.callCount(); got != 0 {
		t.Fatalf("fetch calls = %d, want 0", got)
	}
}

// TestV1LibraryReadThroughDedupsInFlight proves a second read while
// the fetch is still running 404s without recording a duplicate job,
// and the read resolves once the fetch lands.
func TestV1LibraryReadThroughDedupsInFlight(t *testing.T) {
	stubReadThroughEgress(t, []string{"images.unsplash.com"})
	fetcher := &readThroughBlockingFetcher{data: readThroughPNGBytes(t), release: make(chan struct{})}
	f := newV1ReadThroughFixture(t, fetcher, false)
	lassetID := f.saveRemoteAsset(t, "photo-readthrough-2")

	status, _, _ := doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("first read = %d, want 404 while fetch runs", status)
	}
	f.pollJobs(t, func(all []*librarydomain.V1Materialization) bool { return len(all) == 1 })

	status, _, _ = doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("second read = %d, want 404 while fetch runs", status)
	}
	if all := f.jobs(t); len(all) != 1 {
		t.Fatalf("jobs = %d, want 1 (in-flight job suppresses a duplicate trigger)", len(all))
	}

	close(fetcher.release)
	completed := f.pollJobs(t, func(all []*librarydomain.V1Materialization) bool {
		return len(all) == 1 && all[0].Status == librarydomain.V1MaterializationCompleted
	})
	if !contract.IsAssetID(completed[0].MediaAssetID) {
		t.Fatalf("completed link = %q, want asset_ shape", completed[0].MediaAssetID)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("third read = %d %v, want 200 after fetch lands", status, payload)
	}
	if got := fetcher.callCount(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
}

// TestV1LibraryReadThroughFetchFailureRecordsFailed proves a failed
// fetch folds into a failed job (fixed message, no error_code
// persistence beyond the seam) and a later read retries with a new
// job instead of resurrecting the terminal one.
func TestV1LibraryReadThroughFetchFailureRecordsFailed(t *testing.T) {
	stubReadThroughEgress(t, []string{"images.unsplash.com"})
	fetcher := &readThroughStubFetcher{err: errors.New("provider down")}
	f := newV1ReadThroughFixture(t, fetcher, true)
	lassetID := f.saveRemoteAsset(t, "photo-readthrough-3")

	status, _, _ := doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("first read = %d, want 404 (fetch fails async)", status)
	}
	all := f.jobs(t)
	if len(all) != 1 || all[0].Status != librarydomain.V1MaterializationFailed {
		t.Fatalf("jobs = %+v, want one failed job", all)
	}
	if all[0].ErrorMessage != "素材转存失败，请稍后重试" {
		t.Fatalf("error message = %q, want fixed retry message", all[0].ErrorMessage)
	}

	status, _, _ = doV1(t, f.engine, v1Call{method: "GET", path: f.mediaPath(lassetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("second read = %d, want 404 (failed history allows a new attempt)", status)
	}
	all = f.jobs(t)
	if len(all) != 2 {
		t.Fatalf("jobs = %d, want 2 (retry records a new job, terminal records stay immutable)", len(all))
	}
	for _, job := range all {
		if job.Status != librarydomain.V1MaterializationFailed {
			t.Fatalf("job %+v, want failed", job)
		}
	}
}
