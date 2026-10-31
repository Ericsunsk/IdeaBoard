package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/users/domain"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1tenancy "ideaboard/api/internal/runtime/interfaces/http/v1/tenancy"
	v1testsupport "ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type ContractV1Deps = Deps

// ---- merged from (base media_test.go) ----
// Workspace upload + media contract tests (issue #803, restored in
// #820 as Phase 4-03 of #809).
//
// Reuses the v1 envelope helpers from v1/test.go (doV1
// asserts the Request-Id echo and JSON envelope shapes; doV1RoundTrip
// covers the non-JSON binary endpoints). Covers the 8-endpoint
// lifecycle (create -> get -> complete -> metadata -> bytes ->
// derivatives -> archive), completion-mismatch failure, idempotency
// replay/conflict, the 404-vs-403 matrix, binary plain-status
// behavior, and fail-closed behavior without a store.

// fakeV1MediaObjects is a map-backed WorkspaceMediaObjects fake. When
// serveAny is set every key reads back canned bytes (the no-byte-PUT
// backend is stubbed); otherwise only stored keys resolve and the rest
// report ErrStoredObjectNotFound.
type fakeV1MediaObjects struct {
	mu       sync.Mutex
	objects  map[string][]byte
	serveAny bool
}

func (f *fakeV1MediaObjects) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data, ok := f.objects[objectKey]; ok && len(data) > 0 {
		return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: append([]byte(nil), data...)}, nil
	}
	if f.serveAny {
		return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: []byte("fake-png-bytes")}, nil
	}
	return nil, storagedomain.ErrStoredObjectNotFound
}

func (f *fakeV1MediaObjects) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[objectKey] = append([]byte(nil), data...)
	return nil
}

// StoreBytesRef honors an explicit ObjectKey (the avatar handler always
// passes the canonical content-hashed key); without one it fails closed
// like a miswired caller.
func (f *fakeV1MediaObjects) StoreBytesRef(ctx context.Context, filename string, data []byte, opts storageports.StorageStoreOptions) (*storageports.StoredUploadRef, error) {
	if strings.TrimSpace(opts.ObjectKey) == "" {
		return nil, storagedomain.ErrStoredObjectNotFound
	}
	if err := f.PutObjectAtProfile(ctx, "", opts.ObjectKey, data, "image/png"); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return &storageports.StoredUploadRef{
		StorageKey:     strings.TrimSpace(opts.ObjectKey),
		MIMEType:       "image/png",
		SizeBytes:      int64(len(data)),
		ChecksumSHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func (f *fakeV1MediaObjects) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, objectKey)
	return nil
}

type v1MediaFixture struct {
	store   *tenancy.InMemoryStore
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeV1MediaObjects
	dir     *staticUserDirectory
	engine  *gin.Engine
	ws      *tenancy.Workspace
	ws2     *tenancy.Workspace
	owner   int64
	member  int64
	other   int64
}

func newV1MediaTestEngine(store tenancy.Store, dir *staticUserDirectory, deps ContractV1Deps, withGlobalMiddleware ...bool) *gin.Engine {
	var userDir *v1testsupport.StaticUserDirectory
	if dir != nil {
		userDir = dir.StaticUserDirectory
	}
	router := v1testsupport.NewTestEngine(userDir)
	if len(withGlobalMiddleware) > 0 && withGlobalMiddleware[0] {
		runtimehttp.RegisterRoutes(router, runtimehttp.Dependencies{
			CORSMiddleware: func(c *gin.Context) { c.Next() },
		}, nil, runtimehttp.FrontendRouteOptions{})
	}
	deps.Store = store
	if deps.Idempotency == nil {
		deps.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	handler := NewHandler(deps)
	handler.Mount(v1)
	handler.MountPublic(v1)
	tenancyHandler := v1tenancy.NewHandler(v1tenancy.Deps{
		Store:       store,
		Users:       userDir,
		Idempotency: deps.Idempotency,
		UserProfile: deps.UserProfile,
	})
	tenancyHandler.Mount(v1)
	return router
}

func newV1MediaFixture(t *testing.T, withGlobalMiddleware ...bool) *v1MediaFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte), serveAny: true}
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	ws2, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws2.PublicID, owner, tenancy.RoleMember)
	deps := ContractV1Deps{Media: ContractV1MediaDeps{Media: media, MediaObjects: objects, MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), nil)}}
	engine := newV1MediaTestEngine(store, dir, deps, withGlobalMiddleware...)
	return &v1MediaFixture{store: store, media: media, objects: objects, dir: dir, engine: engine, ws: ws, ws2: ws2, owner: owner, member: member, other: other}
}

// v1TestImgproxyConfig provides a signed renderer that echoes its requested path.
func v1TestImgproxyConfig(t *testing.T) imgproxy.Config {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	t.Cleanup(upstream.Close)
	return imgproxy.Config{BaseURL: upstream.URL, KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
}

// assertImgproxyImage verifies the API response and signed internal render request.
func assertImgproxyImage(t *testing.T, engine *gin.Engine, cfg imgproxy.Config, call v1Call, reqIDPrefix, label, wantCacheControl, wantGeometrySubstr string) {
	t.Helper()
	before := time.Now()
	status, header, body := doV1RoundTrip(t, engine, call, reqIDPrefix)
	if status != http.StatusOK || header.Get("Location") != "" || header.Get("Content-Type") != "image/webp" {
		t.Fatalf("image %s = %d %v", label, status, header)
	}
	if cc := header.Get("Cache-Control"); cc != wantCacheControl {
		t.Fatalf("image %s Cache-Control = %q, want %q", label, cc, wantCacheControl)
	}
	sig, signedPath, ok := strings.Cut(strings.TrimPrefix(body, "/"), "/")
	if !ok {
		t.Fatalf("image %s did not reach renderer: %q", label, body)
	}
	signedPath = "/" + signedPath
	wantSig, err := cfg.SignPath(signedPath)
	if err != nil || sig != wantSig {
		t.Fatalf("image %s invalid signature", label)
	}
	if !strings.Contains(signedPath, wantGeometrySubstr) {
		t.Fatalf("image %s path = %q, want geometry %q", label, signedPath, wantGeometrySubstr)
	}
	if !strings.HasPrefix(signedPath, "/exp:") {
		t.Fatal("missing render expiry")
	}
	expUnix, err := strconv.ParseInt(strings.SplitN(strings.TrimPrefix(signedPath, "/exp:"), "/", 2)[0], 10, 64)
	if err != nil || expUnix < before.Add(4*time.Minute).Unix() || expUnix > time.Now().Add(5*time.Minute+time.Second).Unix() {
		t.Fatalf("image %s invalid expiry: %d", label, expUnix)
	}
}

func assertV1DisplayTierImage(t *testing.T, engine *gin.Engine, cfg imgproxy.Config, path string, userID int64, tier string, side int) {
	t.Helper()
	assertImgproxyImage(t, engine, cfg,
		v1Call{method: "GET", path: path, userID: userID}, "test-media-",
		tier, "private, max-age=300",
		fmt.Sprintf("/rs:fit:%d:%d/", side, side))
}

const v1MediaTestChecksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func (f *v1MediaFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

func v1NewUploadBody() string {
	return fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, v1MediaTestChecksum)
}

func v1MediaUploadID(t *testing.T, data any) string {
	t.Helper()
	dto, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("upload dto = %T, want object", data)
	}
	id, _ := dto["id"].(string)
	if !contract.IsUploadID(id) {
		t.Fatalf("upload id = %q, want upload_ shape", id)
	}
	return id
}

func v1MediaAssetID(t *testing.T, data any) string {
	t.Helper()
	dto, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("asset dto = %T, want object", data)
	}
	id, _ := dto["id"].(string)
	if !contract.IsAssetID(id) {
		t.Fatalf("asset id = %q, want asset_ shape", id)
	}
	return id
}

