package workspacetasks

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/generation/routing"

	"github.com/jackc/pgx/v5"
)

type fakeQuoter struct {
	snapshot BillingSnapshot
	err      error
	calls    int
}

func (f *fakeQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (BillingSnapshot, error) {
	f.calls++
	if f.err != nil {
		return BillingSnapshot{}, f.err
	}
	out := f.snapshot
	out.SKU = "ai-image:" + kind
	if out.Quantity == 0 {
		out.Quantity = quantity
	}
	if out.TotalCredits == 0 {
		out.TotalCredits = out.UnitCredits * out.Quantity
	}
	return out, nil
}

type fakeHolder struct {
	holds    map[string]int64
	captured map[string]int64
	released map[string]int64
	holdErr  error
}

func newFakeHolder() *fakeHolder {
	return &fakeHolder{holds: map[string]int64{}, captured: map[string]int64{}, released: map[string]int64{}}
}

// allowTestPoolResolver serves every service/resolution from one enabled
// entry (#854: replaces the legacy allowTestRouting gate; the resolver is
// the only availability seam).
func allowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{1, 2, 3}
	return routing.NewRoutingResolver(allowAllPoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllPoolStore struct{}

func (allowAllPoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Model: "openrouter/test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (f *fakeHolder) Hold(ctx context.Context, userID int64, amount int64, key, taskID string) (string, error) {
	if f.holdErr != nil {
		return "", f.holdErr
	}
	if amount <= 0 {
		return "", nil
	}
	holdID := "hold_" + taskID
	f.holds[holdID] = amount
	return holdID, nil
}

func (f *fakeHolder) Capture(ctx context.Context, holdID string, amount int64, taskID string) error {
	f.captured[holdID] = amount
	return nil
}

func (f *fakeHolder) Release(ctx context.Context, holdID string, amount int64, taskID string) error {
	f.released[holdID] = amount
	return nil
}

type fakeEnqueuer struct {
	enqueued []*WorkspaceTask
	failNext error
	calls    int
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, task *WorkspaceTask) error {
	f.calls++
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.enqueued = append(f.enqueued, task)
	return nil
}

type fakeAssets struct {
	assets map[string]*AssetInfo
	err    error
}

func (f *fakeAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*AssetInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	info, ok := f.assets[workspaceID+"\x00"+assetID]
	if !ok {
		return nil, ErrMediaNotFound
	}
	return info, nil
}

func testSnapshot() BillingSnapshot {
	return BillingSnapshot{
		SKU: "ai-image:render", DisplayName: "Render", PriceVersionID: 1, PriceVersion: 1,
		OutputResolution: "default", UnitCredits: 10, Quantity: 1, TotalCredits: 10,
		SettlementState: "held",
	}
}

func TestValidateCreateRejectsUnknownKind(t *testing.T) {
	if _, err := ValidateCreate("nope", nil, nil); !IsValidationError(err) {
		t.Fatalf("kind nope err = %v, want validation", err)
	}
	for _, kind := range []string{"render", "variations", "inpaint", "cutout", "fusion", "upscale", "outpaint"} {
		if _, err := ValidateCreate(kind, nil, nil); err != nil {
			t.Fatalf("kind %s err = %v, want nil", kind, err)
		}
	}
}

func TestValidateCreateRejectsRawShapeAndURLs(t *testing.T) {
	if _, err := ValidateCreate("render", []string{"https://example.com/a.png"}, nil); !IsValidationError(err) {
		t.Fatalf("raw url err = %v, want validation", err)
	}
	if _, err := ValidateCreate("render", []string{"asset_abc123"}, nil); err != nil {
		t.Fatalf("asset id err = %v, want nil", err)
	}
	params := map[string]any{"image_urls": []any{"https://example.com/a.png"}}
	if _, err := ValidateCreate("render", nil, params); !IsValidationError(err) {
		t.Fatalf("params url err = %v, want validation", err)
	}
	params = map[string]any{"mystery_field": "x"}
	if _, err := ValidateCreate("render", nil, params); !IsValidationError(err) {
		t.Fatalf("unknown field err = %v, want validation", err)
	}
}

func TestCreateAtomicityRollbackOnEnqueueFailure(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	queue := &fakeEnqueuer{failNext: errors.New("river down")}
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(),
	})
	_, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "k1")
	if err == nil {
		t.Fatal("create with failing enqueue must fail")
	}
	count, _ := store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0 (insert rolled back)", count)
	}
	if len(holder.holds) != 0 && len(holder.released) == 0 {
		t.Fatalf("hold stranded: holds=%v released=%v", holder.holds, holder.released)
	}
	if len(service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed settlement")
	}
}

