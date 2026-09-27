package httpapi

// Workspace-scoped search + vision-query contract tests (issue #806).
//
// Reuses the v1 envelope helpers from contract_v1_test.go (doV1 asserts
// the Request-Id echo and JSON envelope shapes; v1ErrorCode asserts
// error.request_id). Covers search cursor + provider filters +
// explicit query validation, vision quote/settle + billing-failure
// branches, idempotent replay + conflict, and the 404-vs-403 matrix.

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
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

func newV1SearchTestEngine(store tenancy.Store, searchDeps ContractV1SearchDeps, dir *staticUserDirectory) *gin.Engine {
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
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Search: searchDeps})
	return router
}

// fakeSearchBackend honors provider filters and pages with offset
// cursors ("offset:N"); err forces a backend outage, cursorErr forces
// a cursor-binding failure.
type fakeSearchBackend struct {
	mu        sync.Mutex
	items     []searchcatalog.V1SearchResult
	err       error
	cursorErr error
	calls     int
}

func (f *fakeSearchBackend) Search(ctx context.Context, query string, providers []string, limit int, cursor string) ([]searchcatalog.V1SearchResult, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, "", false, f.err
	}
	offset := 0
	if strings.TrimSpace(cursor) != "" {
		if !strings.HasPrefix(cursor, "offset:") {
			return nil, "", false, f.cursorErr
		}
		parsed, parseErr := strconv.Atoi(strings.TrimPrefix(cursor, "offset:"))
		if parseErr != nil || parsed < 0 {
			return nil, "", false, f.cursorErr
		}
		offset = parsed
	}
	filtered := make([]searchcatalog.V1SearchResult, 0, len(f.items))
	for _, item := range f.items {
		if v1ProviderSelected(providers, item.Provider) {
			filtered = append(filtered, item)
		}
	}
	if offset > len(filtered) {
		return nil, "", false, f.cursorErr
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	next := ""
	hasMore := end < len(filtered)
	if hasMore {
		next = "offset:" + strconv.Itoa(end)
	}
	return filtered[offset:end], next, hasMore, nil
}

type fakeVisionExtractor struct {
	mu        sync.Mutex
	query     string
	err       error
	calls     int
	lastInput V1VisionExtractInput
}

func (f *fakeVisionExtractor) ExtractVisionQuery(ctx context.Context, input V1VisionExtractInput) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastInput = input
	if f.err != nil {
		return "", f.err
	}
	return f.query, nil
}

type v1SearchFixture struct {
	store  *tenancy.InMemoryStore
	lib    *library.InMemoryWorkspaceLibraryStore
	dir    *staticUserDirectory
	engine *gin.Engine
	ws     *tenancy.Workspace
	owner  int64
	member int64
	other  int64
}

func newV1SearchFixture(t *testing.T, searchDeps ContractV1SearchDeps) *v1SearchFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	if searchDeps.Library == nil {
		searchDeps.Library = library.NewInMemoryWorkspaceLibraryStoreForTests()
	}
	lib, _ := searchDeps.Library.(*library.InMemoryWorkspaceLibraryStore)
	engine := newV1SearchTestEngine(store, searchDeps, dir)
	const owner, member, other = 1, 2, 99
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "search", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	return &v1SearchFixture{store: store, lib: lib, dir: dir, engine: engine, ws: ws, owner: owner, member: member, other: other}
}

func (f *v1SearchFixture) searchPath(query string) string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/search" + query
}

func (f *v1SearchFixture) visionPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/search/vision-queries"
}

func v1SearchItem(provider, sourceID string) searchcatalog.V1SearchResult {
	url := "https://images.unsplash.com/" + sourceID
	if provider == "pexels" {
		url = "https://images.pexels.com/" + sourceID
	}
	return searchcatalog.V1SearchResult{Provider: provider, SourceID: sourceID, Title: "t-" + sourceID,
		Author: "a", SourceURL: url, PreviewURL: url, OriginalURL: url, Width: 100, Height: 80}
}