// TestV1MediaUploadLifecycle proves the endpoint lifecycle end to
// end: create -> get -> complete -> metadata -> bytes -> derivatives
// -> archive -> post-archive gates.
func TestV1MediaUploadLifecycle(t *testing.T) {
	f := newV1MediaFixture(t)
	staged := v1GapsPNGBytes(1024)
	checksum := v1GapsChecksum(staged)
	createBody := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, checksum)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: createBody, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-life-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.uploadsPath() + "/" + uploadID, userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("get = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "transferring" {
		t.Fatalf("session status = %q, want transferring after first read", got)
	}

	// Stage the declared bytes before completing (complete-after-bytes).
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.member, "media-life-stage")

	finishBody := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024,"width":100,"height":80}}`, checksum)
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: finishBody, userID: f.member, headers: map[string]string{"Idempotency-Key": "media-life-2"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	completeData, ok := v1Data(t, payload).(map[string]any)
	if !ok {
		t.Fatalf("complete data = %T, want object", v1Data(t, payload))
	}
	assetDTO, ok := completeData["asset"].(map[string]any)
	if !ok {
		t.Fatalf("complete asset = %T, want object", completeData["asset"])
	}
	assetID := v1MediaAssetID(t, assetDTO)
	if got, _ := assetDTO["status"].(string); got != "ready" {
		t.Fatalf("asset status = %q, want ready", got)
	}
	if got, _ := assetDTO["checksum_sha256"].(string); got != checksum {
		t.Fatalf("asset checksum = %q", got)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get asset = %d %v, want 200", status, payload)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	items, ok := v1Data(t, payload).([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list items = %v, want 1 asset", v1Data(t, payload))
	}

	// Bytes never ride the JSON envelope: plain 200 with the staged bytes.
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.owner}, "test-media-")
	if status != http.StatusOK || body != string(staged) {
		t.Fatalf("content = %d len %d, want 200 staged bytes len %d", status, len(body), len(staged))
	}
	// Display tiers render on demand after the asset is ready.
	assertV1DisplayTierImage(t, f.engine, v1TestImgproxyConfig(t),
		f.assetsPath()+"/"+assetID+"/derivatives/thumb", f.owner,
		derivativedomain.TierThumb, derivativedomain.TierSpecs[derivativedomain.TierThumb].MaxSide)

	// Archive needs owner/admin: the member is forbidden.
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/" + assetID + "/archive", userID: f.member, headers: map[string]string{"Idempotency-Key": "media-life-3"}})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceForbidden) {
		t.Fatalf("member archive = %d %v, want 403 workspace_forbidden", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/" + assetID + "/archive", userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-life-3"}})
	if status != http.StatusOK {
		t.Fatalf("archive = %d %v, want 200", status, payload)
	}
	archivedDTO, ok := v1Data(t, payload).(map[string]any)
	if !ok || archivedDTO["archived_at"] == nil {
		t.Fatalf("archived dto = %v, want archived_at marker", v1Data(t, payload))
	}

	// Post-archive gates: metadata stays readable, bytes go 410.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get archived = %d %v, want 200", status, payload)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.owner}, "test-media-")
	if status != http.StatusGone {
		t.Fatalf("archived content = %d, want 410", status)
	}
	// The archive gate precedes rendering: derivatives go 410 too.
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.owner}, "test-media-")
	if status != http.StatusGone {
		t.Fatalf("archived derivative = %d, want 410", status)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("list after archive = %d %v, want 200", status, payload)
	}
	if items, _ := v1Data(t, payload).([]any); len(items) != 0 {
		t.Fatalf("list after archive = %v, want empty", items)
	}
}

func TestV1MediaCompleteMismatch(t *testing.T) {
	f := newV1MediaFixture(t)
	staged := v1GapsPNGBytes(1024)
	checksum := v1GapsChecksum(staged)
	createBody := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, checksum)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: createBody, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-mismatch-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.owner, "media-mismatch-stage")

	badBody := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":2048,"width":100,"height":80}}`, checksum)
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: badBody, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-mismatch-2"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("mismatch complete = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// The session failed terminally and stays readable as failed.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.uploadsPath() + "/" + uploadID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get failed session = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "failed" {
		t.Fatalf("session status = %q, want failed", got)
	}
}

func TestV1MediaIdempotency(t *testing.T) {
	f := newV1MediaFixture(t)

	first := v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-idem-1"}}
	status, _, payload := doV1(t, f.engine, first)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	firstID := v1MediaUploadID(t, v1Data(t, payload))

	status, _, payload = doV1(t, f.engine, first)
	if status != http.StatusCreated {
		t.Fatalf("replay = %d %v, want 201", status, payload)
	}
	if got := v1MediaUploadID(t, v1Data(t, payload)); got != firstID {
		t.Fatalf("replay id = %q, want %q", got, firstID)
	}

	other := strings.Replace(v1NewUploadBody(), "photo.png", "other.png", 1)
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: other, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-idem-1"}})
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeUploadIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 upload_idempotency_conflict", status, payload)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner})
	if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, payload)
	}
}

func TestV1MediaAuthMatrix(t *testing.T) {
	f := newV1MediaFixture(t)

	// Anonymous callers get 401 on JSON endpoints, plain 401 on bytes.
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), headers: map[string]string{"Idempotency-Key": "media-auth-1"}})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d %v, want 401", status, payload)
	}

	// Outsiders converge to 404 workspace_not_found (never 403).
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.other, headers: map[string]string{"Idempotency-Key": "media-auth-2"}})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider create = %d %v, want 404 workspace_not_found", status, payload)
	}

	// Malformed shapes converge to resource 404s.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.uploadsPath() + "/nope", userID: f.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("malformed upload = %d %v, want 404 upload_not_found", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/nope", userID: f.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("malformed asset = %d %v, want 404 media_not_found", status, payload)
	}

	// Cross-workspace ids are invisible even to workspace members.
	authStagedForCreate := v1GapsPNGBytes(1024)
	authCreateBody := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, v1GapsChecksum(authStagedForCreate))
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: authCreateBody, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-auth-3"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	foreignUploads := "/api/v1/workspaces/" + f.ws2.PublicID + "/uploads"
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: foreignUploads + "/" + uploadID, userID: f.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("cross-workspace upload = %d %v, want 404 upload_not_found", status, payload)
	}

	// Unknown ids converge to the same 404s.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.uploadsPath() + "/upload_missing00000001", userID: f.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("unknown upload = %d %v, want 404 upload_not_found", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/asset_missing00000001", userID: f.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("unknown asset = %d %v, want 404 media_not_found", status, payload)
	}

	// Create + stage + complete an asset, then probe binary gates.
	stagedAuth := authStagedForCreate
	checksumAuth := v1GapsChecksum(stagedAuth)
	authFinish := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024,"width":100,"height":80}}`, checksumAuth)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", stagedAuth, f.owner, "media-auth-stage")
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: authFinish, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-auth-4"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	assetID := v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])

	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content"}, "test-media-")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous bytes = %d, want plain 401", status)
	}
	foreignAssets := "/api/v1/workspaces/" + f.ws2.PublicID + "/media-assets"
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: foreignAssets + "/" + assetID + "/content", userID: f.owner}, "test-media-")
	if status != http.StatusNotFound {
		t.Fatalf("cross-workspace bytes = %d, want plain 404", status)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/bogus", userID: f.owner}, "test-media-")
	if status != http.StatusBadRequest {
		t.Fatalf("unknown variant = %d, want plain 400", status)
	}
	// processing-input is the materialized tier, never a display tier.
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/processing-input", userID: f.owner}, "test-media-")
	if status != http.StatusBadRequest {
		t.Fatalf("processing-input variant = %d, want plain 400", status)
	}
}

func TestV1MediaDerivativeRendersOnDemand(t *testing.T) {
	f := newV1MediaFixture(t)

	// Declare with a stageable checksum (the fixed-checksum helper
	// cannot be staged: sha256 preimage).
	pendingStaged := v1GapsPNGBytes(1024)
	pendingChecksum := v1GapsChecksum(pendingStaged)
	pendingCreate := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, pendingChecksum)
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: pendingCreate, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-pending-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", pendingStaged, f.owner, "media-pending-stage")
	pendingFinish := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024,"width":100,"height":80}}`, pendingChecksum)
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: pendingFinish, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-pending-2"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	assetID := v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])

	assertV1DisplayTierImage(t, f.engine, v1TestImgproxyConfig(t),
		f.assetsPath()+"/"+assetID+"/derivatives/thumb", f.owner,
		derivativedomain.TierThumb, derivativedomain.TierSpecs[derivativedomain.TierThumb].MaxSide)
	// Original content is independent of the rendered display tiers.
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.owner}, "test-media-")
	if status != http.StatusOK || body != string(pendingStaged) {
		t.Fatalf("content = %d len %d, want 200 staged bytes len %d", status, len(body), len(pendingStaged))
	}
}

// TestV1MediaDerivativeWithoutImgproxyBinding proves degenerate wiring
// (no signing binding) fails closed with plain 409 on the existing
// media_not_ready surface — never unsigned URLs, never new codes.
// Production cannot reach this: composition fails closed at boot via
// validateImgproxyOrFatal.
func TestV1MediaDerivativeWithoutImgproxyBinding(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	const owner, member = 1, 2
	dir.ensure(owner)
	dir.ensure(member)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-nosign", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{Media: ContractV1MediaDeps{Media: media, MediaObjects: objects}})
	uploadsPath := "/api/v1/workspaces/" + ws.PublicID + "/uploads"
	assetsPath := "/api/v1/workspaces/" + ws.PublicID + "/media-assets"

	staged := v1GapsPNGBytes(512)
	uploadID := v1GapsDeclare(t, engine, uploadsPath, owner, "nosign-create", "photo.png", staged)
	v1GapsStage(t, engine, uploadsPath+"/"+uploadID+"/bytes", staged, member, "nosign-stage")
	assetID := v1GapsComplete(t, engine, uploadsPath+"/"+uploadID+"/complete", staged, member, "nosign-complete")

	status, _, _ := doV1RoundTrip(t, engine, v1Call{method: "GET",
		path: assetsPath + "/" + assetID + "/derivatives/thumb", userID: owner}, "test-media-")
	if status != http.StatusConflict {
		t.Fatalf("unsigned derivative = %d, want plain 409", status)
	}
}

// fakeV1StorageProfiler resolves a canned active storage profile for
// display-tier source-selection tests (fake profile/config, no network).
type fakeV1StorageProfiler struct {
	profile *storagedomain.StorageProfile
	err     error
}

func (f fakeV1StorageProfiler) ResolveActiveProfile(ctx context.Context) (*storagedomain.StorageProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}

// r2KeyMediaStore replays the in-memory media store but projects the
// completed asset onto a non-local key: the upload flow always reserves
// canonical uploads/ws/ slots, so only an override can place an R2-profile key
// behind a real asset id.
type r2KeyMediaStore struct {
	*workspacemedia.InMemoryWorkspaceMediaStore
	r2Key string
}

func (s r2KeyMediaStore) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacemedia.Asset, error) {
	asset, err := s.InMemoryWorkspaceMediaStore.GetAsset(ctx, workspaceID, assetID)
	if err != nil {
		return nil, err
	}
	clone := *asset
	clone.StorageKey = s.r2Key
	return &clone, nil
}

