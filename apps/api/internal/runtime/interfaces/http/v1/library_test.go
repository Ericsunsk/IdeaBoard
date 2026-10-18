package v1

// Workspace-scoped library contract tests (issue #806).
//
// Reuses the v1 envelope helpers from v1/test.go (doV1 asserts
// the Request-Id echo and JSON envelope shapes; v1ErrorCode asserts
// error.request_id). Covers folder/asset CRUD + move/delete,
// duplicate-save idempotency, bounded resolve rejection, the
// 404-vs-403 matrix, and fail-closed behavior without a store.

import (
	"context"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/identity/principal"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func newV1LibraryTestEngine(store tenancy.Store, libDeps ContractV1LibraryDeps, dir *staticUserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
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
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Library: libDeps})
	return router
}

type v1LibraryFixture struct {
	store  *tenancy.InMemoryStore
	lib    *librarymemory.InMemoryWorkspaceLibraryStore
	dir    *staticUserDirectory
	engine *gin.Engine
	ws     *tenancy.Workspace
	owner  int64
	member int64
	other  int64
}

func newV1LibraryFixture(t *testing.T, libDeps ContractV1LibraryDeps) *v1LibraryFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	if libDeps.Store == nil {
		libDeps.Store = librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	}
	lib, _ := libDeps.Store.(*librarymemory.InMemoryWorkspaceLibraryStore)
	engine := newV1LibraryTestEngine(store, libDeps, dir)
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "lib", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	return &v1LibraryFixture{store: store, lib: lib, dir: dir, engine: engine, ws: ws, owner: owner, member: member, other: other}
}

func (f *v1LibraryFixture) foldersPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/library/folders"
}

func (f *v1LibraryFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/library/assets"
}

const v1TestAssetURL = "https://images.unsplash.com/photo-test"

func v1SaveAssetBody(sourceID, folderID string) string {
	folder := ""
	if folderID != "" {
		folder = fmt.Sprintf(`,"folder_id":%q`, folderID)
	}
	return fmt.Sprintf(`{"data":{"source":"external","provider":"unsplash","source_id":%q,"source_url":%q,"preview_url":%q,"original_url":%q,"title":"dunes","author":"a","width":100,"height":80%s}}`,
		sourceID, v1TestAssetURL, v1TestAssetURL, v1TestAssetURL, folder)
}

// fakeLibraryBilling and fakeLibraryQuoter implement the shared
// V1CreditSettler/V1BillingQuoter seams. The library surface no longer
// wires them (permanent free tier); the search surface tests in
// v1/search_test.go still consume them — keep both types.

type fakeLibraryBilling struct {
	mu       sync.Mutex
	holdErr  error
	captures int
	releases int
}

func (f *fakeLibraryBilling) Hold(ctx context.Context, userID int64, amountCredits int64, idempotencyKey, reason string) (string, error) {
	if f.holdErr != nil {
		return "", f.holdErr
	}
	return "hold-1", nil
}

func (f *fakeLibraryBilling) Capture(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captures++
	return nil
}

func (f *fakeLibraryBilling) Release(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases++
	return nil
}

type fakeLibraryQuoter struct {
	amount int64
	err    error
}

func (f *fakeLibraryQuoter) Quote(ctx context.Context, workspacePublicID, sku string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.amount, nil
}

