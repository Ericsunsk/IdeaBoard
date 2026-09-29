package httpapi

// Control Model Pool handler tests (issue #849): per-Service pool reads
// and atomic replaces behind control:billing.
//
// The fake below is an in-memory stand-in for *catalog.Repository with
// the same observable semantics (version compare-and-swap, no-op saves
// that do not bump, effective-change bumps, enabled-without-resolution
// rejection). Repository-level atomicity and concurrency races are
// covered in the catalog package (wave 1); these tests lock the HTTP
// surface: SKU->service mapping, validation/conflict status codes,
// version monotonicity through the API, and the auth/fail-closed gates.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"

	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/contract"
)

type fakeControlModelPools struct {
	mu    sync.Mutex
	pools map[string]fakePoolState

	replaceCalls int
	lastService  string
	lastExpected uint64
	lastInputs   []catalog.PoolEntryInput
}

type fakePoolState struct {
	version uint64
	entries []routing.ModelPoolEntry
}

func newFakeControlModelPools() *fakeControlModelPools {
	return &fakeControlModelPools{pools: map[string]fakePoolState{}}
}

func (f *fakeControlModelPools) seed(service string, version uint64, entries []routing.ModelPoolEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pools[service] = fakePoolState{version: version, entries: entries}
}

func (f *fakeControlModelPools) ListPool(_ context.Context, service string) (routing.Pool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service == "vision" {
		return routing.Pool{}, fmt.Errorf("fake: %w: unsupported service %q", catalog.ErrInvalidPoolInput, service)
	}
	state := f.pools[service]
	entries := append([]routing.ModelPoolEntry(nil), state.entries...)
	return routing.Pool{Service: service, Version: state.version, Entries: entries}, nil
}

func (f *fakeControlModelPools) ReplacePool(_ context.Context, service string, expectedVersion uint64, inputs []catalog.PoolEntryInput) (catalog.ReplacePoolResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaceCalls++
	f.lastService = service
	f.lastExpected = expectedVersion
	f.lastInputs = append([]catalog.PoolEntryInput(nil), inputs...)
	if service == "vision" {
		return catalog.ReplacePoolResult{}, fmt.Errorf("fake: %w: unsupported service %q", catalog.ErrInvalidPoolInput, service)
	}
	for i, input := range inputs {
		if input.Enabled && len(input.Resolutions) == 0 {
			return catalog.ReplacePoolResult{}, fmt.Errorf("fake: %w: entry %d enabled without resolutions", catalog.ErrInvalidPoolInput, i)
		}
	}
	state := f.pools[service]
	if state.version != expectedVersion {
		return catalog.ReplacePoolResult{}, &catalog.PoolVersionConflict{Service: service, Expected: expectedVersion, Current: state.version}
	}
	desired := make([]routing.ModelPoolEntry, 0, len(inputs))
	for _, input := range inputs {
		resolutions := make([]routing.Resolution, 0, len(input.Resolutions))
		for _, raw := range input.Resolutions {
			resolutions = append(resolutions, routing.Resolution(raw))
		}
		sort.Slice(resolutions, func(a, b int) bool { return resolutions[a] < resolutions[b] })
		desired = append(desired, routing.ModelPoolEntry{Model: input.ModelID, Enabled: input.Enabled, SupportedResolutions: resolutions})
	}
	sort.Slice(desired, func(a, b int) bool { return desired[a].Model < desired[b].Model })
	if poolEntriesEqual(state.entries, desired) {
		return catalog.ReplacePoolResult{
			Version: state.version,
			Changed: false,
			Pool:    routing.Pool{Service: service, Version: state.version, Entries: append([]routing.ModelPoolEntry(nil), state.entries...)},
		}, nil
	}
	next := state.version + 1
	if state.version == 0 {
		next = 1
	}
	f.pools[service] = fakePoolState{version: next, entries: desired}
	return catalog.ReplacePoolResult{
		Version: next,
		Changed: true,
		Pool:    routing.Pool{Service: service, Version: next, Entries: append([]routing.ModelPoolEntry(nil), desired...)},
	}, nil
}

