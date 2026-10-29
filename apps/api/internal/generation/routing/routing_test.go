package routing

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/contract"
)

// fakePoolStore is the in-memory PoolStore fake. The sibling catalog
// agent implements the port for real; routing behavior is verified
// here without a database.
type fakePoolStore struct {
	pools map[string]Pool
	err   error
}

func (f *fakePoolStore) ListPool(ctx context.Context, service string) (Pool, error) {
	if f.err != nil {
		return Pool{}, f.err
	}
	if err := ctx.Err(); err != nil {
		return Pool{}, err
	}
	if p, ok := f.pools[service]; ok {
		return p, nil
	}
	return Pool{Service: service, Version: 1}, nil
}

func testPool() Pool {
	return Pool{
		Service: "generation",
		Version: 7,
		Entries: []ModelPoolEntry{
			{Provider: "prov-a", Model: "model-a", Enabled: true, SupportedResolutions: []Resolution{Resolution1K, Resolution2K, Resolution4K}},
			{Provider: "prov-b", Model: "model-b", Enabled: true, SupportedResolutions: []Resolution{Resolution2K, Resolution4K}},
			{Provider: "prov-c", Model: "model-c", Enabled: false, SupportedResolutions: []Resolution{Resolution1K}},
		},
	}
}

func resolveWithSeed(t *testing.T, store PoolStore, seed SelectionSeed, service, resolution string) RoutingSnapshot {
	t.Helper()
	r := NewRoutingResolver(store, WithSeedGenerator(func() (SelectionSeed, error) { return seed, nil }))
	snap, err := r.Resolve(context.Background(), service, resolution)
	if err != nil {
		t.Fatalf("Resolve(%q, %q) err = %v", service, resolution, err)
	}
	return snap
}

func TestNormalizeResolution(t *testing.T) {
	cases := map[string]Resolution{
		"1k": Resolution1K, "1K": Resolution1K, " 2k ": Resolution2K,
		"2K": Resolution2K, "4k": Resolution4K, "4K": Resolution4K,
	}
	for raw, want := range cases {
		got, err := NormalizeResolution(raw)
		if err != nil || got != want {
			t.Errorf("NormalizeResolution(%q) = %q, %v; want %q, nil", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "  ", "8k", "1.5k", "1 k", "all"} {
		if res, err := NormalizeResolution(raw); err == nil {
			t.Errorf("NormalizeResolution(%q) = %q, nil; want invalid input error", raw, res)
		} else if _, ok := err.(*InvalidRoutingInputError); !ok {
			t.Errorf("NormalizeResolution(%q) err type = %T; want *InvalidRoutingInputError", raw, err)
		}
	}
}

func TestResolveFailsClosedWhenStoreIsMissing(t *testing.T) {
	_, err := (*RoutingResolver)(nil).Resolve(context.Background(), "generation", "1k")
	if !errors.Is(err, ErrPoolStoreUnavailable) {
		t.Fatalf("Resolve nil resolver err = %v, want ErrPoolStoreUnavailable", err)
	}
	_, err = NewRoutingResolver(nil).Resolve(context.Background(), "generation", "1k")
	if !errors.Is(err, ErrPoolStoreUnavailable) {
		t.Fatalf("Resolve nil store err = %v, want ErrPoolStoreUnavailable", err)
	}
}

func TestResolveFiltersByResolution(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	snap := resolveWithSeed(t, store, SelectionSeed{1}, "generation", "2k")
	if len(snap.EligibleModels) != 2 ||
		snap.EligibleModels[0] != "prov-a/model-a" ||
		snap.EligibleModels[1] != "prov-b/model-b" {
		t.Fatalf("eligible = %v; want [prov-a/model-a prov-b/model-b]", snap.EligibleModels)
	}
	if snap.Service != "generation" || snap.RequestedResolution != Resolution2K || snap.PoolVersion != 7 {
		t.Fatalf("snapshot pinning = %+v; want service=generation resolution=2k version=7", snap)
	}
}

func TestResolveExcludesDisabled(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	snap := resolveWithSeed(t, store, SelectionSeed{1}, "generation", "1k")
	if len(snap.EligibleModels) != 1 || snap.EligibleModels[0] != "prov-a/model-a" {
		t.Fatalf("eligible = %v; disabled prov-c/model-c must be invisible", snap.EligibleModels)
	}
	if snap.RetryCandidate != "" {
		t.Fatalf("single-candidate retry = %q; want empty", snap.RetryCandidate)
	}
}

func TestResolveRejectsDefault(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	if _, err := NewRoutingResolver(store).Resolve(context.Background(), "generation", "default"); err == nil {
		t.Fatal("Resolve(default) = nil; want invalid routing input")
	}
}

func TestResolveEmptyPoolSentinel(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{}}
	_, err := NewRoutingResolver(store).Resolve(context.Background(), " Generation ", "2K")
	if !errors.Is(err, ErrServiceResolutionUnavailable) {
		t.Fatalf("err = %v; want errors.Is ErrServiceResolutionUnavailable", err)
	}
	var unavailable *ResolutionUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err type = %T; want *ResolutionUnavailableError", err)
	}
	if unavailable.Service != "generation" || unavailable.Resolution != Resolution2K {
		t.Fatalf("sentinel carries service=%q resolution=%q; want normalized generation/2k",
			unavailable.Service, unavailable.Resolution)
	}
	// The kernel registry entry this sentinel maps to at the wave-2
	// edge must stay 422.
	if got := contract.StatusOf(contract.CodeServiceResolutionUnavailable); got != 422 {
		t.Fatalf("StatusOf(service_resolution_unavailable) = %d; want 422", got)
	}
	if string(contract.CodeServiceResolutionUnavailable) != "service_resolution_unavailable" {
		t.Fatalf("registry code = %q; wire value must not drift", string(contract.CodeServiceResolutionUnavailable))
	}
}