// AuthorizeAsset projects the completed asset onto the replayed R2 key,
// mirroring the GetAsset override: authorization still runs first (#936),
// only the served storage locator is rewritten.
func (s r2KeyMediaStore) AuthorizeAsset(ctx context.Context, workspaceID, assetID string, callerUserID int64, workspaceRole string, access workspacemedia.BoardAccess) (*workspacemedia.Asset, error) {
	asset, err := s.InMemoryWorkspaceMediaStore.AuthorizeAsset(ctx, workspaceID, assetID, callerUserID, workspaceRole, access)
	if err != nil {
		return nil, err
	}
	clone := *asset
	clone.StorageKey = s.r2Key
	return &clone, nil
}

// ListAuthorizedAssets projects every listed asset onto the replayed R2
// key, mirroring the GetAsset override.
func (s r2KeyMediaStore) ListAuthorizedAssets(ctx context.Context, workspaceID string, callerUserID int64, workspaceRole string, access workspacemedia.BoardAccess, limit, offset int) ([]*workspacemedia.Asset, error) {
	assets, err := s.InMemoryWorkspaceMediaStore.ListAuthorizedAssets(ctx, workspaceID, callerUserID, workspaceRole, access, limit, offset)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		asset.StorageKey = s.r2Key
	}
	return assets, nil
}

func TestV1MediaDerivativeUsesLocalProfile(t *testing.T) {
	const bucket = "test-r2-bucket"
	const r2Key = "tenant-migrated/photo.png"
	r2Profile := &storagedomain.StorageProfile{
		ID: "r2-profile", Name: "r2", Provider: storagedomain.StorageProviderS3Compatible,
		Enabled: true, IsCurrent: true, Config: map[string]interface{}{"bucket_name": bucket},
	}
	build := func(t *testing.T, profiler imgproxy.ProfileResolver, key string) (*gin.Engine, string, string) {
		t.Helper()
		ctx := context.Background()
		store := tenancy.NewInMemoryStoreForTests()
		dir := newStaticUserDirectory()
		media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
		objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
		const owner, member = 1, 2
		dir.ensure(owner)
		dir.ensure(member)
		ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-r2", OwnerUserID: owner})
		if err != nil {
			t.Fatal(err)
		}
		store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
		engine := newV1MediaTestEngine(store, dir, ContractV1Deps{
			Media: ContractV1MediaDeps{
				Media:         &r2KeyMediaStore{InMemoryWorkspaceMediaStore: media, r2Key: key},
				MediaObjects:  objects,
				MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), profiler),
			},
		})
		uploadsPath := "/api/v1/workspaces/" + ws.PublicID + "/uploads"
		assetsPath := "/api/v1/workspaces/" + ws.PublicID + "/media-assets"
		staged := v1GapsPNGBytes(512)
		uploadID := v1GapsDeclare(t, engine, uploadsPath, owner, "r2-src-create", "photo.png", staged)
		v1GapsStage(t, engine, uploadsPath+"/"+uploadID+"/bytes", staged, member, "r2-src-stage")
		assetID := v1GapsComplete(t, engine, uploadsPath+"/"+uploadID+"/complete", staged, member, "r2-src-complete")
		return engine, assetsPath, assetID
	}

	t.Run("workspace remains local with active s3 profile", func(t *testing.T) {
		const key = "uploads/ws/workspace/upload/image.png"
		engine, assetsPath, assetID := build(t, fakeV1StorageProfiler{profile: r2Profile}, key)
		assertImgproxyImage(t, engine, v1TestImgproxyConfig(t), v1Call{method: "GET", path: assetsPath + "/" + assetID + "/derivatives/thumb", userID: 1}, "local-media-", "workspace local", "private, max-age=300", "/plain/local:///"+key+"@webp")
	})

	t.Run("anything else stays 409", func(t *testing.T) {
		localProfile := &storagedomain.StorageProfile{
			ID: "builtin-local", Provider: storagedomain.StorageProviderLocal,
			Enabled: true, IsCurrent: true,
		}
		disabledS3 := &storagedomain.StorageProfile{
			ID: "r2-off", Provider: storagedomain.StorageProviderS3Compatible,
			Enabled: false, Config: map[string]interface{}{"bucket_name": bucket},
		}
		bucketlessS3 := &storagedomain.StorageProfile{
			ID: "r2-nobucket", Provider: storagedomain.StorageProviderS3Compatible,
			Enabled: true, IsCurrent: true,
		}
		for _, tc := range []struct {
			name     string
			profiler imgproxy.ProfileResolver
			key      string
		}{
			{"no resolver wired", nil, r2Key},
			{"resolver failure", fakeV1StorageProfiler{err: context.DeadlineExceeded}, r2Key},
			{"local active profile", fakeV1StorageProfiler{profile: localProfile}, r2Key},
			{"disabled s3 profile", fakeV1StorageProfiler{profile: disabledS3}, r2Key},
			{"s3 profile without bucket", fakeV1StorageProfiler{profile: bucketlessS3}, r2Key},
			{"traversal key under s3 profile", fakeV1StorageProfiler{profile: r2Profile}, "../evil.png"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				engine, assetsPath, assetID := build(t, tc.profiler, tc.key)
				status, _, _ := doV1RoundTrip(t, engine, v1Call{method: "GET",
					path: assetsPath + "/" + assetID + "/derivatives/thumb", userID: 1}, "test-media-")
				if status != http.StatusConflict {
					t.Fatalf("%s derivative = %d, want plain 409", tc.name, status)
				}
			})
		}
	})
}

func TestV1MediaFailClosedWithoutStore(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	dir.ensure(1)
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{})

	status, _, payload := doV1(t, engine, v1Call{method: "POST", path: "/api/v1/workspaces/ws_missing00000001/uploads", body: v1NewUploadBody(), userID: 1, headers: map[string]string{"Idempotency-Key": "media-closed-1"}})
	if status != http.StatusNotImplemented || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("create without store = %d %v, want 501", status, payload)
	}
	status, _, _ = doV1RoundTrip(t, engine, v1Call{method: "GET", path: "/api/v1/workspaces/ws_missing00000001/media-assets/asset_missing00000001/content", userID: 1}, "test-media-")
	if status != http.StatusNotImplemented {
		t.Fatalf("bytes without store = %d, want plain 501", status)
	}
}

func TestV1MediaValidation(t *testing.T) {
	f := newV1MediaFixture(t)

	badMime := strings.Replace(v1NewUploadBody(), "image/png", "image/heic", 1)
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: badMime, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-valid-1"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("bad mime = %d %v, want 422 upload_validation_failed", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: `{"data":{"filename":"x"}}`, userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-valid-2"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("short declaration = %d %v, want 422", status, payload)
	}
}

// ---- merged from media_gaps_test.go ----
// Backend gap batch for #809-close: media/upload/avatar/vision-query
// coverage (declare -> PUT bytes -> complete -> asset, avatar,
// batch metadata, vision file/external refs, incomplete-complete
// rejection). Uses serveAny=false byte fakes so unstaged slots
// genuinely miss; the permissive serveAny fixture in
// v1/media_test.go cannot prove the complete-after-bytes
// gate.

// fakeAvatarUID is a deterministic V1AvatarUIDDirectory for tests.
type fakeAvatarUID struct {
	uid          string
	err          error
	avatar       string
	avatarUserID int64
	writeErr     error
	writes       int
}

// errFakeAvatarUserNotFound is the fixture's unknown-uid signal. It
// aliases the platform not-found sentinel so the public avatar route
// converges it to 404 exactly like production.
var errFakeAvatarUserNotFound = database.ErrNotFound

func (f *fakeAvatarUID) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if strings.TrimSpace(f.uid) == "" {
		return "", errV1AvatarUIDUnavailable
	}
	return f.uid, nil
}

func (f *fakeAvatarUID) UpdateUserAvatar(ctx context.Context, userID int64, avatar string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.avatar, f.avatarUserID = avatar, userID
	f.writes++
	return nil
}

// UpdateDisplayNameByID keeps fakeAvatarUID satisfying the extended
// runtimehttp.V1UserProfileDirectory. The avatar surface never
// renames, so the write is only recorded, never projected.
func (f *fakeAvatarUID) UpdateDisplayNameByID(_ context.Context, _ int64, _ string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes++
	return nil
}

// GetUserByID keeps fakeAvatarUID satisfying the extended
// runtimehttp.V1UserProfileDirectory (storage/status/timestamps projection).
// Unknown users project an empty row; only the avatar-owning user
// carries the persisted URL.
func (f *fakeAvatarUID) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	user := &domain.User{ID: userID, StorageUID: f.uid, Status: domain.StatusActive}
	if f.avatarUserID == userID {
		user.Avatar = f.avatar
	}
	return user, nil
}

// GetUserByStorageUID resolves the same single-uid fixture by storage
// namespace: a mismatched uid reads as not-found (the public avatar
// route converges it to 404).
func (f *fakeAvatarUID) GetUserByStorageUID(ctx context.Context, storageUID string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if strings.TrimSpace(storageUID) == "" || strings.TrimSpace(f.uid) == "" ||
		strings.TrimSpace(storageUID) != strings.TrimSpace(f.uid) {
		return nil, errFakeAvatarUserNotFound
	}
	return f.GetUserByID(ctx, f.avatarUserID)
}

type v1MediaGapsFixture struct {
	store   *tenancy.InMemoryStore
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeV1MediaObjects
	dir     *staticUserDirectory
	engine  *gin.Engine
	ws      *tenancy.Workspace
	ws2     *tenancy.Workspace
	owner   int64
	member  int64
	other   int64
}

