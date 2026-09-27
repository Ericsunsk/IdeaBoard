package httpapi

// Workspace-scoped library + materialization contract tests (issue #806).
//
// Reuses the v1 envelope helpers from contract_v1_test.go (doV1 asserts
// the Request-Id echo and JSON envelope shapes; v1ErrorCode asserts
// error.request_id). Covers folder/asset CRUD + move/delete,
// duplicate-save idempotency, bounded resolve rejection,
// materialization replay + asset link + billing branches, the
// 404-vs-403 matrix, and fail-closed behavior without a store.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

func newV1LibraryTestEngine(store tenancy.Store, libDeps ContractV1LibraryDeps, dir *staticUserDirectory) *gin.Engine {
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
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Library: libDeps})
	return router
}

type v1LibraryFixture struct {
	store  *tenancy.InMemoryStore
	lib    *library.InMemoryWorkspaceLibraryStore
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
		libDeps.Store = library.NewInMemoryWorkspaceLibraryStoreForTests()
	}
	lib, _ := libDeps.Store.(*library.InMemoryWorkspaceLibraryStore)
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

func (f *v1LibraryFixture) materializationsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/library/materializations"
}

const v1TestAssetURL = "https://images.unsplash.com/photo-test"

func v1SaveAssetBody(sourceID, folderID string) string {
	folder := ""
	if folderID != "" {
		folder = fmt.Sprintf(`,"folder_id":%q`, folderID)
	}
	return fmt.Sprintf(`{"data":{"provider":"unsplash","source_id":%q,"source_url":%q,"preview_url":%q,"original_url":%q,"title":"dunes","author":"a","width":100,"height":80%s}}`,
		sourceID, v1TestAssetURL, v1TestAssetURL, v1TestAssetURL, folder)
}

type fakeLibraryMaterializer struct {
	mu      sync.Mutex
	mediaID string
	err     error
	calls   int
	last    *library.V1Asset
}

func (f *fakeLibraryMaterializer) MaterializeWorkspaceAsset(ctx context.Context, workspacePublicID string, asset *library.V1Asset) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = asset
	if f.err != nil {
		return "", f.err
	}
	return f.mediaID, nil
}

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

func TestV1LibraryMaterialization(t *testing.T) {
	materializer := &fakeLibraryMaterializer{mediaID: "asset_abc123def456"}
	billing := &fakeLibraryBilling{}
	quoter := &fakeLibraryQuoter{amount: 5}
	fx := newV1LibraryFixture(t, ContractV1LibraryDeps{Materializer: materializer, Billing: billing, Quoter: quoter})

	status, _, saved := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.assetsPath(), userID: fx.owner,
		body: v1SaveAssetBody("m1", ""), headers: map[string]string{contract.IdempotencyKeyHeader: "mat-save-1"}})
	if status != http.StatusCreated {
		t.Fatalf("save = %d %v", status, saved)
	}
	assetID := saved["data"].(map[string]any)["id"].(string)

	post := func(key, body string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: fx.materializationsPath(), userID: fx.member, body: body}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, fx.engine, call)
		return status, payload
	}
	status, created := post("mat-key-1", fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID))
	if status != http.StatusAccepted {
		t.Fatalf("materialize = %d %v, want 202", status, created)
	}
	data := created["data"].(map[string]any)
	if !strings.HasPrefix(data["id"].(string), "lmat_") {
		t.Fatalf("materialization id = %v, want lmat_...", data["id"])
	}
	if data["status"] != "completed" || data["media_asset_id"] != "asset_abc123def456" {
		t.Fatalf("materialization = %v, want completed + asset_ link", data)
	}
	if billing.captures != 1 || billing.releases != 0 {
		t.Fatalf("billing captures=%d releases=%d, want 1/0", billing.captures, billing.releases)
	}

	// Replay converges on the same job.
	status, replay := post("mat-key-1", fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID))
	if status != http.StatusAccepted || replay["data"].(map[string]any)["id"] != data["id"] {
		t.Fatalf("replay = %d %v, want same job %v", status, replay, data["id"])
	}
	if materializer.calls != 1 {
		t.Fatalf("materializer calls = %d, want 1 (replay must not re-run)", materializer.calls)
	}

	// Status-addressable GET.
	status, _, got := doV1(t, fx.engine, v1Call{method: http.MethodGet,
		path: fx.materializationsPath() + "/" + data["id"].(string), userID: fx.owner})
	if status != http.StatusOK || got["data"].(map[string]any)["status"] != "completed" {
		t.Fatalf("get = %d %v, want completed", status, got)
	}
	status, _, unknown := doV1(t, fx.engine, v1Call{method: http.MethodGet,
		path: fx.materializationsPath() + "/lmat_doesnotexist0000000", userID: fx.owner})
	if status != http.StatusNotFound || v1ErrorCode(t, unknown) != string(contract.CodeLibraryMaterializationNotFound) {
		t.Fatalf("unknown job = %d %v, want 404", status, unknown)
	}

	// Materializer failure folds into a failed job record (202) and
	// releases the hold.
	materializer.err = errors.New("provider fetch exploded")
	status, failed := post("mat-key-2", fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID))
	if status != http.StatusAccepted || failed["data"].(map[string]any)["status"] != "failed" {
		t.Fatalf("failed job = %d %v, want 202 failed", status, failed)
	}
	if billing.releases != 1 {
		t.Fatalf("releases = %d, want 1 after failure", billing.releases)
	}
	materializer.err = nil
}