func TestResolveNoEligibleForResolution(t *testing.T) {
	pool := Pool{Service: "upscale", Version: 3, Entries: []ModelPoolEntry{
		{Provider: "prov-a", Model: "model-a", Enabled: true, SupportedResolutions: []Resolution{Resolution1K}},
		{Provider: "prov-x", Model: "model-x", Enabled: false, SupportedResolutions: []Resolution{Resolution4K}},
	}}
	store := &fakePoolStore{pools: map[string]Pool{"upscale": pool}}
	_, err := NewRoutingResolver(store).Resolve(context.Background(), "upscale", "4k")
	if !errors.Is(err, ErrServiceResolutionUnavailable) {
		t.Fatalf("err = %v; want the availability sentinel, not an input/store error", err)
	}
}

func TestResolveInvalidInputIsNotSentinel(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	r := NewRoutingResolver(store)
	for _, tc := range [][2]string{{"", "2k"}, {"   ", "2k"}, {"generation", ""}, {"generation", "8k"}} {
		_, err := r.Resolve(context.Background(), tc[0], tc[1])
		if err == nil {
			t.Fatalf("Resolve(%q, %q) = nil; want input error", tc[0], tc[1])
		}
		if errors.Is(err, ErrServiceResolutionUnavailable) {
			t.Fatalf("Resolve(%q, %q) matched the availability sentinel; want input error", tc[0], tc[1])
		}
		var inputErr *InvalidRoutingInputError
		if !errors.As(err, &inputErr) {
			t.Fatalf("Resolve(%q, %q) err type = %T; want *InvalidRoutingInputError", tc[0], tc[1], err)
		}
	}
}

func TestResolveStoreErrorPropagatesUnclassified(t *testing.T) {
	sentinel := errors.New("db down")
	store := &fakePoolStore{err: sentinel}
	_, err := NewRoutingResolver(store).Resolve(context.Background(), "generation", "2k")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v; store failures must propagate, not convert to the sentinel", err)
	}
	if errors.Is(err, ErrServiceResolutionUnavailable) {
		t.Fatalf("store outage matched the availability sentinel; outages are not empty pools")
	}
}

func TestSelectionDeterministicAndReplayable(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	seed := SelectionSeed{9, 8, 7}
	first := resolveWithSeed(t, store, seed, "generation", "2k")
	second := resolveWithSeed(t, store, seed, "generation", "2k")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same seed gave different snapshots:\n%+v\n%+v", first, second)
	}
	if err := first.Verify(); err != nil {
		t.Fatalf("Verify() = %v; snapshot must replay from its own fields", err)
	}
	tampered := first
	tampered.SelectedModel = tampered.RetryCandidate
	tampered.RetryCandidate = first.SelectedModel
	if err := tampered.Verify(); err == nil {
		t.Fatal("Verify() = nil for swapped selection; want a mismatch error")
	}
	unsorted := first
	unsorted.EligibleModels = []string{"prov-b/model-b", "prov-a/model-a"}
	if err := unsorted.Verify(); err == nil {
		t.Fatal("Verify() = nil for unsorted eligible list; want an error")
	}
}