func poolEntriesEqual(left, right []routing.ModelPoolEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Model != right[i].Model || left[i].Enabled != right[i].Enabled {
			return false
		}
		if len(left[i].SupportedResolutions) != len(right[i].SupportedResolutions) {
			return false
		}
		for j := range left[i].SupportedResolutions {
			if left[i].SupportedResolutions[j] != right[i].SupportedResolutions[j] {
				return false
			}
		}
	}
	return true
}

func poolHarness(t *testing.T, pools *fakeControlModelPools) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(deps *ContractControlDeps) {
		deps.ModelPools = pools
	})
}

func poolAdminCall(method, path, body string, keySuffix int) controlCall {
	call := controlCall{method: method, path: path, body: body, userID: controlOperatorID, role: "admin"}
	if method != http.MethodGet {
		call.headers = controlMutationHeaders(fmt.Sprintf("pool-key-%d", keySuffix), "pool review")
	}
	return call
}

func poolData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("success envelope requires data object: %v", payload)
	}
	return data
}

func TestControlModelPoolGet(t *testing.T) {
	pools := newFakeControlModelPools()
	pools.seed("generation", 2, []routing.ModelPoolEntry{
		{Model: "a/first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution1K}},
		{Model: "b/second", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K, routing.Resolution4K}},
	})
	h := poolHarness(t, pools)

	status, _, payload := h.serve(poolAdminCall(http.MethodGet, "/api/control/v1/billing/catalog/ai-image:generation/pool", "", 0))
	if status != http.StatusOK {
		t.Fatalf("GET pool status = %d, want 200: %v", status, payload)
	}
	data := poolData(t, payload)
	if data["service"] != "generation" || data["sku"] != "ai-image:generation" {
		t.Fatalf("pool identity = %v, want service generation sku ai-image:generation", data)
	}
	if version, _ := data["version"].(float64); version != 2 {
		t.Fatalf("pool version = %v, want 2", data["version"])
	}
	entries, _ := data["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("pool entries = %v, want 2 rows", data["entries"])
	}
}

func TestControlModelPoolGetNeverWritten(t *testing.T) {
	h := poolHarness(t, newFakeControlModelPools())

	status, _, payload := h.serve(poolAdminCall(http.MethodGet, "/api/control/v1/billing/catalog/ai-image:render/pool", "", 0))
	if status != http.StatusOK {
		t.Fatalf("GET unwritten pool status = %d, want 200: %v", status, payload)
	}
	data := poolData(t, payload)
	if version, _ := data["version"].(float64); version != 0 {
		t.Fatalf("unwritten pool version = %v, want 0", data["version"])
	}
	if entries, _ := data["entries"].([]any); len(entries) != 0 {
		t.Fatalf("unwritten pool entries = %v, want empty", data["entries"])
	}
}

func TestControlModelPoolRejectsNonImageSKU(t *testing.T) {
	pools := newFakeControlModelPools()
	h := poolHarness(t, pools)

	for _, call := range []controlCall{
		poolAdminCall(http.MethodGet, "/api/control/v1/billing/catalog/legacy:removed/pool", "", 0),
		poolAdminCall(http.MethodPut, "/api/control/v1/billing/catalog/legacy:removed/pool", `{"expected_version":0,"entries":[]}`, 1),
	} {
		status, _, payload := h.serve(call)
		if status != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want 400: %v", call.method, call.path, status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
			t.Fatalf("code = %q, want bad_request", code)
		}
	}
	if pools.replaceCalls != 0 {
		t.Fatalf("replaceCalls = %d, want 0 (rejected before the store)", pools.replaceCalls)
	}
}

