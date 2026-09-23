package httpapi

// Workspace upload sessions + media assets (issue #803).
//
// Covers the full lifecycle (create -> transferring -> complete ->
// asset), declaration validation, completion idempotency, the
// 404-vs-403 matrix, derivative auth, archived-asset behavior, and the
// binary/metadata protocol split (binary endpoints never carry the
// JSON envelope).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspaceuploads"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
)

const v1UploadTestChecksum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// fakeV1MediaObjects serves stored bytes for the binary endpoints.
type fakeV1MediaObjects struct {
	objects map[string]*ports.StoredUploadObject
}

func (f *fakeV1MediaObjects) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	if object, ok := f.objects[profileID+"\x00"+objectKey]; ok {
		return object, nil
	}
	return nil, fmt.Errorf("object missing: %w", storagedomain.ErrStoredObjectNotFound)
}

// readyV1AssetStore flips seeded assets to ready so the binary paths can
// be exercised without a byte backend (#805 owns verification).
type readyV1AssetStore struct {
	*workspaceuploads.InMemoryAssetStore
	ready       map[string]bool
	derivatives map[string][]workspaceuploads.Derivative
}

func (s *readyV1AssetStore) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspaceuploads.Asset, error) {
	asset, err := s.InMemoryAssetStore.GetAsset(ctx, workspaceID, assetID)
	if err != nil {
		return nil, err
	}
	if s.ready[assetID] {
		asset.Status = "ready"
	}
	return asset, nil
}

func (s *readyV1AssetStore) ListDerivatives(ctx context.Context, ownerUserID, assetInternalID int64) ([]workspaceuploads.Derivative, error) {
	return s.derivatives[strconv.FormatInt(assetInternalID, 10)], nil
}

func newV1UploadTestEngine(store tenancy.Store, dir *staticUserDirectory, sessions workspaceuploads.SessionStore, assets workspaceuploads.AssetStore, objects V1MediaObjectReader) *gin.Engine {
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
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir,
		UploadSessions: sessions, WorkspaceAssets: assets, MediaObjects: objects,
	})
	return router
}

// doV1Raw exercises binary endpoints: it returns the raw body and
// asserts the Request-Id echo without requiring a JSON envelope.
func doV1Raw(t *testing.T, engine *gin.Engine, call v1Call) (int, http.Header, string) {
	t.Helper()
	return doV1RoundTrip(t, engine, call, "test-raw-")
}

type v1UploadFixture struct {
	engine   *gin.Engine
	store    *tenancy.InMemoryStore
	ws       *tenancy.Workspace
	sessions *workspaceuploads.InMemorySessionStore
	assets   *workspaceuploads.InMemoryAssetStore
	objects  *fakeV1MediaObjects
}

func newV1UploadFixture(t *testing.T) *v1UploadFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	sessions := workspaceuploads.NewInMemorySessionStore()
	assets := workspaceuploads.NewInMemoryAssetStore()
	objects := &fakeV1MediaObjects{objects: map[string]*ports.StoredUploadObject{}}
	engine := newV1UploadTestEngine(store, dir, sessions, assets, objects)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "uploads", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 2, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	return &v1UploadFixture{engine: engine, store: store, ws: ws, sessions: sessions, assets: assets, objects: objects}
}

func v1CreateUpload(t *testing.T, engine *gin.Engine, wsPublicID string, userID int64, key, body string) (int, map[string]any) {
	t.Helper()
	call := v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + wsPublicID + "/uploads",
		userID: userID, body: body,
		headers: map[string]string{contract.IdempotencyKeyHeader: key},
	}
	status, _, payload := doV1(t, engine, call)
	return status, payload
}

func v1UploadCreateBody() string {
	return fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":%q,"media_kind":"image"}}`, v1UploadTestChecksum)
}