func TestV1LibraryFolderLifecycle(t *testing.T) {
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{})

	create := func(userID int64, key, name string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: fx.foldersPath(), userID: userID,
			body: fmt.Sprintf(`{"data":{"name":%q}}`, name)}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, fx.engine, call)
		return status, payload
	}

	status, first := create(fx.owner, "folder-key-1", "inspo")
	if status != http.StatusCreated {
		t.Fatalf("create folder = %d %v, want 201", status, first)
	}
	folderID := first["data"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(folderID, "lfolder_") {
		t.Fatalf("folder id = %q, want lfolder_...", folderID)
	}

	status, replay := create(fx.owner, "folder-key-1", "inspo")
	if status != http.StatusCreated || replay["data"].(map[string]any)["id"] != folderID {
		t.Fatalf("replay = %d %v, want same id %s", status, replay, folderID)
	}
	status, conflict := create(fx.owner, "folder-key-1", "other")
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeLibraryIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 library_idempotency_conflict", status, conflict)
	}
	status, missing := create(fx.owner, "", "nokey")
	if status != http.StatusBadRequest || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, missing)
	}
	status, dup := create(fx.member, "folder-key-2", "inspo")
	if status != http.StatusConflict {
		t.Fatalf("duplicate name = %d %v, want 409", status, dup)
	}

	// Unknown-field rejection (Baseline 5.1 strict decoding).
	status, _, strict := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.foldersPath(), userID: fx.owner,
		body: `{"data":{"name":"x","bogus":1}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "folder-key-4"}})
	if status != http.StatusBadRequest || v1ErrorCode(t, strict) != string(contract.CodeBadRequest) {
		t.Fatalf("unknown field = %d %v, want 400 bad_request", status, strict)
	}

	// Rename + pagination walk.
	status, _, renamed := doV1(t, fx.engine, v1Call{method: http.MethodPatch,
		path: fx.foldersPath() + "/" + folderID, userID: fx.member, body: `{"data":{"name":"inspo-2"}}`})
	if status != http.StatusOK || renamed["data"].(map[string]any)["name"] != "inspo-2" {
		t.Fatalf("rename = %d %v, want 200 renamed", status, renamed)
	}
	for _, name := range []string{"b", "c"} {
		if status, _ := create(fx.owner, "folder-key-"+name, name); status != http.StatusCreated {
			t.Fatalf("create %s = %d", name, status)
		}
	}
	status, _, firstPage := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.foldersPath() + "?page_size=2", userID: fx.member})
	if status != http.StatusOK || len(firstPage["data"].([]any)) != 2 || firstPage["meta"].(map[string]any)["has_more"] != true {
		t.Fatalf("page 1 = %d %v, want 2 items + has_more", status, firstPage)
	}
	cursor := firstPage["meta"].(map[string]any)["next_cursor"].(string)
	status, _, secondPage := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.foldersPath() + "?page_size=2&cursor=" + cursor, userID: fx.member})
	if status != http.StatusOK || len(secondPage["data"].([]any)) != 1 || secondPage["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("page 2 = %d %v, want 1 item exhausted", status, secondPage)
	}

	// Malformed + unknown folder ids converge to 404 (legacy integer
	// ids are malformed on v1, never a distinct status).
	for _, id := range []string{"42", "nope", "lfolder_doesnotexist00000000"} {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPatch,
			path: fx.foldersPath() + "/" + id, userID: fx.owner, body: `{"data":{"name":"x"}}`})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeLibraryFolderNotFound) {
			t.Fatalf("rename %s = %d %v, want 404 library_folder_not_found", id, status, payload)
		}
	}

	// Delete + idempotent replay.
	del := func(key string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodDelete,
			path: fx.foldersPath() + "/" + folderID, userID: fx.owner,
			headers: map[string]string{contract.IdempotencyKeyHeader: key}})
		return status, payload
	}
	if status, _ := del("folder-del-1"); status != http.StatusOK {
		t.Fatalf("delete = %d, want 200", status)
	}
	if status, payload := del("folder-del-1"); status != http.StatusOK {
		t.Fatalf("delete replay = %d %v, want 200", status, payload)
	}
	status, _, gone := doV1(t, fx.engine, v1Call{method: http.MethodPatch,
		path: fx.foldersPath() + "/" + folderID, userID: fx.owner, body: `{"data":{"name":"x"}}`})
	if status != http.StatusNotFound || v1ErrorCode(t, gone) != string(contract.CodeLibraryFolderNotFound) {
		t.Fatalf("renamed deleted = %d %v, want 404", status, gone)
	}
}

func TestV1LibraryAssetSaveMoveDelete(t *testing.T) {
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{})

	save := func(key, body string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: fx.assetsPath(), userID: fx.owner, body: body}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, fx.engine, call)
		return status, payload
	}
	status, first := save("asset-key-1", v1SaveAssetBody("u1", ""))
	if status != http.StatusCreated {
		t.Fatalf("save = %d %v, want 201", status, first)
	}
	assetID := first["data"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(assetID, "lasset_") {
		t.Fatalf("asset id = %q, want lasset_...", assetID)
	}

	// Duplicate save (different key, same provider+source) returns the
	// existing asset with 200 instead of duplicating.
	status, dup := save("asset-key-2", v1SaveAssetBody("u1", ""))
	if status != http.StatusOK || dup["data"].(map[string]any)["id"] != assetID {
		t.Fatalf("duplicate save = %d %v, want 200 same id %s", status, dup, assetID)
	}

	// Save into a folder, then move + unfile.
	status, _, folder := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.foldersPath(), userID: fx.owner,
		body: `{"data":{"name":"f"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "asset-folder-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create folder = %d %v", status, folder)
	}
	folderID := folder["data"].(map[string]any)["id"].(string)
	status, second := save("asset-key-3", v1SaveAssetBody("u2", folderID))
	if status != http.StatusCreated || second["data"].(map[string]any)["folder_id"] != folderID {
		t.Fatalf("save into folder = %d %v, want folder echoed", status, second)
	}
	secondID := second["data"].(map[string]any)["id"].(string)
	move := func(id, folder string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPatch,
			path: fx.assetsPath() + "/" + id, userID: fx.member, body: fmt.Sprintf(`{"data":{"folder_id":%q}}`, folder)})
		return status, payload
	}
	if status, moved := move(secondID, ""); status != http.StatusOK || moved["data"].(map[string]any)["folder_id"] != nil {
		t.Fatalf("unfile = %d %v, want folder cleared", status, moved)
	}
	if status, payload := move(secondID, "lfolder_doesnotexist00000000"); status != http.StatusNotFound ||
		v1ErrorCode(t, payload) != string(contract.CodeLibraryFolderNotFound) {
		t.Fatalf("move unknown folder = %d %v, want 404", status, payload)
	}

	// Cross-workspace asset ids are invisible (404, never 403):
	// saving in a second workspace then addressing from the first.
	otherWS, err := fx.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "other", OwnerUserID: fx.other})
	if err != nil {
		t.Fatal(err)
	}
	otherPath := "/api/v1/workspaces/" + otherWS.PublicID + "/library/assets"
	status, _, otherSaved := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: otherPath, userID: fx.other,
		body: v1SaveAssetBody("ux", ""), headers: map[string]string{contract.IdempotencyKeyHeader: "asset-other-1"}})
	if status != http.StatusCreated {
		t.Fatalf("save in other ws = %d %v", status, otherSaved)
	}
	foreignID := otherSaved["data"].(map[string]any)["id"].(string)
	status, _, foreign := doV1(t, fx.engine, v1Call{method: http.MethodPatch,
		path: fx.assetsPath() + "/" + foreignID, userID: fx.owner, body: `{"data":{"folder_id":""}}`})
	if status != http.StatusNotFound || v1ErrorCode(t, foreign) != string(contract.CodeLibraryAssetNotFound) {
		t.Fatalf("cross-workspace asset = %d %v, want 404 library_asset_not_found", status, foreign)
	}

	// Legacy integer ids are malformed on v1.
	status, _, legacy := doV1(t, fx.engine, v1Call{method: http.MethodDelete,
		path: fx.assetsPath() + "/42", userID: fx.owner,
		headers: map[string]string{contract.IdempotencyKeyHeader: "asset-del-int"}})
	if status != http.StatusNotFound || v1ErrorCode(t, legacy) != string(contract.CodeLibraryAssetNotFound) {
		t.Fatalf("integer id = %d %v, want 404", status, legacy)
	}

	// Delete + replay.
	del := func(key string) int {
		status, _, _ := doV1(t, fx.engine, v1Call{method: http.MethodDelete,
			path: fx.assetsPath() + "/" + assetID, userID: fx.owner,
			headers: map[string]string{contract.IdempotencyKeyHeader: key}})
		return status
	}
	if status := del("asset-del-1"); status != http.StatusOK {
		t.Fatalf("delete = %d, want 200", status)
	}
	if status := del("asset-del-1"); status != http.StatusOK {
		t.Fatalf("delete replay = %d, want 200", status)
	}
}