func TestControlModelPoolRejectsUnknownService(t *testing.T) {
	h := poolHarness(t, newFakeControlModelPools())

	status, _, payload := h.serve(poolAdminCall(http.MethodGet, "/api/control/v1/billing/catalog/ai-image:vision/pool", "", 0))
	if status != http.StatusBadRequest {
		t.Fatalf("GET vision pool status = %d, want 400: %v", status, payload)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
		t.Fatalf("code = %q, want bad_request", code)
	}
}

func TestControlModelPoolReplaceVersionLifecycle(t *testing.T) {
	pools := newFakeControlModelPools()
	h := poolHarness(t, pools)

	put := func(expected uint64, body string, key int) (int, map[string]any) {
		status, _, payload := h.serve(poolAdminCall(http.MethodPut, "/api/control/v1/billing/catalog/ai-image:generation/pool", body, key))
		return status, payload
	}

	// First effective write starts at version 1.
	status, payload := put(0, `{"expected_version":0,"entries":[{"model_id":"a/first","enabled":true,"resolutions":["1k"]}]}`, 11)
	if status != http.StatusOK {
		t.Fatalf("first PUT status = %d, want 200: %v", status, payload)
	}
	data := poolData(t, payload)
	if version, _ := data["version"].(float64); version != 1 {
		t.Fatalf("first PUT version = %v, want 1", data["version"])
	}
	if changed, _ := data["changed"].(bool); !changed {
		t.Fatalf("first PUT changed = %v, want true", data["changed"])
	}

	// Effective change bumps monotonically.
	status, payload = put(1, `{"expected_version":1,"entries":[{"model_id":"a/first","enabled":true,"resolutions":["1k","2k"]}]}`, 12)
	if status != http.StatusOK {
		t.Fatalf("second PUT status = %d, want 200: %v", status, payload)
	}
	if version, _ := poolData(t, payload)["version"].(float64); version != 2 {
		t.Fatalf("second PUT version = %v, want 2", payload)
	}

	// No-op save preserves the version and reports unchanged.
	status, payload = put(2, `{"expected_version":2,"entries":[{"model_id":"a/first","enabled":true,"resolutions":["2k","1k"]}]}`, 13)
	if status != http.StatusOK {
		t.Fatalf("no-op PUT status = %d, want 200: %v", status, payload)
	}
	data = poolData(t, payload)
	if version, _ := data["version"].(float64); version != 2 {
		t.Fatalf("no-op PUT version = %v, want 2 (no bump)", data["version"])
	}
	if changed, _ := data["changed"].(bool); changed {
		t.Fatalf("no-op PUT changed = %v, want false", data["changed"])
	}

	if pools.lastService != "generation" || pools.lastExpected != 2 {
		t.Fatalf("last replace = service %q expected %d, want generation/2", pools.lastService, pools.lastExpected)
	}
}