func TestV1UploadFullLifecycle(t *testing.T) {
	fx := newV1UploadFixture(t)
	const owner, member, outsider = 1, 3, 9
	uploadsPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/uploads"

	// Create: 201 with an opaque session, pending state, and an opaque
	// transfer target (no bucket/secret/path material).
	status, created := v1CreateUpload(t, fx.engine, fx.ws.PublicID, member, "upload-key-1", v1UploadCreateBody())
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, created)
	}
	session := v1Data(t, created).(map[string]any)
	uploadID, _ := session["id"].(string)
	if !strings.HasPrefix(uploadID, "upload_") {
		t.Fatalf("upload id = %q, want upload_...", uploadID)
	}
	if session["status"] != workspaceuploads.StatusPending {
		t.Fatalf("status = %v, want pending", session["status"])
	}
	target, _ := session["upload_target"].(map[string]any)
	slot, _ := target["slot"].(string)
	if slot != uploadID || target["kind"] != "same-origin-slot" {
		t.Fatalf("upload_target = %v, want opaque same-origin slot", target)
	}
	serialized, _ := json.Marshal(created)
	for _, leaked := range []string{"bucket", "secret", "storage_key", "storageKey", "builtin-local", "ws-uploads"} {
		if strings.Contains(string(serialized), leaked) {
			t.Fatalf("create response leaks %q: %s", leaked, serialized)
		}
	}

	// First read signals transfer start.
	sessionPath := uploadsPath + "/" + uploadID
	status, _, read := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: sessionPath, userID: member})
	if status != 200 || read["data"].(map[string]any)["status"] != workspaceuploads.StatusTransferring {
		t.Fatalf("get = %d %v, want transferring", status, read)
	}

	// Complete: only now does the durable asset row exist.
	completeBody := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024,"mime_type":"image/png","media_kind":"image","width":4,"height":3}}`, v1UploadTestChecksum)
	status, _, completed := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: sessionPath + "/complete", userID: member,
		body: completeBody, headers: map[string]string{contract.IdempotencyKeyHeader: "complete-key-1"},
	})
	if status != 200 {
		t.Fatalf("complete = %d %v, want 200", status, completed)
	}
	completeData := v1Data(t, completed).(map[string]any)
	asset, _ := completeData["asset"].(map[string]any)
	assetID, _ := asset["id"].(string)
	if !strings.HasPrefix(assetID, "asset_") {
		t.Fatalf("asset id = %q, want asset_...", assetID)
	}
	if completeData["upload"].(map[string]any)["status"] != workspaceuploads.StatusCompleted {
		t.Fatalf("upload status = %v, want completed", completeData["upload"])
	}
	contentURL, _ := asset["content_url"].(string)
	if !strings.HasPrefix(contentURL, "/api/v1/workspaces/"+fx.ws.PublicID+"/media-assets/"+assetID+"/content") {
		t.Fatalf("content_url = %q, want canonical v1 asset URL", contentURL)
	}
	serialized, _ = json.Marshal(completed)
	for _, leaked := range []string{"storage_key", "storageKey", "builtin-local", "ws-uploads"} {
		if strings.Contains(string(serialized), leaked) {
			t.Fatalf("complete response leaks %q: %s", leaked, serialized)
		}
	}

	// Asset metadata: pending until byte verification lands (#805), so
	// content stays 409 with a plain (non-JSON) body.
	assetPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/media-assets/" + assetID
	status, _, got := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: assetPath, userID: member})
	if status != 200 {
		t.Fatalf("get asset = %d %v, want 200", status, got)
	}
	status, header, rawBody := doV1Raw(t, fx.engine, v1Call{method: http.MethodGet, path: assetPath + "/content", userID: member})
	if status != http.StatusConflict {
		t.Fatalf("content pending = %d %q, want 409", status, rawBody)
	}
	if contentType := header.Get("Content-Type"); strings.Contains(contentType, "json") {
		t.Fatalf("binary endpoint must never serve JSON: %q", contentType)
	}
	if json.Valid([]byte(rawBody)) {
		t.Fatalf("binary error must never be a JSON envelope: %q", rawBody)
	}

	// Double complete with the same key but a different body conflicts.
	status, _, conflict := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: sessionPath + "/complete", userID: member,
		body:    fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":2048}}`, v1UploadTestChecksum),
		headers: map[string]string{contract.IdempotencyKeyHeader: "complete-key-1"},
	})
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeUploadIdempotencyConflict) {
		t.Fatalf("conflicting complete = %d %v, want 409 upload_idempotency_conflict", status, conflict)
	}

	// Cross-workspace and outsider reads are invisible (404), never 403.
	other, err := fx.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "other", OwnerUserID: 8})
	if err != nil {
		t.Fatal(err)
	}
	status, _, cross := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + other.PublicID + "/uploads/" + uploadID, userID: 8})
	if status != 404 || v1ErrorCode(t, cross) != string(contract.CodeUploadNotFound) {
		t.Fatalf("cross-workspace session = %d %v, want 404 upload_not_found", status, cross)
	}
	status, _, hidden := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: sessionPath, userID: outsider})
	if status != 404 || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider session = %d %v, want 404 workspace_not_found", status, hidden)
	}
	_ = owner
}