func TestCreateAtomicityRollbackOnHoldFailure(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	holder.holdErr = ErrInsufficientCredits
	queue := &fakeEnqueuer{}
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(),
	})
	_, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "k1")
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("err = %v, want insufficient", err)
	}
	count, _ := store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 (never enqueued without hold)", queue.calls)
	}
}

func TestCancelTerminalSettleReleases(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(),
	})
	created, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	_ = service.StagedEvents()
	cancelled, already, err := service.Cancel(ctx, "ws_abc", created.ID, 7, "user cancel")
	if err != nil || already {
		t.Fatalf("cancel = %v %v %v", cancelled, already, err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}
	if len(holder.released) == 0 {
		t.Fatal("cancel must Release the hold atomically with the state flip")
	}
	// Second cancel replays idempotently.
	replay, already, err := service.Cancel(ctx, "ws_abc", created.ID, 7, "again")
	if err != nil || !already || replay.Status != StatusCancelled {
		t.Fatalf("replay = %v %v %v", replay, already, err)
	}
}

func TestRetryBudgetBounded(t *testing.T) {
	if !ShouldRetry(0, true) || !ShouldRetry(1, true) {
		t.Fatal("retry 0/1 must allow retry")
	}
	if ShouldRetry(2, true) {
		t.Fatal("retry 2 must exhaust the budget (max 2)")
	}
	if ShouldRetry(0, false) {
		t.Fatal("deterministic failures must go terminal directly")
	}
	if IsRetryableProviderError(429, nil) != true {
		t.Fatal("429 must be retryable")
	}
	if IsRetryableProviderError(500, nil) != true {
		t.Fatal("5xx must be retryable")
	}
	if IsRetryableProviderError(400, nil) != false {
		t.Fatal("400 must be terminal")
	}
	if IsRetryableProviderError(0, errors.New("timeout")) != true {
		t.Fatal("timeout must be retryable")
	}
}

func TestOutputRefsValidation(t *testing.T) {
	if err := ValidateOutputRef(OutputRef{Kind: OutputKindAsset, AssetID: "asset_abc123"}); err != nil {
		t.Fatalf("asset ref err = %v", err)
	}
	if err := ValidateOutputRef(OutputRef{Kind: OutputKindAsset, AssetID: "https://x/y.png"}); !IsValidationError(err) {
		t.Fatalf("raw url asset err = %v, want validation", err)
	}
	checksum := strings.Repeat("a", 64)
	if err := ValidateOutputRef(OutputRef{Kind: OutputKindRemote, URL: "https://cdn.example.com/o.png", ChecksumSHA256: checksum}); err != nil {
		t.Fatalf("remote ref err = %v", err)
	}
	if err := ValidateOutputRef(OutputRef{Kind: OutputKindRemote, URL: "https://internal/ws-uploads/x", ChecksumSHA256: checksum}); !IsValidationError(err) {
		t.Fatalf("storage internals err = %v, want validation", err)
	}
}

func TestInputAssetsRequireReady(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	assets := &fakeAssets{assets: map[string]*AssetInfo{
		"ws_abc\x00asset_ready":   {ID: "asset_ready", WorkspaceID: "ws_abc", Status: "ready"},
		"ws_abc\x00asset_pending": {ID: "asset_pending", WorkspaceID: "ws_abc", Status: "pending"},
	}}
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: newFakeHolder(), Queue: &fakeEnqueuer{}, Assets: assets, Resolver: allowTestPoolResolver(),
	})
	if _, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", MediaAssetIDs: []string{"asset_ready"}}, "k1"); err != nil {
		t.Fatalf("ready input err = %v", err)
	}
	if _, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", MediaAssetIDs: []string{"asset_pending"}}, "k2"); !errors.Is(err, ErrMediaNotReady) {
		t.Fatalf("pending input err = %v, want media_not_ready", err)
	}
	if _, err := service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", MediaAssetIDs: []string{"asset_missing"}}, "k3"); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("missing input err = %v, want media_not_found", err)
	}
}

// Submit-path routing tests (#848): quote and submit consume the same
// RoutingResolver, an empty eligible set reserves nothing, and the pinned
// snapshot survives later pool edits.

type stubPoolStore struct {
	pools map[string]routing.Pool
	err   error
}

func (s *stubPoolStore) ListPool(ctx context.Context, service string) (routing.Pool, error) {
	if s.err != nil {
		return routing.Pool{}, s.err
	}
	if err := ctx.Err(); err != nil {
		return routing.Pool{}, err
	}
	if p, ok := s.pools[service]; ok {
		return p, nil
	}
	return routing.Pool{Service: service}, nil
}

func routingPool(version uint64, entries ...routing.ModelPoolEntry) routing.Pool {
	return routing.Pool{Service: "render", Version: version, Entries: entries}
}