func newV1MediaGapsFixture(t *testing.T, avatarUID V1AvatarUIDDirectory) *v1MediaGapsFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	// serveAny=false: only PUT-staged keys read back; unstaged slots
	// miss so the complete-after-bytes gate is genuinely exercised.
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-gaps", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	ws2, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-gaps-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws2.PublicID, owner, tenancy.RoleMember)
	deps := ContractV1Deps{
		Media: ContractV1MediaDeps{
			Media:         media,
			MediaObjects:  objects,
			MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}}),
		},
		Avatar: ContractV1AvatarDeps{
			AvatarUID:     avatarUID,
			AvatarObjects: objects,
		},
	}
	if profile, ok := avatarUID.(runtimehttp.V1UserProfileDirectory); ok {
		deps.UserProfile = profile
	}
	engine := newV1MediaTestEngine(store, dir, deps)
	return &v1MediaGapsFixture{store: store, media: media, objects: objects, dir: dir, engine: engine,
		ws: ws, ws2: ws2, owner: owner, member: member, other: other}
}

func (f *v1MediaGapsFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaGapsFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

// v1GapsPNGBytes pads a valid PNG to the requested size.
func v1GapsPNGBytes(size int) []byte {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		panic(err)
	}
	out := make([]byte, max(size, encoded.Len()))
	copy(out, encoded.Bytes())
	return out
}

// v1GapsGIFBytes builds deterministic sniff-valid GIF bytes.
func v1GapsGIFBytes(size int) []byte {
	if size < 6 {
		size = 6
	}
	out := make([]byte, size)
	copy(out, []byte("GIF89a"))
	return out
}

func v1GapsChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// v1GapsErrorFields extracts error.details.fields from an error
// envelope for field-level assertions.
func v1GapsErrorFields(t *testing.T, payload map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	errBody, _ := payload["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	fields, _ := details["fields"].(map[string]any)
	for k, v := range fields {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// v1GapsJSONStringArray renders a JSON string array for batch bodies.
func v1GapsJSONStringArray(ids []string) string {
	var sb strings.Builder
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%q", id)
	}
	return sb.String()
}

func v1GapsNewUploadBody(filename, checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"filename":%q,"mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"}}`,
		filename, size, checksum)
}

func v1GapsFinishUploadBody(checksum string, size int) string {
	return fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"mime_type":"image/png","media_kind":"image","width":100,"height":80}}`,
		checksum, size)
}

// v1GapsDeclare creates a session whose declaration matches body.
func v1GapsDeclare(t *testing.T, engine *gin.Engine, uploadsPath string, userID int64, key, filename string, body []byte) string {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "POST", path: uploadsPath,
		body:   v1GapsNewUploadBody(filename, v1GapsChecksum(body), len(body)),
		userID: userID, headers: map[string]string{"Idempotency-Key": key}})
	if status != http.StatusCreated {
		t.Fatalf("declare = %d %v, want 201", status, payload)
	}
	return v1MediaUploadID(t, v1Data(t, payload))
}

// v1GapsStage PUTs raw bytes and asserts the 200 session envelope.
func v1GapsStage(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "PUT", path: path, body: string(body),
		userID: userID, headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": key}})
	if status != http.StatusOK {
		t.Fatalf("stage bytes = %d %v, want 200", status, payload)
	}
}

// v1GapsComplete completes a staged session and returns the asset id.
func v1GapsComplete(t *testing.T, engine *gin.Engine, path string, body []byte, userID int64, key string) string {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{method: "POST", path: path,
		body:   v1GapsFinishUploadBody(v1GapsChecksum(body), len(body)),
		userID: userID, headers: map[string]string{"Idempotency-Key": key}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])
}

// TestV1UploadBytesFullLifecycle proves the real transfer path end to
// end: declare -> PUT bytes -> complete -> asset -> content serves the
// staged bytes -> derivatives -> archive.
func TestV1UploadBytesFullLifecycle(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(1024)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-life-create", "photo.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.member, "gaps-life-stage")
	assetID := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.member, "gaps-life-complete")
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET",
		path: f.assetsPath() + "/" + assetID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("asset metadata = %d %v, want 200", status, payload)
	}
	dto := v1Data(t, payload).(map[string]any)
	if dto["width"] != float64(2) || dto["height"] != float64(3) {
		t.Fatalf("asset dimensions = %v x %v, want staged 2 x 3 instead of declared 100 x 80", dto["width"], dto["height"])
	}

	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: f.assetsPath() + "/" + assetID + "/content", userID: f.owner}, "test-gaps-")
	if status != http.StatusOK || body != string(staged) {
		t.Fatalf("content = %d len %d, want 200 staged bytes len %d", status, len(body), len(staged))
	}
	assertV1DisplayTierImage(t, f.engine, v1TestImgproxyConfig(t),
		f.assetsPath()+"/"+assetID+"/derivatives/thumb", f.owner,
		derivativedomain.TierThumb, derivativedomain.TierSpecs[derivativedomain.TierThumb].MaxSide)

	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/" + assetID + "/archive",
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-life-archive"}})
	if status != http.StatusOK {
		t.Fatalf("archive = %d %v, want 200", status, payload)
	}
}

// TestV1CompleteWithoutStagedBytesRejected proves declare+complete
// without the byte PUT fails loudly (422, nothing materialized) and
// stays retryable: PUT then complete succeeds on the same session.
func TestV1CompleteWithoutStagedBytesRejected(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(512)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-nostage-create", "photo.png", staged)
	completeBody := v1GapsFinishUploadBody(v1GapsChecksum(staged), len(staged))
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST",
		path: f.uploadsPath() + "/" + uploadID + "/complete", body: completeBody,
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-nostage-complete"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("incomplete complete = %d %v, want 422 upload_validation_failed", status, payload)
	}
	if fields := v1GapsErrorFields(t, payload); fields["bytes"] == "" {
		t.Fatalf("incomplete complete fields = %v, want bytes=missing", fields)
	}

	// Nothing materialized: the asset list stays empty.
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	if items, _ := v1Data(t, payload).([]any); len(items) != 0 {
		t.Fatalf("list after rejected complete = %v, want empty", items)
	}

	// The session stays live (transferring) for the real PUT...
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET",
		path: f.uploadsPath() + "/" + uploadID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get session = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "transferring" {
		t.Fatalf("session status = %q, want transferring (retryable)", got)
	}

	// ...after which the same completion succeeds.
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.owner, "gaps-nostage-stage")
	assetID := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.owner, "gaps-nostage-complete-2")
	if !contract.IsAssetID(assetID) {
		t.Fatalf("recovered asset id = %q, want asset_ shape", assetID)
	}
}

// TestV1CompleteCorruptedStagedBytesFailsSession proves backend
// corruption/eviction between PUT and complete fails loudly AND
// terminally (mirroring completion-echo mismatch semantics).
func TestV1CompleteCorruptedStagedBytesFailsSession(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(256)

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-corrupt-create", "photo.png", staged)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, f.owner, "gaps-corrupt-stage")

	// Corrupt the staged object out of band (eviction/corruption analog).
	session, err := f.media.GetSession(context.Background(), f.ws.PublicID, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	f.objects.mu.Lock()
	f.objects.objects[session.StorageKey] = v1GapsGIFBytes(len(staged))
	f.objects.mu.Unlock()

	completeBody := v1GapsFinishUploadBody(v1GapsChecksum(staged), len(staged))
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST",
		path: f.uploadsPath() + "/" + uploadID + "/complete", body: completeBody,
		userID: f.owner, headers: map[string]string{"Idempotency-Key": "gaps-corrupt-complete"}})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("corrupt complete = %d %v, want 422 upload_validation_failed", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET",
		path: f.uploadsPath() + "/" + uploadID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("get session = %d %v, want 200", status, payload)
	}
	if got, _ := v1Data(t, payload).(map[string]any)["status"].(string); got != "failed" {
		t.Fatalf("session status = %q, want failed", got)
	}
}