func TestV1LibraryMaterializationBillingBranches(t *testing.T) {
	newBillingFixture := func(quoter *fakeLibraryQuoter, billing *fakeLibraryBilling, materializer *fakeLibraryMaterializer) *v1LibraryFixture {
		return newV1LibraryFixture(t, ContractV1LibraryDeps{Materializer: materializer, Billing: billing, Quoter: quoter})
	}
	saveOne := func(fx *v1LibraryFixture) string {
		status, _, saved := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.assetsPath(), userID: fx.owner,
			body: v1SaveAssetBody("b1", ""), headers: map[string]string{contract.IdempotencyKeyHeader: "billsave"}})
		if status != http.StatusCreated {
			t.Fatalf("save = %d %v", status, saved)
		}
		return saved["data"].(map[string]any)["id"].(string)
	}

	fx := newBillingFixture(&fakeLibraryQuoter{err: errors.New("catalog down")}, &fakeLibraryBilling{}, &fakeLibraryMaterializer{mediaID: "asset_x"})
	assetID := saveOne(fx)
	status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.materializationsPath(), userID: fx.owner,
		body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
		headers: map[string]string{contract.IdempotencyKeyHeader: "bill-q"}})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeBillingQuoteUnavailable) {
		t.Fatalf("quote failure = %d %v, want 503 billing_quote_unavailable", status, payload)
	}

	// Unpriced/unregistered SKUs are deterministic 422s (no retry),
	// whether the quoter surfaces ErrPricingUnavailable (the production
	// catalog normalizes unknown SKUs to it) or ErrSKUNotFound.
	for i, quoteErr := range []error{billing.ErrPricingUnavailable, billing.ErrSKUNotFound} {
		fx = newBillingFixture(&fakeLibraryQuoter{err: quoteErr}, &fakeLibraryBilling{}, &fakeLibraryMaterializer{mediaID: "asset_x"})
		assetID = saveOne(fx)
		status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.materializationsPath(), userID: fx.owner,
			body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
			headers: map[string]string{contract.IdempotencyKeyHeader: fmt.Sprintf("bill-p-%d", i)}})
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeBillingPricingUnavailable) {
			t.Fatalf("pricing failure (%v) = %d %v, want 422 billing_pricing_unavailable", quoteErr, status, payload)
		}
	}

	fx = newBillingFixture(&fakeLibraryQuoter{amount: 5},
		&fakeLibraryBilling{holdErr: errV1BillingRestricted}, &fakeLibraryMaterializer{mediaID: "asset_x"})
	assetID = saveOne(fx)
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.materializationsPath(), userID: fx.owner,
		body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
		headers: map[string]string{contract.IdempotencyKeyHeader: "bill-h"}})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("hold rejection = %d %v, want 403 billing_wallet_restricted", status, payload)
	}

	// Missing seams fail closed without touching billing.
	fx = newV1LibraryFixture(t, ContractV1LibraryDeps{})
	assetID = saveOne(fx)
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.materializationsPath(), userID: fx.owner,
		body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
		headers: map[string]string{contract.IdempotencyKeyHeader: "bill-nil"}})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("nil seams = %d %v, want 503", status, payload)
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