func routingEntry(model string, enabled bool, resolutions ...routing.Resolution) routing.ModelPoolEntry {
	return routing.ModelPoolEntry{Model: model, Enabled: enabled, SupportedResolutions: resolutions}
}

type routingTestEnv struct {
	store     *InMemoryStore
	holder    *fakeHolder
	queue     *fakeEnqueuer
	quoter    *fakeQuoter
	service   *Service
	saved     map[string]routing.RoutingSnapshot
	saveErr   error
	saveCalls int
}

func newRoutingTestEnv(pools map[string]routing.Pool) *routingTestEnv {
	env := &routingTestEnv{
		store:  NewInMemoryStoreForTests(),
		holder: newFakeHolder(),
		queue:  &fakeEnqueuer{},
		quoter: &fakeQuoter{snapshot: testSnapshot()},
		saved:  make(map[string]routing.RoutingSnapshot),
	}
	seed := routing.SelectionSeed{7, 7, 7}
	resolver := routing.NewRoutingResolver(&stubPoolStore{pools: pools},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
	env.service = NewTestServiceForTests(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.store, Quoter: env.quoter, Holder: env.holder, Queue: env.queue,
		Resolver: resolver,
		SnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			env.saveCalls++
			if env.saveErr != nil {
				return env.saveErr
			}
			env.saved[taskID] = snap
			return nil
		},
	})
	return env
}

func TestResolverUnavailableReservesNothing(t *testing.T) {
	ctx := context.Background()
	env := newRoutingTestEnv(map[string]routing.Pool{})

	_, quoteErr := env.service.Quote(ctx, QuoteParams{Service: "render", Resolution: "2k"})
	if !errors.Is(quoteErr, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Quote err = %v, want ErrServiceResolutionUnavailable", quoteErr)
	}
	var unavailable *routing.ResolutionUnavailableError
	if !errors.As(quoteErr, &unavailable) {
		t.Fatalf("Quote err type = %T, want *ResolutionUnavailableError", quoteErr)
	}
	if unavailable.Service != "render" || unavailable.Resolution != routing.Resolution2K {
		t.Fatalf("Quote fields = %q/%q, want normalized render/2k", unavailable.Service, unavailable.Resolution)
	}

	_, createErr := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "2K",
	}, "routing-unavail-1")
	if !errors.Is(createErr, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err = %v, want ErrServiceResolutionUnavailable", createErr)
	}
	if !errors.As(createErr, &unavailable) || unavailable.Service != "render" || unavailable.Resolution != routing.Resolution2K {
		t.Fatalf("Create fields = %+v, want normalized render/2k", unavailable)
	}

	count, _ := env.store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if len(env.holder.holds) != 0 || len(env.holder.released) != 0 {
		t.Fatalf("unavailable resolution must not touch credits: holds=%v released=%v", env.holder.holds, env.holder.released)
	}
	if env.queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0", env.queue.calls)
	}
	if env.quoter.calls != 0 {
		t.Fatalf("quote calls = %d, want 0 (availability precedes quoting)", env.quoter.calls)
	}
	if env.saveCalls != 0 || len(env.saved) != 0 {
		t.Fatalf("snapshot saves = %d, want 0 (no task to pin)", env.saveCalls)
	}
}

func TestResolverPinsSnapshotAndSurvivesPoolEdits(t *testing.T) {
	ctx := context.Background()
	pools := map[string]routing.Pool{"render": routingPool(3,
		routingEntry("a/first", true, routing.Resolution1K, routing.Resolution2K),
		routingEntry("b/second", true, routing.Resolution2K, routing.Resolution4K),
		routingEntry("c/off", false, routing.Resolution2K),
	)}
	env := newRoutingTestEnv(pools)

	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "2k",
	}, "routing-pin-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	snap, ok := env.saved[created.ID]
	if !ok {
		t.Fatalf("no snapshot pinned for task %s", created.ID)
	}
	if err := snap.Verify(); err != nil {
		t.Fatalf("pinned snapshot must replay without the live pool: %v", err)
	}
	if snap.Service != "render" || snap.RequestedResolution != routing.Resolution2K || snap.PoolVersion != 3 {
		t.Fatalf("pinned pinning = %+v, want render/2k/version 3", snap)
	}
	if len(snap.EligibleModels) != 2 || snap.EligibleModels[0] != "a/first" || snap.EligibleModels[1] != "b/second" {
		t.Fatalf("eligible = %v, want sorted [a/first b/second] (disabled c/off invisible)", snap.EligibleModels)
	}
	if !snap.HasRetryCandidate() || snap.RetryCandidate == snap.SelectedModel {
		t.Fatalf("retry = %q selected = %q, want a distinct bounded candidate", snap.RetryCandidate, snap.SelectedModel)
	}
	pinned := snap

	// Later Admin pool edits (disable the selected model, bump the version)
	// never invalidate the accepted task nor change its candidate set.
	edited := pools["render"]
	edited.Version = 4
	for i := range edited.Entries {
		if edited.Entries[i].Model == snap.SelectedModel {
			edited.Entries[i].Enabled = false
		}
	}
	pools["render"] = edited

	if !reflect.DeepEqual(env.saved[created.ID], pinned) {
		t.Fatal("accepted snapshot mutated by the pool edit")
	}
	if err := env.saved[created.ID].Verify(); err != nil {
		t.Fatalf("Verify() after pool edit = %v; replay must not need the live pool", err)
	}
}