// TestV1PutUploadBytesValidation covers the byte-PUT contract: exact
// size echo, checksum echo, sniffed-MIME echo, terminal-state 409,
// idempotent replay/conflict, and the 404-vs-400 matrix.
func TestV1PutUploadBytesValidation(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	staged := v1GapsPNGBytes(1024)
	bytesPath := func(uploadID string) string { return f.uploadsPath() + "/" + uploadID + "/bytes" }
	put := func(uploadID string, body []byte, userID int64, key, contentType string) (int, map[string]any) {
		headers := map[string]string{"Idempotency-Key": key}
		if contentType != "" {
			headers["Content-Type"] = contentType
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: bytesPath(uploadID),
			body: string(body), userID: userID, headers: headers})
		return status, payload
	}

	uploadID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-put-create", "photo.png", staged)

	// Wrong size (short body, same declaration).
	if status, payload := put(uploadID, staged[:100], f.owner, "gaps-put-short", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("short PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Same size, different content (checksum mismatch).
	mutated := append([]byte(nil), staged...)
	mutated[len(mutated)-1] ^= 0xFF
	if status, payload := put(uploadID, mutated, f.owner, "gaps-put-badsum", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("checksum-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Same size, wrong format (GIF bytes against a PNG declaration).
	if status, payload := put(uploadID, v1GapsGIFBytes(len(staged)), f.owner, "gaps-put-badmime", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("mime-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Same size, unknown format (non-image bytes): 415, not 422.
	unknown := bytes.Repeat([]byte("A"), len(staged))
	if status, payload := put(uploadID, unknown, f.owner, "gaps-put-unknown", "image/png"); status != http.StatusUnsupportedMediaType ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaUnsupportedType) {
		t.Fatalf("unknown-format PUT = %d %v, want 415 media_unsupported_type", status, payload)
	}
	// Header lies while bytes are right.
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-badheader", "image/gif"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
		t.Fatalf("header-mismatch PUT = %d %v, want 422 upload_validation_failed", status, payload)
	}
	// Empty body.
	if status, payload := put(uploadID, nil, f.owner, "gaps-put-empty", "image/png"); status != http.StatusUnprocessableEntity {
		t.Fatalf("empty PUT = %d %v, want 422", status, payload)
	}

	// Happy-path PUT, then idempotent replay + conflict.
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-ok", "image/png"); status != http.StatusOK {
		t.Fatalf("PUT = %d %v, want 200", status, payload)
	}
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-ok", "image/png"); status != http.StatusOK {
		t.Fatalf("PUT replay = %d %v, want 200", status, payload)
	}
	other := v1GapsPNGBytes(512)
	otherID := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-put-other", "other.png", other)
	// Same key against a different (valid) session: hash diverges → 409.
	otherHeaders := map[string]string{"Content-Type": "image/png", "Idempotency-Key": "gaps-put-ok"}
	status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: bytesPath(otherID),
		body: string(other), userID: f.owner, headers: otherHeaders})
	if status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadIdempotencyConflict) {
		t.Fatalf("PUT conflict = %d %v, want 409 upload_idempotency_conflict", status, payload)
	}

	// Missing key, malformed id, outsider, and terminal-state PUT.
	noKeyCall := v1Call{method: "PUT", path: bytesPath(uploadID), body: string(staged),
		userID: f.owner, headers: map[string]string{"Content-Type": "image/png"}}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless PUT = %d %v, want 200 (server-minted content-hash key)", status, payload)
	}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless PUT replay = %d %v, want 200", status, payload)
	}
	// A present-but-malformed (overlong) key still fails closed with 400.
	if status, payload := put(uploadID, staged, f.owner, strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), "image/png"); status != http.StatusBadRequest ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("overlong key PUT = %d %v, want 400 idempotency_key_required", status, payload)
	}
	if status, payload := put("nope", staged, f.owner, "gaps-put-malformed", "image/png"); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadNotFound) {
		t.Fatalf("malformed PUT = %d %v, want 404 upload_not_found", status, payload)
	}
	if status, payload := put(uploadID, staged, f.other, "gaps-put-outsider", "image/png"); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider PUT = %d %v, want 404 workspace_not_found", status, payload)
	}
	v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+uploadID+"/complete", staged, f.owner, "gaps-put-then-complete")
	if status, payload := put(uploadID, staged, f.owner, "gaps-put-terminal", "image/png"); status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeUploadStateConflict) {
		t.Fatalf("terminal PUT = %d %v, want 409 upload_state_conflict", status, payload)
	}
}

// TestV1AvatarUpload covers the user-scoped avatar PUT: sniffed MIME +
// size validation, canonical content-hashed keying through the storage
// abstraction, public-route-URL projection, idempotency, superseded-key
// cleanup, and the fail-closed seams.
func TestV1AvatarUpload(t *testing.T) {
	profile := &fakeAvatarUID{uid: "AVTR01"}
	f := newV1MediaGapsFixture(t, profile)
	avatar := v1GapsPNGBytes(2048)
	wantKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(avatar)[:16] + ".png"
	wantURL := runtimehttp.V1PublicAvatarURLForRow("AVTR01", wantKey)
	put := func(body []byte, userID int64, key, contentType string) (int, map[string]any) {
		headers := map[string]string{"Idempotency-Key": key}
		if contentType != "" {
			headers["Content-Type"] = contentType
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: "/api/v1/user/avatar",
			body: string(body), userID: userID, headers: headers})
		return status, payload
	}

	status, payload := put(avatar, f.owner, "avatar-1", "image/png")
	if status != http.StatusOK {
		t.Fatalf("avatar PUT = %d %v, want 200", status, payload)
	}
	url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string)
	if url != wantURL {
		t.Fatalf("avatar_url = %q, want the public avatar route %q", url, wantURL)
	}
	// The row keeps the canonical storage key, never the route URL.
	if profile.avatar != wantKey || profile.avatarUserID != f.owner {
		t.Fatalf("avatar projection = %q for %d, want key %q for owner", profile.avatar, profile.avatarUserID, wantKey)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["avatar"] != wantURL {
		t.Fatalf("user avatar projection = %d %v, want the public route URL", status, payload)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.other})
	if _, leaked := v1Data(t, payload).(map[string]any)["avatar"]; status != http.StatusOK || leaked {
		t.Fatalf("cross-user avatar = %d %v, want no avatar", status, payload)
	}
	// The staged object is readable through the byte seam at that key.
	object, err := f.objects.ReadObjectAtProfile(context.Background(), workspacemedia.DefaultStorageProfile,
		wantKey, v1AvatarMaxBytes)
	if err != nil || string(object.Data) != string(avatar) {
		t.Fatalf("staged avatar = %v %v, want the PUT bytes", object, err)
	}

	// Replay converges; a different body under the same key conflicts.
	if status, _ := put(avatar, f.owner, "avatar-1", "image/png"); status != http.StatusOK {
		t.Fatalf("avatar replay = %d, want 200", status)
	}
	if profile.writes != 1 {
		t.Fatalf("replay avatar writes = %d, want 1", profile.writes)
	}
	if status, payload := put(v1GapsPNGBytes(1024), f.owner, "avatar-1", "image/png"); status != http.StatusConflict ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaIdempotencyConflict) {
		t.Fatalf("avatar conflict = %d %v, want 409 media_idempotency_conflict", status, payload)
	}

	// Non-image bytes, oversize body, anonymous, missing key.
	if status, payload := put([]byte("not an image at all...."), f.owner, "avatar-bad", "image/png"); status != http.StatusUnsupportedMediaType ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaUnsupportedType) {
		t.Fatalf("bad avatar = %d %v, want 415 media_unsupported_type", status, payload)
	}
	// heic/svg stay rejected at the edge (allowlist unchanged): unknown
	// containers sniff to "" and render 415, never 422 or success.
	heic := []byte{
		0x00, 0x00, 0x00, 0x18,
		0x66, 0x74, 0x79, 0x70,
		0x68, 0x65, 0x69, 0x63,
		0x00, 0x00, 0x00, 0x00,
		0x68, 0x65, 0x69, 0x63,
		0x00, 0x00, 0x00, 0x00,
	}
	if status, payload := put(heic, f.owner, "avatar-heic", "image/heic"); status != http.StatusUnsupportedMediaType ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaUnsupportedType) {
		t.Fatalf("heic avatar = %d %v, want 415 media_unsupported_type", status, payload)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"></svg>`)
	if status, payload := put(svg, f.owner, "avatar-svg", "image/svg+xml"); status != http.StatusUnsupportedMediaType ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaUnsupportedType) {
		t.Fatalf("svg avatar = %d %v, want 415 media_unsupported_type", status, payload)
	}
	if status, payload := put(make([]byte, v1AvatarMaxBytes+1), f.owner, "avatar-big", "image/png"); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("oversize avatar = %d %v, want 422 media_validation_failed", status, payload)
	}
	anonCall := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar),
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-anon"}}
	if status, _, _ := doV1(t, f.engine, anonCall); status != http.StatusUnauthorized {
		t.Fatalf("anonymous avatar = %d, want 401", status)
	}
	noKeyCall := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar),
		userID: f.owner, headers: map[string]string{"Content-Type": "image/png"}}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless avatar = %d %v, want 200 (server-minted content-hash key)", status, payload)
	} else if url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string); url != wantURL {
		t.Fatalf("keyless avatar_url = %q, want the public avatar route", url)
	}
	if status, _, payload := doV1(t, f.engine, noKeyCall); status != http.StatusOK {
		t.Fatalf("keyless avatar replay = %d %v, want 200", status, payload)
	}
	if status, payload := put(avatar, f.owner, strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), "image/png"); status != http.StatusBadRequest ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("overlong key avatar = %d %v, want 400 idempotency_key_required", status, payload)
	}

	// A replaced avatar stores under its own content key, projects the
	// same route URL, and removes the superseded object best-effort.
	replacement := v1GapsGIFBytes(2048)
	replacementKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(replacement)[:16] + ".gif"
	if status, payload := put(replacement, f.owner, "avatar-2", "image/gif"); status != http.StatusOK {
		t.Fatalf("avatar replacement = %d %v, want 200", status, payload)
	} else if url, _ := v1Data(t, payload).(map[string]any)["avatar_url"].(string); url != runtimehttp.V1PublicAvatarURLForRow("AVTR01", replacementKey) {
		t.Fatalf("replacement avatar_url = %q, want the replacement version", url)
	}
	if profile.avatar != replacementKey {
		t.Fatalf("replacement projection = %q, want key %q", profile.avatar, replacementKey)
	}
	if profile.writes != 2 {
		t.Fatalf("replacement avatar writes = %d, want 2", profile.writes)
	}
	f.objects.mu.Lock()
	_, superseded := f.objects.objects[wantKey]
	f.objects.mu.Unlock()
	if superseded {
		t.Fatalf("superseded avatar key %q was not cleaned up", wantKey)
	}
}