func TestV1SearchValidation(t *testing.T) {
	backend := &fakeSearchBackend{}
	fx := newV1SearchFixture(t, ContractV1SearchDeps{Backend: backend})

	cases := []struct {
		name     string
		query    string
		wantCode string
		want     int
	}{
		{"missing q", "", string(contract.CodeValidationFailed), http.StatusUnprocessableEntity},
		{"blank q", "?q=%20%20", string(contract.CodeValidationFailed), http.StatusUnprocessableEntity},
		{"overlong q", "?q=" + strings.Repeat("x", 201), string(contract.CodeValidationFailed), http.StatusUnprocessableEntity},
		{"bad provider", "?q=cafe&provider=bogus", string(contract.CodeValidationFailed), http.StatusUnprocessableEntity},
		{"bad page size", "?q=cafe&page_size=101", string(contract.CodeBadRequest), http.StatusBadRequest},
	}
	for _, tc := range cases {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath(tc.query), userID: fx.member})
		if status != tc.want || v1ErrorCode(t, payload) != tc.wantCode {
			t.Fatalf("%s = %d %v, want %d %s", tc.name, status, payload, tc.want, tc.wantCode)
		}
	}

	// Cursor binding failures converge to pagination_cursor_invalid.
	backend.cursorErr = errV1CursorInvalid
	status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath("?q=cafe&cursor=garbage"), userID: fx.member})
	if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("bad cursor = %d %v, want 400 pagination_cursor_invalid", status, payload)
	}
	if backend.calls != 1 {
		t.Fatalf("backend calls = %d, want the cursor rejected by the backend seam", backend.calls)
	}
}

func TestV1SearchCursorAndFilter(t *testing.T) {
	backend := &fakeSearchBackend{items: []searchcatalog.V1SearchResult{
		v1SearchItem("unsplash", "u1"), v1SearchItem("pexels", "p1"), v1SearchItem("unsplash", "u2"),
		v1SearchItem("pexels", "p2"), v1SearchItem("unsplash", "u3"),
	}}
	fx := newV1SearchFixture(t, ContractV1SearchDeps{Backend: backend})

	get := func(query string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath(query), userID: fx.member})
		return status, payload
	}
	status, first := get("?q=warm+cafe&page_size=2")
	if status != http.StatusOK {
		t.Fatalf("page 1 = %d %v", status, first)
	}
	if len(first["data"].([]any)) != 2 || first["meta"].(map[string]any)["has_more"] != true {
		t.Fatalf("page 1 = %v, want 2 items + has_more", first)
	}
	cursor := first["meta"].(map[string]any)["next_cursor"].(string)
	status, second := get("?q=warm+cafe&page_size=2&cursor=" + cursor)
	if status != http.StatusOK || len(second["data"].([]any)) != 2 {
		t.Fatalf("page 2 = %d %v, want 2 items", status, second)
	}
	lastCursor := second["meta"].(map[string]any)["next_cursor"].(string)
	status, third := get("?q=warm+cafe&page_size=2&cursor=" + lastCursor)
	if status != http.StatusOK || len(third["data"].([]any)) != 1 || third["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("page 3 = %d %v, want 1 item exhausted", status, third)
	}
	entry := first["data"].([]any)[0].(map[string]any)
	for _, field := range []string{"provider", "source_id", "source_url", "preview_url", "original_url"} {
		if entry[field] == nil || entry[field] == "" {
			t.Fatalf("result missing %s: %v", field, entry)
		}
	}

	// Provider filter narrows to one supplier across pages.
	status, filtered := get("?q=warm+cafe&page_size=10&provider=pexels")
	if status != http.StatusOK {
		t.Fatalf("filtered = %d %v", status, filtered)
	}
	items := filtered["data"].([]any)
	if len(items) != 2 {
		t.Fatalf("filtered items = %d, want 2", len(items))
	}
	for _, raw := range items {
		if raw.(map[string]any)["provider"] != "pexels" {
			t.Fatalf("unfiltered provider leaked: %v", raw)
		}
	}
}

func TestV1SearchBackendUnavailable(t *testing.T) {
	fx := newV1SearchFixture(t, ContractV1SearchDeps{Backend: nil})
	status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath("?q=cafe"), userID: fx.owner})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeSearchProviderUnavailable) {
		t.Fatalf("nil backend = %d %v, want 503 search_provider_unavailable", status, payload)
	}

	outage := &fakeSearchBackend{err: errors.New("suppliers down")}
	fx = newV1SearchFixture(t, ContractV1SearchDeps{Backend: outage})
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath("?q=cafe"), userID: fx.owner})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("backend outage = %d %v, want 503 service_unavailable", status, payload)
	}
}