func TestSelectionSchemeMatchesContract(t *testing.T) {
	// Byte-exact ADR-0012 rule 5 scheme, recomputed independently here:
	// SHA-256(seed || U64BE(version) || 0x00 || service || 0x00 || resolution) mod n.
	var seed SelectionSeed
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	eligible := []string{"prov-b/model-b", "prov-a/model-a", "prov-c/model-c"}
	selected, retry := SelectCandidate(seed, 7, "generation", Resolution2K, eligible)

	h := sha256.New()
	h.Write(seed[:])
	var vb [8]byte
	binary.BigEndian.PutUint64(vb[:], 7)
	h.Write(vb[:])
	h.Write([]byte{0x00})
	h.Write([]byte("generation"))
	h.Write([]byte{0x00})
	h.Write([]byte("2k"))
	sum := h.Sum(nil)
	sorted := []string{"prov-a/model-a", "prov-b/model-b", "prov-c/model-c"}
	wantIdx := 0
	for _, b := range sum {
		wantIdx = (wantIdx*256 + int(b)) % len(sorted)
	}
	if selected != sorted[wantIdx] {
		t.Fatalf("selected = %q; contract scheme gives %q", selected, sorted[wantIdx])
	}
	wantRetry := sorted[(wantIdx+1)%len(sorted)]
	if retry != wantRetry {
		t.Fatalf("retry = %q; want next sorted candidate %q", retry, wantRetry)
	}
}

func TestSelectionCoversAllCandidates(t *testing.T) {
	eligible := []string{"prov-a/model-a", "prov-b/model-b", "prov-c/model-c"}
	seen := map[string]int{}
	for i := 0; i < 300; i++ {
		sum := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
		var seed SelectionSeed
		copy(seed[:], sum[:])
		selected, retry := SelectCandidate(seed, 1, "generation", Resolution2K, eligible)
		seen[selected]++
		if retry == "" || retry == selected {
			t.Fatalf("seed %d: selected=%q retry=%q; multi-candidate retry must be a distinct candidate", i, selected, retry)
		}
	}
	for _, m := range eligible {
		if seen[m] < 60 || seen[m] > 140 {
			t.Fatalf("selection counts = %v; want roughly uniform over 300 seeds", seen)
		}
	}
}

func TestRetryCandidateWrapsFromLastToFirst(t *testing.T) {
	eligible := []string{"prov-a/model-a", "prov-b/model-b"}
	found := false
	for i := 0; i < 200 && !found; i++ {
		sum := sha256.Sum256([]byte{byte(i), 0xA5})
		var seed SelectionSeed
		copy(seed[:], sum[:])
		selected, retry := SelectCandidate(seed, 1, "generation", Resolution2K, eligible)
		if selected == "prov-b/model-b" {
			found = true
			if retry != "prov-a/model-a" {
				t.Fatalf("selected last %q but retry = %q; want wrap to first", selected, retry)
			}
		}
	}
	if !found {
		t.Fatal("no seed in 200 selected the last candidate; selection looks skewed")
	}
}

func TestNewSelectionSeedUnique(t *testing.T) {
	a, err := NewSelectionSeed()
	if err != nil {
		t.Fatalf("NewSelectionSeed() err = %v", err)
	}
	b, err := NewSelectionSeed()
	if err != nil {
		t.Fatalf("NewSelectionSeed() err = %v", err)
	}
	if a == b {
		t.Fatal("two seeds identical; randomness failure")
	}
}