// assertV1PublicAvatarImage verifies a public size variant and its internal source.
func assertV1PublicAvatarImage(t *testing.T, engine *gin.Engine, cfg imgproxy.Config, path string, userID int64, size int, key string) {
	t.Helper()
	call := v1Call{method: "GET", path: path}
	if userID > 0 {
		call.userID = userID
	}
	assertImgproxyImage(t, engine, cfg, call, "test-avatar-",
		path, "no-store",
		fmt.Sprintf("/rs:fill:%d:%d/plain/local:///%s@webp", size, size, key))
}

// TestV1PublicAvatarRoute covers the public avatar GET: sizeless and
// size variants (anonymous included), the size whitelist, unknown/malformed
// uids, and the bundled default SVG for callers without an avatar.
func TestV1PublicAvatarRoute(t *testing.T) {
	profile := &fakeAvatarUID{uid: "AVTR01"}
	f := newV1MediaGapsFixture(t, profile)
	cfg := v1TestImgproxyConfig(t)
	route := "/api/v1/public/users/AVTR01/avatar"

	// No custom avatar yet: the bundled default SVG serves inline.
	status, header, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: route}, "test-avatar-")
	if status != http.StatusOK {
		t.Fatalf("default avatar = %d, want 200", status)
	}
	if ct := header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("default avatar Content-Type = %q, want image/svg+xml", ct)
	}
	if cc := header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("default avatar Cache-Control = %q, want no-store", cc)
	}
	if !strings.Contains(body, "<svg") {
		t.Fatalf("default avatar body is not SVG: %q", body)
	}

	avatar := v1GapsPNGBytes(2048)
	wantKey := "uploads/avatars/AVTR01/" + v1GapsChecksum(avatar)[:16] + ".png"
	status, _, payload := doV1(t, f.engine, v1Call{method: "PUT", path: "/api/v1/user/avatar",
		body: string(avatar), userID: f.owner,
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-route-1"}})
	if status != http.StatusOK {
		t.Fatalf("avatar PUT = %d %v, want 200", status, payload)
	}

	// Sizeless defaults to 64; explicit sizes render their fill
	// geometry. Anonymous callers pass (avatars are public).
	assertV1PublicAvatarImage(t, f.engine, cfg, route, f.owner, 64, wantKey)
	assertV1PublicAvatarImage(t, f.engine, cfg, route, 0, 64, wantKey)
	for _, size := range []int{32, 128, 256} {
		assertV1PublicAvatarImage(t, f.engine, cfg, route+"?size="+strconv.Itoa(size), f.owner, size, wantKey)
	}

	// Outside the whitelist (or unparsable) reads as plain 400.
	for _, size := range []string{"100", "0", "-1", "abc", "64.5"} {
		status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET",
			path: route + "?size=" + size, userID: f.owner}, "test-avatar-")
		if status != http.StatusBadRequest {
			t.Fatalf("avatar size %q = %d, want plain 400", size, status)
		}
	}

	// Well-shaped but unknown uids converge to 404; malformed uids to
	// 400. Both stay envelope-free like the binary endpoints.
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: "/api/v1/public/users/ZZZZ99/avatar", userID: f.owner}, "test-avatar-")
	if status != http.StatusNotFound {
		t.Fatalf("unknown avatar uid = %d, want plain 404", status)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET",
		path: "/api/v1/public/users/bad%20uid!/avatar", userID: f.owner}, "test-avatar-")
	if status != http.StatusBadRequest {
		t.Fatalf("malformed avatar uid = %d, want plain 400", status)
	}
}

// TestV1AvatarFailClosed proves the avatar endpoint fails closed
// without its seams (501 without the UID directory, 503 without the
// byte backend).
func TestV1AvatarFailClosed(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	avatar := v1GapsPNGBytes(512)
	call := v1Call{method: "PUT", path: "/api/v1/user/avatar", body: string(avatar), userID: f.owner,
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-closed-1"}}
	if status, _, payload := doV1(t, f.engine, call); status != http.StatusNotImplemented ||
		v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("avatar without directory = %d %v, want 501", status, payload)
	}

	// UID directory present but no byte backend: 503.
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	dir.ensure(f.owner)
	ws, err := store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "avatar-closed", OwnerUserID: f.owner})
	if err != nil {
		t.Fatal(err)
	}
	_ = ws
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{
		Media:  ContractV1MediaDeps{Media: media},
		Avatar: ContractV1AvatarDeps{AvatarUID: &fakeAvatarUID{uid: "AVTR01"}},
	})
	call.headers = map[string]string{"Content-Type": "image/png", "Idempotency-Key": "avatar-closed-2"}
	if status, _, payload := doV1(t, engine, call); status != http.StatusServiceUnavailable ||
		v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("avatar without objects = %d %v, want 503 service_unavailable", status, payload)
	}
}

// TestV1BatchGetMediaAssets covers the bounded batch metadata read:
// order preservation, 404 convergence per invisible id, bounds, and
// auth.
func TestV1BatchGetMediaAssets(t *testing.T) {
	f := newV1MediaGapsFixture(t, &fakeAvatarUID{uid: "AVTR01"})
	first := v1GapsPNGBytes(256)
	second := v1GapsPNGBytes(512)

	firstUpload := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-batch-c1", "a.png", first)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+firstUpload+"/bytes", first, f.owner, "gaps-batch-s1")
	firstAsset := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+firstUpload+"/complete", first, f.owner, "gaps-batch-f1")
	secondUpload := v1GapsDeclare(t, f.engine, f.uploadsPath(), f.owner, "gaps-batch-c2", "b.png", second)
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+secondUpload+"/bytes", second, f.owner, "gaps-batch-s2")
	secondAsset := v1GapsComplete(t, f.engine, f.uploadsPath()+"/"+secondUpload+"/complete", second, f.owner, "gaps-batch-f2")

	batch := func(ids []string, userID int64) (int, map[string]any) {
		status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
			body: fmt.Sprintf(`{"data":{"asset_ids":[%s]}}`, v1GapsJSONStringArray(ids)), userID: userID})
		return status, payload
	}

	// Reversed request order comes back reversed (order preserved).
	status, payload := batch([]string{secondAsset, firstAsset}, f.owner)
	if status != http.StatusOK {
		t.Fatalf("batch = %d %v, want 200", status, payload)
	}
	items, ok := v1Data(t, payload).(map[string]any)["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("batch items = %v, want 2", v1Data(t, payload))
	}
	if items[0].(map[string]any)["id"] != secondAsset || items[1].(map[string]any)["id"] != firstAsset {
		t.Fatalf("batch order = %v, want request order", items)
	}
	validBody := fmt.Sprintf(`{"data":{"asset_ids":[%q],"unknown":true}}`, firstAsset)
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"unknown field", validBody, http.StatusOK},
		{"trailing whitespace", validBody + " \n", http.StatusOK},
		{"trailing JSON", validBody + ` {"unexpected":true}`, http.StatusBadRequest},
		{"trailing garbage", validBody + " invalid", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost,
				path: f.assetsPath() + "/batch", body: tc.body, userID: f.owner})
			if status != tc.want {
				t.Fatalf("batch = %d %v, want %d", status, payload, tc.want)
			}
		})
	}

	// Invisible ids converge to 404 without leaking which half failed.
	for name, ids := range map[string][]string{
		"unknown":   {"asset_missing00000001"},
		"malformed": {"nope"},
	} {
		if status, payload := batch(ids, f.owner); status != http.StatusNotFound ||
			v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
			t.Fatalf("batch %s = %d %v, want 404 media_not_found", name, status, payload)
		}
	}
	// Cross-workspace id: stage an asset in ws2, then batch it from ws1.
	foreignBody := v1GapsPNGBytes(128)
	foreignUpload := v1GapsDeclare(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads", f.other, "gaps-batch-c3", "c.png", foreignBody)
	v1GapsStage(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads/"+foreignUpload+"/bytes", foreignBody, f.other, "gaps-batch-s3")
	foreignAsset := v1GapsComplete(t, f.engine, "/api/v1/workspaces/"+f.ws2.PublicID+"/uploads/"+foreignUpload+"/complete", foreignBody, f.other, "gaps-batch-f3")
	if status, payload := batch([]string{foreignAsset}, f.owner); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
		t.Fatalf("batch cross-workspace = %d %v, want 404 media_not_found", status, payload)
	}

	// Bounds: empty and over-limit render 422; outsiders 404.
	if status, payload := batch(nil, f.owner); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("batch empty = %d %v, want 422 media_validation_failed", status, payload)
	}
	big := make([]string, maxV1MediaBatchAssets+1)
	for i := range big {
		big[i] = firstAsset
	}
	if status, payload := batch(big, f.owner); status != http.StatusUnprocessableEntity ||
		v1ErrorCode(t, payload) != string(contract.CodeMediaValidationFailed) {
		t.Fatalf("batch over-limit = %d %v, want 422 media_validation_failed", status, payload)
	}
	if status, payload := batch([]string{firstAsset}, f.other); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("batch outsider = %d %v, want 404 workspace_not_found", status, payload)
	}
}

// ---- merged from media_access_postgres_test.go ----
// Media asset authorization over real PostgreSQL + HTTP (issue #936,
// criterion 7).
//
// End to end on PG-backed tenancy/boards/media: same-tenant unauthorized
// members, foreign workspace ids, the bound-upload pre-save window,
// prewarmed-cache/304 behavior, batch, multiple sources, and reference
// GC. Binary error contracts stay plain-status throughout.

type v1MediaAccessPGFixture struct {
	engine *gin.Engine
	ws     *tenancy.Workspace
	ws2    *tenancy.Workspace
	owner  int64
	member int64
	boards *boards.PostgresWorkspaceBoardStore
	media  *workspacemedia.Service
}