func TestV1UploadValidation(t *testing.T) {
	fx := newV1UploadFixture(t)

	cases := []struct {
		name  string
		body  string
		field string
	}{
		{"bad mime", `{"data":{"filename":"a.pdf","mime_type":"application/pdf","size_bytes":10,"checksum_sha256":"` + v1UploadTestChecksum + `","media_kind":"image"}}`, "mime_type"},
		{"oversize", fmt.Sprintf(`{"data":{"filename":"a.png","mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"}}`, workspaceuploads.MaxUploadSizeBytes+1, v1UploadTestChecksum), "size_bytes"},
		{"bad checksum", `{"data":{"filename":"a.png","mime_type":"image/png","size_bytes":10,"checksum_sha256":"abc","media_kind":"image"}}`, "checksum_sha256"},
		{"bad kind", fmt.Sprintf(`{"data":{"filename":"a.png","mime_type":"image/png","size_bytes":10,"checksum_sha256":%q,"media_kind":"video"}}`, v1UploadTestChecksum), "media_kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, payload := v1CreateUpload(t, fx.engine, fx.ws.PublicID, 1, "key-"+tc.name, tc.body)
			if status != 422 || v1ErrorCode(t, payload) != string(contract.CodeUploadValidationFailed) {
				t.Fatalf("create = %d %v, want 422 upload_validation_failed", status, payload)
			}
			errBody := payload["error"].(map[string]any)
			details, _ := errBody["details"].(map[string]any)
			fields, _ := details["fields"].(map[string]any)
			if _, ok := fields[tc.field]; !ok {
				t.Fatalf("details.fields = %v, want %q", fields, tc.field)
			}
			if _, ok := errBody["request_id"]; !ok {
				t.Fatalf("error envelope requires request_id: %v", payload)
			}
		})

		// Checksum mismatch at complete fails the session (terminal).
		t.Run(tc.name+" complete mismatch", func(t *testing.T) {
			status, payload := v1CreateUpload(t, fx.engine, fx.ws.PublicID, 1, "mismatch-"+tc.name, v1UploadCreateBody())
			if status != 201 {
				t.Fatalf("create = %d %v", status, payload)
			}
			uploadID := payload["data"].(map[string]any)["id"].(string)
			otherChecksum := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			status, _, done := doV1(t, fx.engine, v1Call{
				method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/uploads/" + uploadID + "/complete",
				userID: 1, body: fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024}}`, otherChecksum),
				headers: map[string]string{contract.IdempotencyKeyHeader: "mismatch-complete-" + tc.name},
			})
			if status != 422 || v1ErrorCode(t, done) != string(contract.CodeUploadValidationFailed) {
				t.Fatalf("mismatched complete = %d %v, want 422", status, done)
			}
			status, _, retry := doV1(t, fx.engine, v1Call{
				method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/uploads/" + uploadID + "/complete",
				userID: 1, body: fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024}}`, v1UploadTestChecksum),
				headers: map[string]string{contract.IdempotencyKeyHeader: "mismatch-retry-" + tc.name},
			})
			if status != 409 || v1ErrorCode(t, retry) != string(contract.CodeUploadStateConflict) {
				t.Fatalf("complete after mismatch = %d %v, want 409 upload_state_conflict", status, retry)
			}
		})
	}

	// Mutating calls without Idempotency-Key fail closed with 400.
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/uploads",
		userID: 1, body: v1UploadCreateBody(),
	})
	if status != 400 || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, payload)
	}
}