func TestV1VisionQueryLifecycle(t *testing.T) {
	extractor := &fakeVisionExtractor{query: "warm cafe"}
	billing := &fakeLibraryBilling{}
	quoter := &fakeLibraryQuoter{amount: 3}
	fx := newV1SearchFixture(t, ContractV1SearchDeps{
		Backend: &fakeSearchBackend{}, Quoter: quoter, Settler: billing, Extractor: extractor,
	})

	// Save an owned library asset to reference from the vision query
	// (direct store save: this fixture wires the library store into the
	// search deps only, keeping the library HTTP surface out of scope).
	asset, _, err := fx.lib.SaveAsset(context.Background(), fx.ws.PublicID,
		library.V1SaveAssetInput{Asset: library.AssetInput{
			Provider: "unsplash", SourceID: "v1",
			SourceURL: v1TestAssetURL, PreviewURL: v1TestAssetURL, OriginalURL: v1TestAssetURL,
			Title: "dunes", Width: 100, Height: 80,
		}}, "lasset_visionref000000001")
	if err != nil {
		t.Fatal(err)
	}

	post := func(key, body string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: fx.visionPath(), userID: fx.member, body: body}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, fx.engine, call)
		return status, payload
	}
	body := fmt.Sprintf(`{"data":{"user_text":"  warm   cafe ","library_asset_ids":[%q]}}`, asset.PublicID)
	status, created := post("vision-key-1", body)
	if status != http.StatusCreated {
		t.Fatalf("vision query = %d %v, want 201", status, created)
	}
	data := created["data"].(map[string]any)
	if !strings.HasPrefix(data["operation_id"].(string), "visq_") {
		t.Fatalf("operation id = %v, want visq_...", data["operation_id"])
	}
	if data["query"] != "warm cafe" || data["status"] != "succeeded" {
		t.Fatalf("extraction = %v, want normalized query", data)
	}
	if billing.captures != 1 || billing.releases != 0 {
		t.Fatalf("billing captures=%d releases=%d, want 1/0", billing.captures, billing.releases)
	}
	if len(extractor.lastInput.References) != 1 || extractor.lastInput.References[0].SourceID != "v1" {
		t.Fatalf("extractor input = %+v, want the resolved owned reference (no bytes)", extractor.lastInput)
	}

	// Replay converges on the same operation; extraction runs once.
	status, replay := post("vision-key-1", body)
	if status != http.StatusCreated || replay["data"].(map[string]any)["operation_id"] != data["operation_id"] {
		t.Fatalf("replay = %d %v, want same operation", status, replay)
	}
	if replay["data"].(map[string]any)["replayed"] != true {
		t.Fatalf("replay = %v, want replayed=true", replay)
	}
	if extractor.calls != 1 {
		t.Fatalf("extractor calls = %d, want 1", extractor.calls)
	}

	// Same key + different request conflicts.
	status, conflict := post("vision-key-1", `{"data":{"user_text":"different"}}`)
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeSearchIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 search_idempotency_conflict", status, conflict)
	}
	status, missing := post("", body)
	if status != http.StatusBadRequest || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, missing)
	}

	// Malformed asset ids fail validation (422); cross-workspace ids 404.
	status, badShape := post("vision-bad-1", `{"data":{"user_text":"x","library_asset_ids":["42"]}}`)
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, badShape) != string(contract.CodeValidationFailed) {
		t.Fatalf("malformed ref = %d %v, want 422 validation_failed", status, badShape)
	}
	otherWS, err := fx.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "vision-other", OwnerUserID: fx.other})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := fx.lib.SaveAsset(context.Background(), otherWS.PublicID,
		library.V1SaveAssetInput{Asset: library.AssetInput{
			Provider: "unsplash", SourceID: "foreign",
			SourceURL: v1TestAssetURL, PreviewURL: v1TestAssetURL, OriginalURL: v1TestAssetURL,
		}}, "lasset_foreign00000000001")
	if err != nil {
		t.Fatal(err)
	}
	status, foreignPayload := post("vision-foreign-1", fmt.Sprintf(`{"data":{"user_text":"x","library_asset_ids":[%q]}}`, foreign.PublicID))
	if status != http.StatusNotFound || v1ErrorCode(t, foreignPayload) != string(contract.CodeLibraryAssetNotFound) {
		t.Fatalf("cross-workspace ref = %d %v, want 404 library_asset_not_found", status, foreignPayload)
	}
}