func newV1MediaAccessPGFixture(t *testing.T) *v1MediaAccessPGFixture {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db, nil)
	dir := newStaticUserDirectory()
	const owner, member, other = 1, 10, 8
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	configureV1WorkspaceDefaultQuota(t, db, ws)
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	ws2, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	media := workspacemedia.NewService(db)
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	boardsStore := boards.NewPostgresWorkspaceBoardStore(db, workspacemedia.ValidateDocumentAssets)
	engine := newV1MediaTestEngine(tstore, dir, ContractV1Deps{
		Media: ContractV1MediaDeps{
			Media:         media,
			MediaObjects:  objects,
			MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), nil),
		},
		Boards: boardsStore,
	})
	return &v1MediaAccessPGFixture{engine: engine, ws: ws, ws2: ws2, owner: owner, member: member, boards: boardsStore, media: media}
}

func (f *v1MediaAccessPGFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaAccessPGFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

// pgAccessUpload declares (optionally board-bound), stages real bytes, and
// completes one asset, returning its id. Board binding requires the
// creator's write access and is verified by the handler, not the client.
func pgAccessUpload(t *testing.T, f *v1MediaAccessPGFixture, creator int64, boardID string, key string) string {
	t.Helper()
	staged := v1GapsPNGBytes(512)
	checksum := v1GapsChecksum(staged)
	body := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"%s}}`,
		len(staged), checksum, func() string {
			if boardID == "" {
				return ""
			}
			return fmt.Sprintf(`,"board_id":%q`, boardID)
		}())
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(),
		body: body, userID: creator, headers: map[string]string{"Idempotency-Key": key + "-create"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, creator, key+"-stage")
	finish := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"width":100,"height":80}}`, checksum, len(staged))
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete",
		body: finish, userID: creator, headers: map[string]string{"Idempotency-Key": key + "-complete"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])
}

func pgAccessBoard(t *testing.T, f *v1MediaAccessPGFixture, name string) string {
	t.Helper()
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.CreateBoard(context.Background(), f.ws.PublicID, f.owner, name, boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	return boardID
}

func pgAccessDoc(t *testing.T, f *v1MediaAccessPGFixture, boardID string, revision int64, assetIDs ...string) {
	t.Helper()
	nodes := make([]string, 0, len(assetIDs))
	for i, id := range assetIDs {
		nodes = append(nodes, fmt.Sprintf(`{"id":%d,"x":0,"y":0,"width":280,"sourceImageUrl":"https://x/y.png","sourceMediaRef":{"assetId":%q},"outputImages":[]}`, i+1, id))
	}
	envelope := fmt.Sprintf(`{"schema_version":1,"payload":{"schemaVersion":4,"imageNodes":[%s],"promptNodes":[],"fusionNodes":[]}}`, strings.Join(nodes, ","))
	if _, err := f.boards.ReplaceDocument(context.Background(), f.ws.PublicID, boardID, f.owner, revision, 1, []byte(envelope)); err != nil {
		t.Fatalf("replace document: %v", err)
	}
}

// Same-tenant unauthorized members read 404 on every surface; lists hide
// the asset; foreign workspace ids converge to 404 too.
func TestV1MediaAccessPGDeniesUnauthorizedMember(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	assetID := pgAccessUpload(t, f, f.owner, "", "deny")

	for name, call := range map[string]v1Call{
		"metadata": {method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.member},
		"batch": {method: "POST", path: f.assetsPath() + "/batch",
			body:   fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID),
			userID: f.member},
	} {
		status, _, payload := doV1(t, f.engine, call)
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
			t.Fatalf("%s = %d %v, want 404 media_not_found", name, status, payload)
		}
	}
	for _, path := range []string{
		f.assetsPath() + "/" + assetID + "/content",
		f.assetsPath() + "/" + assetID + "/derivatives/thumb",
	} {
		status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: path, userID: f.member}, "pg-acc-")
		if status != http.StatusNotFound {
			t.Fatalf("binary %s = %d, want plain 404", path, status)
		}
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	for _, item := range v1Data(t, payload).([]any) {
		if item.(map[string]any)["id"] == assetID {
			t.Fatalf("list leaked unauthorized asset %s", assetID)
		}
	}
	foreign := "/api/v1/workspaces/" + f.ws2.PublicID + "/media-assets/" + assetID + "/content"
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: foreign, userID: f.owner}, "pg-deny-")
	if status != http.StatusNotFound {
		t.Fatalf("foreign workspace content = %d, want plain 404", status)
	}
}

// The bound-upload pre-save window: a board reader reads the bytes before
// any document accepts the asset; unbound uploads stay owner-only even
// though the board is not autosaved/accepted yet.
func TestV1MediaAccessPGTargetWindow(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "target")
	assetID := pgAccessUpload(t, f, f.owner, boardID, "window-bound")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "pg-window-")
	if status != http.StatusOK {
		t.Fatalf("target-reader content = %d, want 200 before any document accept", status)
	}
	plain := pgAccessUpload(t, f, f.owner, "", "window-plain")
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + plain + "/content", userID: f.member}, "pg-window-")
	if status != http.StatusNotFound {
		t.Fatalf("unbound content = %d, want 404 (no workspace-readable window)", status)
	}
	// Served DTOs carry same-origin API URLs only: the internal imgproxy
	// signed URL never appears on the wire as a public bypass.
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("owner metadata = %d %v, want 200", status, payload)
	}
	dto := v1Data(t, payload).(map[string]any)
	contentURL, _ := dto["content_url"].(string)
	if !strings.HasPrefix(contentURL, "/api/v1/workspaces/") || strings.Contains(contentURL, "?") {
		t.Fatalf("content_url = %q, want same-origin API path without signature", contentURL)
	}
	derivatives, _ := dto["derivatives"].(map[string]any)
	for tier, raw := range derivatives {
		entry, _ := raw.(map[string]any)
		u, _ := entry["url"].(string)
		if entry["status"] == "ready" && (!strings.HasPrefix(u, "/api/v1/workspaces/") || strings.Contains(u, "?")) {
			t.Fatalf("derivative %s url = %q, want same-origin API path", tier, u)
		}
	}
	// Forged bindings fail closed at create (404, never a grant).
	staged := v1GapsPNGBytes(512)
	body := fmt.Sprintf(`{"data":{"filename":"evil.png","mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image","board_id":%q}}`,
		len(staged), v1GapsChecksum(staged), "board_forged000000001")
	status, _, _ = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: body,
		userID: f.member, headers: map[string]string{"Idempotency-Key": "window-forge"}})
	if status != http.StatusNotFound {
		t.Fatalf("forged binding create = %d, want 404", status)
	}
}

// Authorization precedes cache behavior: an unauthorized caller with a
// stolen ETag reads 404 (never 304), and prewarming as the owner never
// makes the derivative serve the member.
func TestV1MediaAccessPGAuthBeforeCache(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "cache")
	assetID := pgAccessUpload(t, f, f.owner, boardID, "cache")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	// Prewarm as the authorized reader: content (captures ETag) + derivative.
	status, headers, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "pg-cache-")
	if status != http.StatusOK {
		t.Fatalf("prewarm content = %d, want 200", status)
	}
	etag := headers.Get("ETag")
	if etag == "" {
		t.Fatal("prewarm content carried no ETag")
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.member}, "pg-cache-")
	if status != http.StatusOK {
		t.Fatalf("prewarm derivative = %d, want 200", status)
	}
	// Lose the grant: the same URLs now 404 — no cross-identity cache reuse.
	if err := func() error {
		_, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{
		method: "GET", path: f.assetsPath() + "/" + assetID + "/content",
		userID: f.member, headers: map[string]string{"If-None-Match": etag},
	}, "pg-cache-")
	if status != http.StatusNotFound {
		t.Fatalf("stolen-ETag content = %d, want 404 (auth before 304)", status)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.member}, "pg-cache-")
	if status != http.StatusNotFound {
		t.Fatalf("prewarmed derivative = %d, want 404 (no shared-cache reuse)", status)
	}
}

// Document lifecycle over HTTP: accept grants, removal revokes (GC), the
// owner source survives, and batch follows the same rule per id.
func TestV1MediaAccessPGDocumentLifecycle(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "doc")
	assetID := pgAccessUpload(t, f, f.owner, "", "doclife")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	metadata := func(user int64) int {
		status, _, _ := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: user})
		return status
	}
	if got := metadata(f.member); got != http.StatusNotFound {
		t.Fatalf("pre-accept metadata = %d, want 404", got)
	}
	pgAccessDoc(t, f, boardID, 1, assetID)
	if got := metadata(f.member); got != http.StatusOK {
		t.Fatalf("post-accept metadata = %d, want 200", got)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
		body: fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("batch post-accept = %d %v, want 200", status, payload)
	}
	// GC: the accept without the asset revokes; owner still reads.
	pgAccessDoc(t, f, boardID, 2)
	if got := metadata(f.member); got != http.StatusNotFound {
		t.Fatalf("post-removal metadata = %d, want 404", got)
	}
	if got := metadata(f.owner); got != http.StatusOK {
		t.Fatalf("owner metadata = %d, want 200 (independent source)", got)
	}
	status, _, _ = doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
		body: fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("batch post-removal = %d, want 404", status)
	}
}

// ---- merged from public_media_storage_test.go ----
func TestPublicMediaUsesActiveStorageProfile(t *testing.T) {
	const bucket = "public-images"
	for _, tc := range []struct {
		name       string
		profiler   imgproxy.ProfileResolver
		wantStatus int
		wantSource string
	}{
		{"local", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}}, http.StatusOK, "local:///"},
		{"s3 canonical key", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderS3Compatible, Enabled: true, Config: map[string]interface{}{"bucket_name": bucket}}}, http.StatusOK, "s3://" + bucket + "/"},
		{"no resolver", nil, http.StatusConflict, ""},
		{"resolver error", fakeV1StorageProfiler{err: context.DeadlineExceeded}, http.StatusConflict, ""},
		{"disabled", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal}}, http.StatusConflict, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const avatarKey = "uploads/avatars/AVTR01/abcdef1234567890.png"
			const logoKey = "uploads/site/assets/logo_abcdef1234567890.png"
			engine := newV1MediaTestEngine(tenancy.NewInMemoryStoreForTests(), newStaticUserDirectory(), ContractV1Deps{
				Media:       ContractV1MediaDeps{MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), tc.profiler)},
				UserProfile: &fakeAvatarUID{uid: "AVTR01", avatar: avatarKey},
			})
			for _, asset := range []struct{ route, key string }{
				{"/api/v1/public/users/AVTR01/avatar", avatarKey},
				{"/api/v1/public/branding/logo" + "?key=" + url.QueryEscape(logoKey), logoKey},
			} {
				status, headers, body := doV1RoundTrip(t, engine, v1Call{method: "GET", path: asset.route}, "public-media-")
				if status != tc.wantStatus {
					t.Fatalf("%s status = %d, want %d", asset.route, status, tc.wantStatus)
				}
				if status == http.StatusOK {
					if !strings.Contains(body, "/plain/"+tc.wantSource+asset.key+"@webp") {
						t.Fatalf("wrong source: %s", body)
					}
					if headers.Get("Cache-Control") != "no-store" {
						t.Fatal("mutable image must not be cached")
					}
				}
			}
		})
	}
}

// ---- merged from rendered_image_test.go ----
func TestRenderedImageProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"ready", nil, http.StatusOK},
		{"source", imgproxy.ErrSource, http.StatusConflict},
		{"missing", imgproxy.ErrNotFound, http.StatusNotFound},
		{"unavailable", imgproxy.ErrUnavailable, http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable},
		{"internal cancellation", context.Canceled, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/image", func(c *gin.Context) {
				runtimehttp.WriteRenderedImage(c, imgproxy.Image{Data: []byte("rendered image"), MIMEType: "image/webp"}, tc.err, "private, max-age=300")
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/image", nil))
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.err != nil {
				if response.Body.String() != http.StatusText(tc.want) || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("error response = %q %v", response.Body.String(), response.Header())
				}
				return
			}
			if response.Body.String() != "rendered image" || response.Header().Get("Cache-Control") != "private, max-age=300" || response.Header().Get("Content-Type") != "image/webp" || response.Header().Get("Content-Disposition") != "inline" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("image response = %q %v", response.Body.String(), response.Header())
			}
		})
	}
}

func TestRenderedImageRequestCancellation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		body string
	}{
		{"client canceled", context.Canceled, 499, ""},
		{"upstream failed", imgproxy.ErrUnavailable, http.StatusServiceUnavailable, "Service Unavailable"},
		{"deadline exceeded", context.DeadlineExceeded, http.StatusServiceUnavailable, "Service Unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/image", func(c *gin.Context) {
				runtimehttp.WriteRenderedImage(c, imgproxy.Image{}, tc.err, "private, max-age=300")
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			request := httptest.NewRequest(http.MethodGet, "/image", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != tc.want || response.Body.String() != tc.body {
				t.Fatalf("canceled response = %d %q, want %d %q", response.Code, response.Body.String(), tc.want, tc.body)
			}
		})
	}
}

func TestV1MediaUploadDirect(t *testing.T) {
	f := newV1MediaFixture(t, true)
	pngBytes := v1GapsPNGBytes(1024)

	buildMultipart := func(fileBytes []byte, filename, boardID string) (string, string) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		if fileBytes != nil {
			part, err := writer.CreateFormFile("file", filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(fileBytes); err != nil {
				t.Fatal(err)
			}
		}
		if boardID != "" {
			if err := writer.WriteField("board_id", boardID); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.String(), writer.FormDataContentType()
	}

	directPath := "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets/direct"

	t.Run("successful direct upload creates ready media asset in single request", func(t *testing.T) {
		body, contentType := buildMultipart(pngBytes, "sample.png", "")
		status, _, payload := doV1(t, f.engine, v1Call{
			method:  "POST",
			path:    directPath,
			body:    body,
			userID:  f.owner,
			headers: map[string]string{"Content-Type": contentType},
		})
		if status != http.StatusCreated {
			t.Fatalf("direct upload status = %d %v, want 201", status, payload)
		}
		data, ok := v1Data(t, payload).(map[string]any)
		if !ok {
			t.Fatalf("payload data = %T, want object", v1Data(t, payload))
		}
		assetID, _ := data["id"].(string)
		if !contract.IsAssetID(assetID) {
			t.Fatalf("asset id = %q, want asset_ prefix", assetID)
		}
		if got, _ := data["status"].(string); got != "ready" {
			t.Fatalf("status = %q, want ready", got)
		}
		if got, _ := data["workspace_id"].(string); got != f.ws.PublicID {
			t.Fatalf("workspace_id = %q, want %q", got, f.ws.PublicID)
		}
		if got, _ := data["mime_type"].(string); got != "image/png" {
			t.Fatalf("mime_type = %q, want image/png", got)
		}

		// Read back metadata via GET /workspaces/{ws}/media-assets/{id}
		getStatus, _, getPayload := doV1(t, f.engine, v1Call{
			method: "GET",
			path:   "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets/" + assetID,
			userID: f.owner,
		})
		if getStatus != http.StatusOK {
			t.Fatalf("get metadata = %d %v, want 200", getStatus, getPayload)
		}

		// Read back content bytes via GET /workspaces/{ws}/media-assets/{id}/content
		contentStatus, header, contentBytes := doV1RoundTrip(t, f.engine, v1Call{
			method: "GET",
			path:   "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets/" + assetID + "/content",
			userID: f.owner,
		}, "v1-content-")
		if contentStatus != http.StatusOK {
			t.Fatalf("get content = %d, want 200", contentStatus)
		}
		if header.Get("Content-Type") != "image/png" {
			t.Fatalf("content Content-Type = %q, want image/png", header.Get("Content-Type"))
		}
		if !bytes.Equal([]byte(contentBytes), pngBytes) {
			t.Fatalf("content bytes mismatch")
		}
	})

	t.Run("production middleware accepts files above JSON limit and enforces both upload limits", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			size int
			want int
		}{
			{"3 MiB image", 3 * 1024 * 1024, http.StatusCreated},
			{"file above 30 MiB", int(workspacemedia.MaxUploadSizeBytes) + 1, http.StatusUnprocessableEntity},
			{"multipart above bounded overhead", int(workspacemedia.MaxUploadSizeBytes) + 2*1024*1024, http.StatusRequestEntityTooLarge},
		} {
			t.Run(tc.name, func(t *testing.T) {
				data := append(append([]byte(nil), pngBytes...), make([]byte, tc.size-len(pngBytes))...)
				body, contentType := buildMultipart(data, "large.png", "")
				status, _, payload := doV1(t, f.engine, v1Call{
					method: http.MethodPost, path: directPath, body: body, userID: f.owner,
					headers: map[string]string{"Content-Type": contentType},
				})
				if status != tc.want {
					t.Fatalf("upload = %d %v, want %d", status, payload, tc.want)
				}
			})
		}
		status, _, _ := doV1(t, f.engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces/" + f.ws.PublicID + "/uploads",
			body: strings.Repeat("x", 3*1024*1024), userID: f.owner,
		})
		if status != http.StatusRequestEntityTooLarge {
			t.Fatalf("ordinary JSON POST = %d, want 413", status)
		}
	})

	t.Run("non-existent board_id returns 404", func(t *testing.T) {
		body, contentType := buildMultipart(pngBytes, "sample.png", "board_nonexistent01")
		status, _, payload := doV1(t, f.engine, v1Call{
			method:  "POST",
			path:    directPath,
			body:    body,
			userID:  f.owner,
			headers: map[string]string{"Content-Type": contentType},
		})
		if status != http.StatusNotFound {
			t.Fatalf("invalid board status = %d %v, want 404", status, payload)
		}
	})

	t.Run("missing file part returns 400 bad request", func(t *testing.T) {
		body, contentType := buildMultipart(nil, "", "")
		status, _, payload := doV1(t, f.engine, v1Call{
			method:  "POST",
			path:    directPath,
			body:    body,
			userID:  f.owner,
			headers: map[string]string{"Content-Type": contentType},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("missing file status = %d %v, want 400", status, payload)
		}
	})

	t.Run("non-image file returns 422 validation error", func(t *testing.T) {
		body, contentType := buildMultipart([]byte("this is plain text not an image"), "test.txt", "")
		status, _, payload := doV1(t, f.engine, v1Call{
			method:  "POST",
			path:    directPath,
			body:    body,
			userID:  f.owner,
			headers: map[string]string{"Content-Type": contentType},
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("non-image status = %d %v, want 422", status, payload)
		}
	})

	t.Run("non-member access returns 404", func(t *testing.T) {
		body, contentType := buildMultipart(pngBytes, "sample.png", "")
		status, _, _ := doV1(t, f.engine, v1Call{
			method:  "POST",
			path:    directPath,
			body:    body,
			userID:  f.other,
			headers: map[string]string{"Content-Type": contentType},
		})
		if status != http.StatusNotFound {
			t.Fatalf("non-member status = %d, want 404", status)
		}
	})
}