func TestV1UploadCompletionIdempotency(t *testing.T) {
	fx := newV1UploadFixture(t)
	status, payload := v1CreateUpload(t, fx.engine, fx.ws.PublicID, 1, "idem-create", v1UploadCreateBody())
	if status != 201 {
		t.Fatalf("create = %d %v", status, payload)
	}
	uploadID := payload["data"].(map[string]any)["id"].(string)
	completePath := "/api/v1/workspaces/" + fx.ws.PublicID + "/uploads/" + uploadID + "/complete"
	completeBody := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":1024}}`, v1UploadTestChecksum)

	complete := func(key string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: completePath, userID: 1,
			body: completeBody, headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status, payload
	}
	status, first := complete("idem-complete")
	if status != 200 {
		t.Fatalf("complete = %d %v", status, first)
	}
	firstAsset := first["data"].(map[string]any)["asset"].(map[string]any)["id"]
	status, replay := complete("idem-complete")
	if status != 200 || replay["data"].(map[string]any)["asset"].(map[string]any)["id"] != firstAsset {
		t.Fatalf("replay = %d %v, want same asset %v", status, replay, firstAsset)
	}

	// Same key, different hash conflicts even though the session is done.
	status, _, conflict := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: completePath, userID: 1,
		body:    fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":512}}`, v1UploadTestChecksum),
		headers: map[string]string{contract.IdempotencyKeyHeader: "idem-complete"},
	})
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeUploadIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 upload_idempotency_conflict", status, conflict)
	}
}