func TestV1VisionQueryFailureBranches(t *testing.T) {
	build := func(quoter *fakeLibraryQuoter, billing *fakeLibraryBilling, extractor V1VisionExtractor) *v1SearchFixture {
		return newV1SearchFixture(t, ContractV1SearchDeps{
			Backend: &fakeSearchBackend{}, Quoter: quoter, Settler: billing, Extractor: extractor,
		})
	}
	post := func(fx *v1SearchFixture, key, body string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.visionPath(),
			userID: fx.owner, body: body, headers: map[string]string{contract.IdempotencyKeyHeader: key}})
		return status, payload
	}

	fx := build(&fakeLibraryQuoter{err: errors.New("catalog down")}, &fakeLibraryBilling{}, &fakeVisionExtractor{query: "x"})
	status, payload := post(fx, "vf-q", `{"data":{"user_text":"cafe"}}`)
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeBillingQuoteUnavailable) {
		t.Fatalf("quote failure = %d %v, want 503 billing_quote_unavailable", status, payload)
	}

	// Unpriced/unregistered SKUs degrade to free (no retry, no 422):
	// the quoter surfacing ErrPricingUnavailable or ErrSKUNotFound
	// proceeds at amount 0 with ledger skipped and a billing_deferred
	// metric emitted. The sentinels stay distinct via the reason label.
	deferred := []struct {
		err    error
		reason string
	}{
		{billing.ErrPricingUnavailable, "pricing_unavailable"},
		{billing.ErrSKUNotFound, "sku_not_found"},
	}
	for i, tc := range deferred {
		var got []struct{ sku, reason, operationID string }
		prev := v1VisionBillingDeferredObserve
		v1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
			got = append(got, struct{ sku, reason, operationID string }{sku, reason, operationID})
		}
		settler := &fakeLibraryBilling{}
		fx = build(&fakeLibraryQuoter{err: tc.err}, settler, &fakeVisionExtractor{query: "x"})
		status, payload = post(fx, fmt.Sprintf("vf-p-%d", i), `{"data":{"user_text":"cafe"}}`)
		v1VisionBillingDeferredObserve = prev
		if status != http.StatusCreated {
			t.Fatalf("deferred pricing (%v) = %d %v, want 201", tc.err, status, payload)
		}
		if payload["data"].(map[string]any)["query"] != "x" {
			t.Fatalf("deferred query = %v, want x", payload)
		}
		if len(got) != 1 || got[0].sku != V1VisionSKU || got[0].reason != tc.reason || got[0].operationID == "" {
			t.Fatalf("deferred metric = %+v, want one billing_deferred with reason %q", got, tc.reason)
		}
		if settler.captures != 0 || settler.releases != 0 {
			t.Fatalf("deferred ledger captures=%d releases=%d, want 0/0 (amount 0 skipped)", settler.captures, settler.releases)
		}
	}

	// Quoted zero without an error degrades the same way (free-marker).
	{
		var got int
		prev := v1VisionBillingDeferredObserve
		v1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
			got++
			if reason != "unpriced_zero" {
				t.Fatalf("zero metric reason = %q, want unpriced_zero", reason)
			}
		}
		settler := &fakeLibraryBilling{}
		fx = build(&fakeLibraryQuoter{amount: 0}, settler, &fakeVisionExtractor{query: "x"})
		status, payload = post(fx, "vf-zero", `{"data":{"user_text":"cafe"}}`)
		v1VisionBillingDeferredObserve = prev
		if status != http.StatusCreated {
			t.Fatalf("zero quote = %d %v, want 201", status, payload)
		}
		if got != 1 {
			t.Fatalf("zero metric calls = %d, want 1", got)
		}
		if settler.captures != 0 || settler.releases != 0 {
			t.Fatalf("zero ledger captures=%d releases=%d, want 0/0", settler.captures, settler.releases)
		}
	}

	billing := &fakeLibraryBilling{holdErr: errV1BillingRestricted}
	fx = build(&fakeLibraryQuoter{amount: 3}, billing, &fakeVisionExtractor{query: "x"})
	status, payload = post(fx, "vf-h", `{"data":{"user_text":"cafe"}}`)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("hold rejection = %d %v, want 403 billing_wallet_restricted", status, payload)
	}

	billing = &fakeLibraryBilling{}
	extractor := &fakeVisionExtractor{err: errors.New("gateway 504")}
	fx = build(&fakeLibraryQuoter{amount: 3}, billing, extractor)
	status, payload = post(fx, "vf-p", `{"data":{"user_text":"cafe"}}`)
	if status != http.StatusBadGateway || v1ErrorCode(t, payload) != string(contract.CodeSearchVisionProviderError) {
		t.Fatalf("provider failure = %d %v, want 502 search_vision_provider_error", status, payload)
	}
	if billing.releases != 1 || billing.captures != 0 {
		t.Fatalf("releases=%d captures=%d, want hold refunded 1/0", billing.releases, billing.captures)
	}

	fx = build(&fakeLibraryQuoter{amount: 3}, &fakeLibraryBilling{}, nil)
	status, payload = post(fx, "vf-n", `{"data":{"user_text":"cafe"}}`)
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeSearchVisionNotConfigured) {
		t.Fatalf("nil extractor = %d %v, want 503 search_vision_not_configured", status, payload)
	}
}

func TestV1SearchAuthorizationMatrix(t *testing.T) {
	fx := newV1SearchFixture(t, ContractV1SearchDeps{Backend: &fakeSearchBackend{}})

	status, _, outsider := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.searchPath("?q=cafe"), userID: fx.other})
	if status != http.StatusNotFound || v1ErrorCode(t, outsider) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, outsider)
	}
	status, _, anon := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.visionPath(), userID: 0,
		body: `{"data":{"user_text":"cafe"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "anon"}})
	if status != http.StatusUnauthorized || v1ErrorCode(t, anon) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon = %d %v, want 401 unauthorized", status, anon)
	}
}
