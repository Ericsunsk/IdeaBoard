package httpapi

// Workspace upload + media contract tests (issue #803, restored in
// #820 as Phase 4-03 of #809).
//
// Reuses the v1 envelope helpers from contract_v1_test.go (doV1
// asserts the Request-Id echo and JSON envelope shapes; doV1RoundTrip
// covers the non-JSON binary endpoints). Covers the 8-endpoint
// lifecycle (create -> get -> complete -> metadata -> bytes ->
// derivatives -> archive), completion-mismatch failure, idempotency
// replay/conflict, the 404-vs-403 matrix, binary plain-status
// behavior, and fail-closed behavior without a store.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

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

func newV1MediaTestEngine(store tenancy.Store, dir *staticUserDirectory, deps ContractV1Deps) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if err := principal.Set(c, identityPrincipal); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	deps.Store = store
	deps.Users = dir
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, deps)
	return router
}

func newV1MediaFixture(t *testing.T) *v1MediaFixture {
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
	deps := ContractV1Deps{Media: media, MediaObjects: objects, MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), nil)}
	engine := newV1MediaTestEngine(store, dir, deps)
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
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{Media: media, MediaObjects: objects})
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
			Media:         &r2KeyMediaStore{InMemoryWorkspaceMediaStore: media, r2Key: key},
			MediaObjects:  objects,
			MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), profiler),
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
