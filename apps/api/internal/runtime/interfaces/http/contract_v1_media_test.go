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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/identity/principal"
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

// fakeV1MediaRenderer returns canned webp bytes with requested dims.
type fakeV1MediaRenderer struct {
	mu    sync.Mutex
	calls []int
}

func (f *fakeV1MediaRenderer) Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, maxSide)
	return []byte("fake-webp-bytes"), maxSide, maxSide / 2, "image/webp", nil
}

type v1MediaFixture struct {
	store    *tenancy.InMemoryStore
	media    *workspacemedia.InMemoryWorkspaceMediaStore
	objects  *fakeV1MediaObjects
	renderer *fakeV1MediaRenderer
	dir      *staticUserDirectory
	engine   *gin.Engine
	ws       *tenancy.Workspace
	ws2      *tenancy.Workspace
	owner    int64
	member   int64
	other    int64
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

func newV1MediaFixture(t *testing.T, withRenderer bool) *v1MediaFixture {
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
	deps := ContractV1Deps{Media: media, MediaObjects: objects}
	if withRenderer {
		deps.MediaRenderer = &fakeV1MediaRenderer{}
	}
	engine := newV1MediaTestEngine(store, dir, deps)
	f := &v1MediaFixture{store: store, media: media, objects: objects, dir: dir, engine: engine, ws: ws, ws2: ws2, owner: owner, member: member, other: other}
	if deps.MediaRenderer != nil {
		f.renderer, _ = deps.MediaRenderer.(*fakeV1MediaRenderer)
	}
	return f
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

func v1FinishUploadBody() string {
	return fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024,"width":100,"height":80}}`, v1MediaTestChecksum)
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
	f := newV1MediaFixture(t, true)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-life-1"}})
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

	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: v1FinishUploadBody(), userID: f.member, headers: map[string]string{"Idempotency-Key": "media-life-2"}})
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
	if got, _ := assetDTO["checksum_sha256"].(string); got != v1MediaTestChecksum {
		t.Fatalf("asset checksum = %q", got)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("get asset = %d %v, want 200", status, payload)
	}

	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	items, ok := v1Data(t, payload).([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list items = %v, want 1 asset", v1Data(t, payload))
	}

	// Bytes never ride the JSON envelope: plain 200 with bytes.
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "test-media-")
	if status != http.StatusOK || body != "fake-png-bytes" {
		t.Fatalf("content = %d %q, want 200 fake-png-bytes", status, body)
	}
	// Generation ran at complete (fake renderer): thumb serves.
	status, _, body = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.member}, "test-media-")
	if status != http.StatusOK || body != "fake-webp-bytes" {
		t.Fatalf("derivative = %d %q, want 200 fake-webp-bytes", status, body)
	}
	if f.renderer == nil || len(f.renderer.calls) != 3 {
		t.Fatalf("renderer calls = %v, want 3 sequential tiers", f.renderer)
	}

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
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("get archived = %d %v, want 200", status, payload)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "test-media-")
	if status != http.StatusGone {
		t.Fatalf("archived content = %d, want 410", status)
	}
	status, _, payload = doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("list after archive = %d %v, want 200", status, payload)
	}
	if items, _ := v1Data(t, payload).([]any); len(items) != 0 {
		t.Fatalf("list after archive = %v, want empty", items)
	}
}

func TestV1MediaCompleteMismatch(t *testing.T) {
	f := newV1MediaFixture(t, true)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-mismatch-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))

	badBody := strings.Replace(v1FinishUploadBody(), "1024", "2048", 1)
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
	f := newV1MediaFixture(t, true)

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
	f := newV1MediaFixture(t, true)

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
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-auth-3"}})
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

	// Create + complete an asset, then probe binary gates.
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: v1FinishUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-auth-4"}})
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
}

func TestV1MediaDerivativePendingWithoutRenderer(t *testing.T) {
	// Without a renderer generation is skipped: metadata tiers stay
	// pending-shaped and the binary endpoint stays 409.
	f := newV1MediaFixture(t, false)

	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: v1NewUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-pending-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete", body: v1FinishUploadBody(), userID: f.owner, headers: map[string]string{"Idempotency-Key": "media-pending-2"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	assetID := v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])

	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.owner}, "test-media-")
	if status != http.StatusConflict {
		t.Fatalf("pending derivative = %d, want plain 409", status)
	}
	// The original still serves: only derivatives are pending.
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.owner}, "test-media-")
	if status != http.StatusOK || body != "fake-png-bytes" {
		t.Fatalf("content = %d %q, want 200 fake-png-bytes", status, body)
	}
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
	f := newV1MediaFixture(t, true)

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