func TestSnapshotPinningSurvivesPoolEdits(t *testing.T) {
	pool := testPool()
	store := &fakePoolStore{pools: map[string]Pool{"generation": pool}}
	snap := resolveWithSeed(t, store, SelectionSeed{5}, "generation", "2k")
	pinned := snap

	// Later admin edits: disable the selected model, bump the version.
	edited := testPool()
	edited.Version = 8
	for i := range edited.Entries {
		if edited.Entries[i].DisplayModel() == snap.SelectedModel {
			edited.Entries[i].Enabled = false
		}
	}
	store.pools["generation"] = edited

	if !reflect.DeepEqual(snap, pinned) {
		t.Fatal("accepted snapshot mutated by the pool edit")
	}
	if err := snap.Verify(); err != nil {
		t.Fatalf("Verify() after pool edit = %v; replay must not need the live pool", err)
	}
	if !snap.HasRetryCandidate() && len(snap.EligibleModels) > 1 {
		t.Fatal("HasRetryCandidate() = false with multiple eligible models")
	}
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	store := &fakePoolStore{pools: map[string]Pool{"generation": testPool()}}
	snap := resolveWithSeed(t, store, SelectionSeed{0xAB, 0xCD}, "generation", "4k")

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("Marshal err = %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map err = %v", err)
	}
	for _, field := range []string{"service", "requested_resolution", "eligible_models", "selected_model", "retry_candidate", "selection_seed", "pool_version"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("snapshot JSON missing field %q: %s", field, data)
		}
	}
	if len(raw) != 7 {
		t.Errorf("snapshot JSON has %d fields; contract fixes exactly 7: %s", len(raw), data)
	}
	var seedRaw string
	if err := json.Unmarshal(raw["selection_seed"], &seedRaw); err != nil {
		t.Fatalf("selection_seed not a JSON string: %s", data)
	}
	if len(seedRaw) != 32 || strings.Trim(seedRaw, "0123456789abcdef") != "" {
		t.Errorf("selection_seed = %q; want 32-char lowercase hex", seedRaw)
	}

	var decoded RoutingSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip Unmarshal err = %v", err)
	}
	if !reflect.DeepEqual(decoded, snap) {
		t.Fatalf("round trip mismatch:\n%+v\n%+v", decoded, snap)
	}
	if err := decoded.Verify(); err != nil {
		t.Fatalf("Verify() after JSON round trip = %v", err)
	}
	var bad RoutingSnapshot
	if err := json.Unmarshal([]byte(`{"selection_seed":"xyz"}`), &bad); err == nil {
		t.Error("corrupt selection_seed decoded without error; want fail-closed")
	}
}

func TestRetryClassification(t *testing.T) {
	retryable := []AttemptFailure{
		{HTTPStatus: 429}, {HTTPStatus: 408}, {HTTPStatus: 500},
		{HTTPStatus: 502}, {HTTPStatus: 503}, {HTTPStatus: 599},
		{Timeout: true}, {NetworkError: true},
		{HTTPStatus: 503, Timeout: true},
	}
	for i, f := range retryable {
		if !f.Retryable() {
			t.Errorf("retryable[%d] (%+v) = false; want true", i, f)
		}
	}
	deterministic := []AttemptFailure{
		{HTTPStatus: 400}, {HTTPStatus: 401}, {HTTPStatus: 402}, {HTTPStatus: 403},
		{HTTPStatus: 404}, {HTTPStatus: 422}, {HTTPStatus: 200},
		{HTTPStatus: 0}, {HTTPStatus: 301}, {HTTPStatus: 600}, {HTTPStatus: -1},
		{Timeout: true, ContentSafetyRejected: true},
		{HTTPStatus: 500, InvalidParams: true},
		{HTTPStatus: 503, InsufficientCredits: true},
		{HTTPStatus: 429, AuthFailed: true},
		{HTTPStatus: 500, NotFound: true},
		{HTTPStatus: 200, UnusablePayload: true},
		{UnusablePayload: true},
		{NetworkError: true, ContentSafetyRejected: true},
		{InvalidParams: true}, {InsufficientCredits: true}, {ContentSafetyRejected: true},
		{AuthFailed: true}, {NotFound: true},
	}
	for i, f := range deterministic {
		if f.Retryable() {
			t.Errorf("deterministic[%d] (%+v) = true; business errors never trigger fallback", i, f)
		}
		if i >= 11 && i <= 22 && !f.Deterministic() {
			t.Errorf("deterministic[%d] (%+v) Deterministic() = false; marker must be recognized", i, f)
		}
	}
	for _, status := range []int{408, 429, 500, 503} {
		if !RetryableHTTPStatus(status) {
			t.Errorf("RetryableHTTPStatus(%d) = false; want true", status)
		}
	}
	for _, status := range []int{0, 200, 400, 401, 402, 403, 404, 422} {
		if RetryableHTTPStatus(status) {
			t.Errorf("RetryableHTTPStatus(%d) = true; want false", status)
		}
	}
}