func TestControlModelPoolReplaceConflictLeavesPoolUntouched(t *testing.T) {
	pools := newFakeControlModelPools()
	pools.seed("generation", 3, []routing.ModelPoolEntry{
		{Model: "a/first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution1K}},
	})
	h := poolHarness(t, pools)

	status, _, payload := h.serve(poolAdminCall(http.MethodPut,
		"/api/control/v1/billing/catalog/ai-image:generation/pool",
		`{"expected_version":2,"entries":[{"model_id":"b/second","enabled":true,"resolutions":["2k"]}]}`, 21))
	if status != http.StatusConflict {
		t.Fatalf("stale PUT status = %d, want 409: %v", status, payload)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeConflict) {
		t.Fatalf("code = %q, want conflict", code)
	}

	stored, err := pools.ListPool(context.Background(), "generation")
	if err != nil {
		t.Fatalf("ListPool failed: %v", err)
	}
	if stored.Version != 3 || len(stored.Entries) != 1 || stored.Entries[0].Model != "a/first" {
		t.Fatalf("stored pool after conflict = %+v, want untouched version 3 with a/first", stored)
	}
}

func TestControlModelPoolReplaceValidation(t *testing.T) {
	pools := newFakeControlModelPools()
	h := poolHarness(t, pools)

	// OpenAPI required fields are enforced at runtime as well; missing or
	// null values must not silently become the zero-value empty pool.
	for key, body := range map[int]string{
		33: `{}`,
		34: `{"expected_version":0}`,
		35: `{"entries":[]}`,
		36: `{"expected_version":null,"entries":[]}`,
		37: `{"expected_version":0,"entries":null}`,
	} {
		status, _, payload := h.serve(poolAdminCall(http.MethodPut,
			"/api/control/v1/billing/catalog/ai-image:generation/pool", body, key))
		if status != http.StatusBadRequest {
			t.Fatalf("required-field body %s status = %d, want 400: %v", body, status, payload)
		}
		if pools.replaceCalls != 0 {
			t.Fatalf("required-field body %s reached store (%d calls)", body, pools.replaceCalls)
		}
	}

	// Enabled entry without resolutions is rejected transactionally.
	status, _, payload := h.serve(poolAdminCall(http.MethodPut,
		"/api/control/v1/billing/catalog/ai-image:generation/pool",
		`{"expected_version":0,"entries":[{"model_id":"a/first","enabled":true,"resolutions":[]}]}`, 31))
	if status != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400: %v", status, payload)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
		t.Fatalf("code = %q, want bad_request", code)
	}

	// Malformed JSON is rejected before the store.
	before := pools.replaceCalls
	status, _, payload = h.serve(poolAdminCall(http.MethodPut,
		"/api/control/v1/billing/catalog/ai-image:generation/pool", `{"expected_version":`, 32))
	if status != http.StatusBadRequest {
		t.Fatalf("malformed PUT status = %d, want 400: %v", status, payload)
	}
	if pools.replaceCalls != before {
		t.Fatalf("replaceCalls = %d, want %d (rejected before the store)", pools.replaceCalls, before)
	}
}

func TestControlModelPoolAuthAndFailClosed(t *testing.T) {
	pools := newFakeControlModelPools()
	h := poolHarness(t, pools)

	reads := []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/billing/catalog/ai-image:generation/pool"},
	}
	mutations := []controlCall{
		{method: http.MethodPut, path: "/api/control/v1/billing/catalog/ai-image:generation/pool",
			body:    `{"expected_version":0,"entries":[]}`,
			headers: controlMutationHeaders("pool-auth-1", "auth reason")},
	}

	for _, call := range append(reads, mutations...) {
		status, _, payload := h.serve(call)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s %s anon status = %d, want 401", call.method, call.path, status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeUnauthorized) {
			t.Fatalf("anon code = %q, want unauthorized", code)
		}
	}

	for _, call := range append(reads, mutations...) {
		call.userID = controlUserAID
		call.role = "user"
		status, _, _ := h.serve(call)
		if status != http.StatusForbidden {
			t.Fatalf("%s %s member status = %d, want 403", call.method, call.path, status)
		}
	}

	nilHarness := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.ModelPools = nil
	})
	admin := controlCall{userID: controlOperatorID, role: "admin"}
	for _, call := range []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/billing/catalog/ai-image:generation/pool", userID: admin.userID, role: admin.role},
		{method: http.MethodPut, path: "/api/control/v1/billing/catalog/ai-image:generation/pool",
			body: `{"expected_version":0,"entries":[]}`, userID: admin.userID, role: admin.role,
			headers: controlMutationHeaders("pool-nil-1", "nil reason")},
	} {
		status, _, payload := nilHarness.serve(call)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("%s %s nil-dep status = %d, want 503", call.method, call.path, status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
			t.Fatalf("code = %q, want service_unavailable", code)
		}
	}

	var asErr *catalog.PoolVersionConflict
	if !errors.As(&catalog.PoolVersionConflict{Service: "generation", Expected: 1, Current: 2}, &asErr) {
		t.Fatal("PoolVersionConflict must match errors.As for the 409 mapping")
	}
}