func TestV1LibraryResolveBounded(t *testing.T) {
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{})

	status, _, saved := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.assetsPath(), userID: fx.owner,
		body: v1SaveAssetBody("r1", ""), headers: map[string]string{contract.IdempotencyKeyHeader: "resolve-save-1"}})
	if status != http.StatusCreated {
		t.Fatalf("save = %d %v", status, saved)
	}

	resolve := func(body string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPost,
			path: "/api/v1/workspaces/" + fx.ws.PublicID + "/library/asset-resolutions", userID: fx.member, body: body})
		return status, payload
	}
	status, out := resolve(`{"data":{"items":[{"provider":"unsplash","source_id":"r1"},{"provider":"unsplash","source_id":"missing"},{"provider":"bogus","source_id":"x"}]}}`)
	if status != http.StatusOK {
		t.Fatalf("resolve = %d %v, want 200", status, out)
	}
	items := out["data"].(map[string]any)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	got := map[string]string{}
	for _, raw := range items {
		entry := raw.(map[string]any)
		key, _ := entry["source_id"].(string)
		if entry["provider"] == "bogus" {
			key = "bogus"
		}
		got[key], _ = entry["status"].(string)
	}
	if got["r1"] != "resolved" || got["missing"] != "unresolved" || got["bogus"] != "unresolved" {
		t.Fatalf("resolve statuses = %v", got)
	}
	if _, hasAsset := items[0].(map[string]any)["asset"]; !hasAsset {
		t.Fatal("resolved item must carry the asset snapshot")
	}

	// Bounded input: 21 items fail closed with 400 (not a proxy).
	big := `{"data":{"items":[`
	for i := 0; i < 21; i++ {
		if i > 0 {
			big += ","
		}
		big += fmt.Sprintf(`{"provider":"unsplash","source_id":"b%d"}`, i)
	}
	big += "]}}"
	status, over := resolve(big)
	if status != http.StatusBadRequest || v1ErrorCode(t, over) != string(contract.CodeBadRequest) {
		t.Fatalf("oversize batch = %d %v, want 400 bad_request", status, over)
	}
}