func TestV1MediaArchiveAndDerivatives(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	sessions := workspaceuploads.NewInMemorySessionStore()
	innerAssets := workspaceuploads.NewInMemoryAssetStore()
	assets := &readyV1AssetStore{InMemoryAssetStore: innerAssets, ready: map[string]bool{}, derivatives: map[string][]workspaceuploads.Derivative{}}
	objects := &fakeV1MediaObjects{objects: map[string]*ports.StoredUploadObject{}}
	engine := newV1UploadTestEngine(store, dir, sessions, assets, objects)

	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	base := "/api/v1/workspaces/" + ws.PublicID

	// Seed a ready asset with bytes + a stored derivative row.
	seeded, err := innerAssets.CreateAsset(ctx, workspaceuploads.CreateAssetParams{
		WorkspaceID: ws.PublicID, OwnerUserID: 1, MediaKind: workspaceuploads.MediaKindImage,
		MIMEType: "image/png", Width: 4, Height: 3, SizeBytes: 4,
		ChecksumSHA256: v1UploadTestChecksum, StorageProfileID: "builtin-local", StorageKey: "slot/seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	assets.ready[seeded.ID] = true
	objects.objects["builtin-local\x00slot/seed"] = &ports.StoredUploadObject{Data: []byte{0x89, 0x50, 0x4E, 0x47}, ContentType: "image/png"}
	assets.derivatives[strconv.FormatInt(seeded.InternalID, 10)] = []workspaceuploads.Derivative{{
		Tier: "preview", Format: "webp", Status: "ready", MIMEType: "image/webp",
		StorageProfileID: "builtin-local", StorageKey: "slot/seed-preview", SourceObjectGeneration: seeded.ObjectGeneration,
	}}
	objects.objects["builtin-local\x00slot/seed-preview"] = &ports.StoredUploadObject{Data: []byte("WEBP")}

	assetPath := base + "/media-assets/" + seeded.ID

	// Content serves bytes, never the envelope.
	status, header, body := doV1Raw(t, engine, v1Call{method: http.MethodGet, path: assetPath + "/content", userID: 3})
	if status != 200 || body != "\x89PNG" {
		t.Fatalf("content = %d %q, want 200 PNG bytes", status, body)
	}
	if contentType := header.Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", contentType)
	}

	// Derivatives: known tier serves its row bytes; unknown tiers 400;
	// tiers without rows 409. All plain, never JSON.
	status, _, preview := doV1Raw(t, engine, v1Call{method: http.MethodGet, path: assetPath + "/derivatives/preview", userID: 3})
	if status != 200 || preview != "WEBP" {
		t.Fatalf("derivative = %d %q, want 200 WEBP bytes", status, preview)
	}
	status, _, unknown := doV1Raw(t, engine, v1Call{method: http.MethodGet, path: assetPath + "/derivatives/nope", userID: 3})
	if status != 400 || json.Valid([]byte(unknown)) {
		t.Fatalf("unknown tier = %d %q, want plain 400", status, unknown)
	}
	status, _, _ = doV1Raw(t, engine, v1Call{method: http.MethodGet, path: assetPath + "/derivatives/thumb", userID: 3})
	if status != 409 {
		t.Fatalf("unmaterialized tier = %d, want 409", status)
	}

	// Cross-workspace derivative reads are invisible.
	other, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "other", OwnerUserID: 8})
	if err != nil {
		t.Fatal(err)
	}
	status, _, _ = doV1Raw(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + other.PublicID + "/media-assets/" + seeded.ID + "/content", userID: 8})
	if status != 404 {
		t.Fatalf("cross-workspace content = %d, want 404", status)
	}

	// Archive needs owner/admin: members 403, owners 200 with the
	// lifecycle marker. Archived assets stay metadata-readable but
	// binary reads go 410, and the list hides them.
	status, _, forbidden := doV1(t, engine, v1Call{
		method: http.MethodPost, path: assetPath + "/archive", userID: 3,
		headers: map[string]string{contract.IdempotencyKeyHeader: "archive-1"},
	})
	if status != 403 || v1ErrorCode(t, forbidden) != string(contract.CodeWorkspaceForbidden) {
		t.Fatalf("member archive = %d %v, want 403 workspace_forbidden", status, forbidden)
	}
	status, _, archived := doV1(t, engine, v1Call{
		method: http.MethodPost, path: assetPath + "/archive", userID: 1,
		headers: map[string]string{contract.IdempotencyKeyHeader: "archive-1"},
	})
	if status != 200 {
		t.Fatalf("archive = %d %v, want 200", status, archived)
	}
	if v1Data(t, archived).(map[string]any)["archived_at"] == nil {
		t.Fatalf("archived asset must carry archived_at: %v", archived)
	}
	status, _, _ = doV1Raw(t, engine, v1Call{method: http.MethodGet, path: assetPath + "/content", userID: 1})
	if status != 410 {
		t.Fatalf("archived content = %d, want 410", status)
	}
	status, _, listed := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "/media-assets?page_size=10", userID: 1})
	if status != 200 {
		t.Fatalf("list = %d %v", status, listed)
	}
	for _, item := range v1Data(t, listed).([]any) {
		if item.(map[string]any)["id"] == seeded.ID {
			t.Fatal("archived asset must be hidden from the list")
		}
	}
	if meta := listed["meta"].(map[string]any); meta["has_more"] != false {
		t.Fatalf("meta = %v, want has_more=false", meta)
	}
}

func TestV1UploadStoresNilFailsClosed(t *testing.T) {
	// Without upload/media stores every JSON session endpoint fails
	// closed with 501 (binary endpoints fail closed with plain 501).
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)
	ws := v1WorkspaceFixture(t, context.Background(), store, "nilstores", 1)
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/uploads",
		userID: 1, body: v1UploadCreateBody(),
		headers: map[string]string{contract.IdempotencyKeyHeader: "nil-key"},
	})
	if status != 501 || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("create without stores = %d %v, want 501 contract_not_implemented", status, payload)
	}
}