func TestResolverResolutionForFixedServices(t *testing.T) {
	ctx := context.Background()
	env := newRoutingTestEnv(map[string]routing.Pool{"cutout": routingPool(1,
		routingEntry("a/first", true, routing.Resolution1K),
	)})

	// Empty resolution canonicalizes to 1k. The pool helper pins Service
	// "render" on the fact, but the stub serves entries by the requested
	// key, so the cutout decision below reads the cutout entries.
	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "cutout",
	}, "routing-default-1")
	if err != nil {
		t.Fatalf("Create with empty resolution: %v", err)
	}
	snap := env.saved[created.ID]
	if snap.Service != "cutout" {
		t.Fatalf("pinned service = %q, want cutout", snap.Service)
	}
	if snap.RequestedResolution != routing.Resolution1K {
		t.Fatalf("requested = %q, want 1k", snap.RequestedResolution)
	}
	if len(snap.EligibleModels) != 1 || snap.EligibleModels[0] != "a/first" {
		t.Fatalf("eligible = %v, want [a/first]", snap.EligibleModels)
	}
}

func TestResolverEmptyResolutionUsesScalableDefault(t *testing.T) {
	ctx := context.Background()
	env := newRoutingTestEnv(map[string]routing.Pool{"render": routingPool(1,
		routingEntry("a/first", true, routing.Resolution1K),
	)})

	// #851: scalable Services converge with the readiness plan default
	// (1k), never the match-all "default".
	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "routing-scalable-default-1")
	if err != nil {
		t.Fatalf("Create with empty resolution: %v", err)
	}
	snap := env.saved[created.ID]
	if snap.RequestedResolution != routing.Resolution1K {
		t.Fatalf("requested = %q, want 1k", snap.RequestedResolution)
	}
}

func TestResolverInvalidInputIsValidationNotAvailability(t *testing.T) {
	ctx := context.Background()
	env := newRoutingTestEnv(map[string]routing.Pool{"render": routingPool(1,
		routingEntry("a/first", true, routing.Resolution1K),
	)})

	_, quoteErr := env.service.Quote(ctx, QuoteParams{Service: "render", Resolution: "8k"})
	if !IsValidationError(quoteErr) {
		t.Fatalf("Quote err = %T %v, want *ValidationError", quoteErr, quoteErr)
	}
	if errors.Is(quoteErr, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Quote err matched the availability sentinel; garbage input is a caller bug")
	}
	validationErr, _ := quoteErr.(*ValidationError)
	if validationErr.Fields["resolution"] == "" {
		t.Fatalf("Quote fields = %v, want a resolution entry", validationErr.Fields)
	}

	_, createErr := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "8k",
	}, "routing-invalid-1")
	if !IsValidationError(createErr) {
		t.Fatalf("Create err = %T %v, want *ValidationError", createErr, createErr)
	}
	count, _ := env.store.CountTasks(ctx, "ws_abc")
	if count != 0 || len(env.holder.holds) != 0 || env.queue.calls != 0 {
		t.Fatalf("invalid input reserved work: tasks=%d holds=%v queued=%d", count, env.holder.holds, env.queue.calls)
	}
}

func TestResolverSnapshotSaveFailureCompensates(t *testing.T) {
	ctx := context.Background()
	env := newRoutingTestEnv(map[string]routing.Pool{"render": routingPool(1,
		routingEntry("a/first", true, routing.Resolution2K),
	)})
	env.saveErr = errors.New("snapshot store down")

	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "2k",
	}, "routing-savefail-1")
	if err == nil || !strings.Contains(err.Error(), "pin routing snapshot") {
		t.Fatalf("Create err = %v, want snapshot pin failure", err)
	}
	count, _ := env.store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0 (insert rolled back)", count)
	}
	if len(env.holder.released) == 0 {
		t.Fatal("hold must be released when snapshot pinning fails (no stranded hold)")
	}
	if env.queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 (never enqueued without snapshot)", env.queue.calls)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed settlement")
	}
}