func TestV1LibraryAuthorizationMatrix(t *testing.T) {
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{})

	// Outsiders converge to 404 workspace_not_found (never 403: no
	// existence is leaked); anonymous callers get 401.
	status, _, outsider := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.foldersPath(), userID: fx.other})
	if status != http.StatusNotFound || v1ErrorCode(t, outsider) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, outsider)
	}
	status, _, anon := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.assetsPath()})
	if status != http.StatusUnauthorized || v1ErrorCode(t, anon) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon = %d %v, want 401 unauthorized", status, anon)
	}
	status, _, malformed := doV1(t, fx.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/nope/library/folders", userID: fx.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, malformed) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("malformed ws = %d %v, want 404 workspace_not_found", status, malformed)
	}

	// Nil library store fails closed with 501 (tenancy store present so
	// the 501 comes from the library surface itself).
	bare := newV1TestEngine(fx.store, fx.dir)
	status, _, payload := doV1(t, bare, v1Call{method: http.MethodGet, path: fx.foldersPath(), userID: fx.owner})
	if status != http.StatusNotImplemented || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("nil store = %d %v, want 501 contract_not_implemented", status, payload)
	}
}

func TestV1LibraryAssetSaveMediaBranch(t *testing.T) {
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{})
	fx.lib.RegisterManagedMediaForTests(fx.ws.PublicID, "asset_httpmedia0000001", nil, nil, false)

	save := func(key, body string, userID int64) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.assetsPath(), userID: userID, body: body,
			headers: map[string]string{contract.IdempotencyKeyHeader: key}})
		return status, payload
	}
	status, first := save("media-key-1", `{"data":{"source":"media","media_asset_id":"asset_httpmedia0000001","title":"http hero"}}`, fx.owner)
	if status != http.StatusCreated {
		t.Fatalf("media save = %d %v, want 201", status, first)
	}
	data := first["data"].(map[string]any)
	if data["source"] != "media" || data["media_asset_id"] != "asset_httpmedia0000001" {
		t.Fatalf("media save shape = %v, want media source with link", data)
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url"} {
		if _, present := data[key]; present {
			t.Fatalf("media save carries %q, want no provider/URL keys", key)
		}
	}
	if width, present := data["width"]; !present || width != nil {
		t.Fatalf("media width = %v (present=%v), want explicit null", width, present)
	}
	if data["title"] != "http hero" {
		t.Fatalf("media title = %v, want durable title", data["title"])
	}
	if data["display"] == nil {
		t.Fatal("media save lacks inline display")
	}
	assetID := data["id"].(string)

	// Repeat converges on the existing row (200, same id).
	status, dup := save("media-key-2", `{"data":{"source":"media","media_asset_id":"asset_httpmedia0000001"}}`, fx.member)
	if status != http.StatusOK || dup["data"].(map[string]any)["id"] != assetID {
		t.Fatalf("media repeat = %d %v, want 200 same id %s", status, dup, assetID)
	}

	// Mixed-shape payloads fail closed: media rows reject provider keys,
	// external rows reject media keys, source is required.
	for _, tt := range []struct {
		name string
		body string
	}{
		{"media with provider", `{"data":{"source":"media","media_asset_id":"asset_httpmedia0000001","provider":"unsplash"}}`},
		{"media with dims", `{"data":{"source":"media","media_asset_id":"asset_httpmedia0000001","width":10,"height":10}}`},
		{"external with media id", `{"data":{"source":"external","provider":"unsplash","source_id":"x1","source_url":"https://images.unsplash.com/x1","preview_url":"https://images.unsplash.com/x1","original_url":"https://images.unsplash.com/x1","media_asset_id":"asset_httpmedia0000001"}}`},
		{"missing source", `{"data":{"provider":"unsplash","source_id":"x1"}}`},
		{"unknown source", `{"data":{"source":"remote","media_asset_id":"asset_httpmedia0000001"}}`},
	} {
		if status, payload := save("media-bad-"+tt.name, tt.body, fx.owner); status != http.StatusUnprocessableEntity ||
			v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tt.name, status, payload)
		}
	}

	// Unknown media fails as not found with nothing written.
	status, missing := save("media-key-3", `{"data":{"source":"media","media_asset_id":"asset_missing0000000001"}}`, fx.owner)
	if status != http.StatusNotFound || v1ErrorCode(t, missing) != string(contract.CodeLibraryAssetNotFound) {
		t.Fatalf("unknown media = %d %v, want 404 library_asset_not_found", status, missing)
	}
}
