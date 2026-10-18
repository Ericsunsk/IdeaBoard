package workspacetasks

import (
	"context"
	"errors"
	"ideaboard/api/internal/archtest"
	"ideaboard/api/internal/generation/routing"
	"reflect"
	"strings"
	"testing"

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

// QuoteInTx serves the settlement-transaction seam from the same snapshot:
// quoting has no durable side effects, so no rollback hook is needed.
func (f *fakeQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, kind, resolution string, quantity int64) (BillingSnapshot, error) {
	return f.Quote(ctx, kind, resolution, quantity)
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

type allowAllPoolStore struct{}

func (allowAllPoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Model: "openrouter/test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (allowAllPoolStore) ListPoolInTx(_ context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllPoolStore{}.ListPool(context.Background(), service)
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

// HoldInTx/CaptureInTx/ReleaseInTx serve the settlement-transaction seams
// from the same ledger maps. Rollback fidelity lives in the txTestHolder
// wrapper (settle_tx_fake_test.go), which regulators of atomicity use.
func (f *fakeHolder) HoldInTx(ctx context.Context, _ pgx.Tx, userID int64, amount int64, key, taskID string) (string, error) {
	return f.Hold(ctx, userID, amount, key, taskID)
}

func (f *fakeHolder) CaptureInTx(ctx context.Context, _ pgx.Tx, _ int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Capture(ctx, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
}

func (f *fakeHolder) ReleaseInTx(ctx context.Context, _ pgx.Tx, _ int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Release(ctx, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
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

// EnqueueInTx serves the settlement-transaction seam from the same queue.
// Rollback fidelity lives in the txTestEnqueuer wrapper.
func (f *fakeEnqueuer) EnqueueInTx(ctx context.Context, _ pgx.Tx, task *WorkspaceTask) error {
	return f.Enqueue(ctx, task)
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
	// Seam contract (P0-4): the reader owns the readiness comparison and
	// reports it through sentinels, mirroring media GateAsset. The submit
	// gate consumes only these sentinels.
	if info.Archived {
		return nil, ErrMediaArchived
	}
	if strings.TrimSpace(info.Status) != "ready" {
		return nil, ErrMediaNotReady
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

// noopSnapshotSaver satisfies the atomic settlement requirement without
// pinning anything: suites that assert pinning pass their own saver.
func noopSnapshotSaver(context.Context, pgx.Tx, string, routing.RoutingSnapshot) error {
	return nil
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
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	holder := newFakeHolder()
	queue := &fakeEnqueuer{failNext: errors.New("river down")}
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: &txTestHolder{fakeHolder: holder}, Queue: &txTestEnqueuer{fakeEnqueuer: queue},
		Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{},
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
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	holder := newFakeHolder()
	holder.holdErr = ErrInsufficientCredits
	queue := &fakeEnqueuer{}
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: &txTestHolder{fakeHolder: holder}, Queue: &txTestEnqueuer{fakeEnqueuer: queue},
		Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{},
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
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: &txTestHolder{fakeHolder: holder}, Queue: &txTestEnqueuer{fakeEnqueuer: queue},
		Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{},
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
	// Single retry taxonomy (P0-4): routing.ClassifyError maps statuses and
	// timeout/network markers onto AttemptFailure; Retryable decides.
	if !routing.ClassifyFailure(429, "").Retryable() {
		t.Fatal("429 must be retryable")
	}
	if !routing.ClassifyFailure(500, "").Retryable() {
		t.Fatal("5xx must be retryable")
	}
	if routing.ClassifyFailure(400, "").Retryable() {
		t.Fatal("400 must be terminal")
	}
	if !routing.ClassifyError(errors.New("timeout")).Retryable() {
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
	if err := ValidateOutputRef(OutputRef{Kind: OutputKindRemote, URL: "https://internal/uploads/ws/abc/def/x.png", ChecksumSHA256: checksum}); !IsValidationError(err) {
		t.Fatalf("canonical slot internals err = %v, want validation", err)
	}
}

func TestInputAssetsRequireReady(t *testing.T) {
	ctx := context.Background()
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	assets := &fakeAssets{assets: map[string]*AssetInfo{
		"ws_abc\x00asset_ready":   {ID: "asset_ready", WorkspaceID: "ws_abc", Status: "ready"},
		"ws_abc\x00asset_pending": {ID: "asset_pending", WorkspaceID: "ws_abc", Status: "pending"},
	}}
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: &txTestHolder{fakeHolder: newFakeHolder()}, Queue: &txTestEnqueuer{fakeEnqueuer: &fakeEnqueuer{}},
		Assets: assets, Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{assets: assets},
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

func (s *stubPoolStore) ListPoolInTx(ctx context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return s.ListPool(ctx, service)
}

func routingPool(version uint64, entries ...routing.ModelPoolEntry) routing.Pool {
	return routing.Pool{Service: "render", Version: version, Entries: entries}
}

func routingEntry(model string, enabled bool, resolutions ...routing.Resolution) routing.ModelPoolEntry {
	return routing.ModelPoolEntry{Model: model, Enabled: enabled, SupportedResolutions: resolutions}
}

type routingTestEnv struct {
	store     *InMemoryStore
	txStore   *txTestStore
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
		txStore: newTxTestStore(),
		holder:  newFakeHolder(),
		queue:   &fakeEnqueuer{},
		quoter:  &fakeQuoter{snapshot: testSnapshot()},
		saved:   make(map[string]routing.RoutingSnapshot),
	}
	env.store = env.txStore.InMemoryStore
	seed := routing.SelectionSeed{7, 7, 7}
	resolver := routing.NewRoutingResolver(&stubPoolStore{pools: pools},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
	env.service = NewService(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.txStore, Quoter: env.quoter,
		Holder: &txTestHolder{fakeHolder: env.holder}, Queue: &txTestEnqueuer{fakeEnqueuer: env.queue},
		Resolver: resolver,
		SnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			env.saveCalls++
			if env.saveErr != nil {
				return env.saveErr
			}
			env.saved[taskID] = snap
			return nil
		},
		Acceptance: &fakeSubmissionAcceptance{},
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
	if len(env.holder.holds) != 0 {
		t.Fatalf("holds = %v, want none (the failed pin rolls the hold back, never strands it)", env.holder.holds)
	}
	if env.queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 (never enqueued without snapshot)", env.queue.calls)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed settlement")
	}
}

// merged from plan_test.go (tasks.go aggregation: submit plan, prompt/mask validation, status guards, billing availability).
// Submit-time durable plan tests: Create persists the task with no
// submit-time model mint. The single-value admin mapping is removed and
// the durable envelope carries no model; the RoutingSnapshot pins in the
// submit transaction instead. Canonical params ride
// CanonicalParams untouched, and the transient ResolvedModel handoff
// stays empty outside worker execution.

func planTestService() (*InMemoryStore, *fakeHolder, *fakeEnqueuer, *Service) {
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: &txTestHolder{fakeHolder: holder}, Queue: &txTestEnqueuer{fakeEnqueuer: queue},
		Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{},
	})
	return store, holder, queue, service
}

func TestCreatePersistsWithoutResolvedModel(t *testing.T) {
	ctx := context.Background()
	store, _, _, service := planTestService()
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		CanonicalParams: `{"schema_version":6,"service":"render"}`,
	}, "plan-key-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Service != "render" {
		t.Fatalf("service = %q, want render", created.Service)
	}
	if created.ResolvedModel != "" {
		t.Fatalf("resolved_model = %q, want empty until the #848 snapshot lands", created.ResolvedModel)
	}
	if created.CanonicalParams != `{"schema_version":6,"service":"render"}` {
		t.Fatalf("canonical_params = %q, want passthrough untouched", created.CanonicalParams)
	}
	// The persisted fact survives the store round-trip (durable plan).
	loaded, err := store.GetTask(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.Service != "render" || loaded.ResolvedModel != "" {
		t.Fatalf("loaded service = %q resolved_model = %q, want render and empty", loaded.Service, loaded.ResolvedModel)
	}
}

func TestCreateRejectsUnknownService(t *testing.T) {
	ctx := context.Background()
	_, _, _, service := planTestService()
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "nope",
	}, "plan-key-2")
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Create err = %T %v, want *ValidationError", err, err)
	}
	if validationErr.Fields["service"] == "" {
		t.Fatalf("Create fields = %v, want a service entry", validationErr.Fields)
	}
}

func TestCreateRejectsBeforeHoldWhenRoutingIsUnavailable(t *testing.T) {
	ctx := context.Background()
	txStore := newTxTestStore()
	store := txStore.InMemoryStore
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	quoter := &fakeQuoter{snapshot: testSnapshot()}
	// No resolver wired: submit fails closed with the canonical
	// unavailable sentinel before any quote, hold, task, or enqueue
	// work.
	service := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: txStore, Quoter: quoter,
		Holder: &txTestHolder{fakeHolder: holder}, Queue: &txTestEnqueuer{fakeEnqueuer: queue},
		SnapshotSaver: noopSnapshotSaver, Acceptance: &fakeSubmissionAcceptance{},
	})

	_, quoteErr := service.Quote(ctx, QuoteParams{Service: "render"})
	if !errors.Is(quoteErr, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Quote err = %v, want ErrServiceResolutionUnavailable", quoteErr)
	}

	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "routing-key-1")
	if !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err = %v, want ErrServiceResolutionUnavailable", err)
	}
	count, _ := store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if len(holder.holds) != 0 || len(holder.released) != 0 {
		t.Fatalf("routing rejection must not touch credits: holds=%v released=%v", holder.holds, holder.released)
	}
	if queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0", queue.calls)
	}
	if quoter.calls != 0 {
		t.Fatalf("quote calls = %d, want 0", quoter.calls)
	}
}

// merged from prompt_mask_test.go (tasks.go aggregation: submit plan, prompt/mask validation, status guards, billing availability).
// v1 prompt input (data.prompt): contract-typed, length-bounded at
// MaxWorkspaceTaskPromptBytes (8KB). Empty means no prompt;
// feature policy is enforced at canonical compile time.
func TestValidateWorkspacePromptBound(t *testing.T) {
	for _, prompt := range []string{"", "   ", "a red bicycle", strings.Repeat("x", MaxWorkspaceTaskPromptBytes)} {
		if err := ValidateWorkspacePrompt(prompt); err != nil {
			t.Fatalf("prompt %q err = %v, want nil", prompt, err)
		}
	}
	over := strings.Repeat("x", MaxWorkspaceTaskPromptBytes+1)
	err := ValidateWorkspacePrompt(over)
	if !IsValidationError(err) {
		t.Fatalf("over-long prompt err = %v, want validation", err)
	}
	verr, ok := err.(*ValidationError)
	if !ok || verr.Fields["prompt"] != "too_long" {
		t.Fatalf("fields = %v, want prompt=too_long", err)
	}
	// The bound is bytes, not runes: CJK text counts 3 bytes per glyph.
	cjk := strings.Repeat("汉", MaxWorkspaceTaskPromptBytes/3+1)
	if err := ValidateWorkspacePrompt(cjk); !IsValidationError(err) {
		t.Fatalf("over-long CJK prompt err = %v, want validation", err)
	}
}

// params.mask_asset_id: the only durable mask path (workspace public
// asset_... from the upload-session flow). Data-URL/storage-URL masks and
// non-string values fail closed; params.prompt stays unknown_field so the
// discriminated union keeps rejecting it.
func TestValidateParamsMaskAssetID(t *testing.T) {
	if err := ValidateParams(map[string]any{"mask_asset_id": "asset_abc123"}); err != nil {
		t.Fatalf("mask asset err = %v, want nil", err)
	}
	for _, tc := range []struct {
		name   string
		params map[string]any
		field  string
	}{
		{"data-url mask", map[string]any{"mask_asset_id": "data:image/png;base64,AAAA"}, "params.mask_asset_id"},
		{"storage url mask", map[string]any{"mask_asset_id": "https://cdn.example.com/mask.png"}, "params.mask_asset_id"},
		{"garbage mask", map[string]any{"mask_asset_id": "not-an-id"}, "params.mask_asset_id"},
		{"numeric mask", map[string]any{"mask_asset_id": 123}, "params.mask_asset_id"},
		{"empty mask", map[string]any{"mask_asset_id": "  "}, "params.mask_asset_id"},
		{"params prompt still rejected", map[string]any{"prompt": "a sunset"}, "params.prompt"},
	} {
		err := ValidateParams(tc.params)
		if !IsValidationError(err) {
			t.Fatalf("%s err = %v, want validation", tc.name, err)
		}
		verr, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("%s err type = %T, want *ValidationError", tc.name, err)
		}
		if _, exists := verr.Fields[tc.field]; !exists {
			t.Fatalf("%s fields = %v, want %q", tc.name, verr.Fields, tc.field)
		}
	}
}

func TestMaskAssetIDFromParams(t *testing.T) {
	id, ok := MaskAssetIDFromParams(map[string]any{"mask_asset_id": "  asset_abc123  "})
	if !ok || id != "asset_abc123" {
		t.Fatalf("got (%q, %v), want (asset_abc123, true)", id, ok)
	}
	for _, params := range []map[string]any{
		nil, {}, {"mask_asset_id": 123}, {"mask_asset_id": "   "}, {"other": "x"},
	} {
		if _, ok := MaskAssetIDFromParams(params); ok {
			t.Fatalf("params %v: want absent", params)
		}
	}
}

// Submit-time mask readiness: ready masks persist; invisible, non-ready,
// and archived masks fail the submit (never a late worker failure).
func TestCreateVerifiesMaskAsset(t *testing.T) {
	ctx := context.Background()
	newMaskedService := func() (*InMemoryStore, *Service) {
		txStore := newTxTestStore()
		store := txStore.InMemoryStore
		assets := &fakeAssets{assets: map[string]*AssetInfo{
			"ws_abc\x00asset_ready":   {ID: "asset_ready", WorkspaceID: "ws_abc", Status: "ready"},
			"ws_abc\x00asset_mask":    {ID: "asset_mask", WorkspaceID: "ws_abc", Status: "ready"},
			"ws_abc\x00asset_pending": {ID: "asset_pending", WorkspaceID: "ws_abc", Status: "pending"},
			"ws_abc\x00asset_old":     {ID: "asset_old", WorkspaceID: "ws_abc", Status: "ready", Archived: true},
		}}
		service := NewService(ServiceDeps{
			TaskRead: store, TaskLifecycle: txStore, Quoter: &fakeQuoter{snapshot: testSnapshot()},
			Holder: &txTestHolder{fakeHolder: newFakeHolder()}, Queue: &txTestEnqueuer{fakeEnqueuer: &fakeEnqueuer{}},
			Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
			Acceptance: &fakeSubmissionAcceptance{assets: assets},
		})
		return store, service
	}
	_, service := newMaskedService()
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "inpaint",
		MediaAssetIDs: []string{"asset_ready"},
		Params:        map[string]any{"mask_asset_id": "asset_mask"},
	}, "mask-ok")
	if err != nil {
		t.Fatalf("ready mask create err = %v", err)
	}
	if id, ok := MaskAssetIDFromParams(created.Params); !ok || id != "asset_mask" {
		t.Fatalf("persisted mask = (%q, %v), want (asset_mask, true)", id, ok)
	}
	for _, tc := range []struct {
		name   string
		params map[string]any
		want   error
	}{
		{"missing mask", map[string]any{"mask_asset_id": "asset_missing"}, ErrMediaNotFound},
		{"pending mask", map[string]any{"mask_asset_id": "asset_pending"}, ErrMediaNotReady},
		{"archived mask", map[string]any{"mask_asset_id": "asset_old"}, ErrMediaArchived},
	} {
		_, service := newMaskedService()
		_, err := service.Create(ctx, CreateInput{
			WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "inpaint",
			MediaAssetIDs: []string{"asset_ready"}, Params: tc.params,
		}, "mask-"+tc.name)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// merged from status_test.go (tasks.go aggregation: submit plan, prompt/mask validation, status guards, billing availability).

// TestCanTransitionMirrorsSQLGuards pins the durable transition table to
// the guards in internal/db/queries/workspace_tasks.sql: Claim fences
// pending -> running; Complete/Fail/Cancel flip a live (pending/running)
// task; Retry returns a live task to pending; terminals have no outgoing
// transitions. Every true entry was already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]Status{
		{StatusPending, StatusRunning},
		{StatusPending, StatusPending},
		{StatusPending, StatusCompleted},
		{StatusPending, StatusFailed},
		{StatusPending, StatusCancelled},
		{StatusRunning, StatusPending},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusCancelled},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []Status{StatusCompleted, StatusFailed, StatusCancelled}
	live := []Status{StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(StatusRunning, StatusRunning) {
		t.Error("CanTransition(running, running) = true, want false (retry returns to pending, never running -> running)")
	}
}

func TestIsTerminalStatus(t *testing.T) {
	for status, want := range map[Status]bool{
		StatusPending:   false,
		StatusRunning:   false,
		StatusCompleted: true,
		StatusFailed:    true,
		StatusCancelled: true,
	} {
		if got := IsTerminalStatus(status); got != want {
			t.Errorf("IsTerminalStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

// merged from billing_availability_test.go (tasks.go aggregation: submit plan, prompt/mask validation, status guards, billing availability).
// Billing + readiness availability gate tests (#851, ADR-0012 rules 3, 8,
// 10, 11). #848 proved the resolver seam, snapshot pinning, and the
// unavailable-reserves-nothing property for the single-transaction path; this file closes the #851 remainder:
//
//   - quote and submit apply the service policy default (1k) for an omitted
//     resolution, converging with readiness instead of silently billing
//     "default";
//   - quoted/settled prices are keyed by Service + resolution + quantity
//     only: swapping pool Models (or versions) never moves the price;
//   - billing snapshots retain the requested Service (SKU), resolution,
//     and quantity;
//   - no billing/readiness/quote file performs a single-value Model
//     mapping lookup (clean-cut: the shared RoutingResolver is the only
//     routing seam).
//
// The fakes below echo the requested resolution into the snapshot (the
// shared fakeQuoter in tasks_test.go pins OutputResolution "default"),
// which is what the retention proofs need.

type echoQuoter struct {
	unit           int64
	calls          int
	lastResolution string
}

func (q *echoQuoter) Quote(ctx context.Context, service, resolution string, quantity int64) (BillingSnapshot, error) {
	q.calls++
	if quantity <= 0 {
		quantity = 1
	}
	unit := q.unit
	if unit == 0 {
		unit = 10
	}
	q.lastResolution = resolution
	return BillingSnapshot{
		SKU: "ai-image:" + service, DisplayName: service,
		PriceVersionID: 1, PriceVersion: 1, OutputResolution: resolution,
		UnitCredits: unit, Quantity: quantity, TotalCredits: unit * quantity,
		SettlementState: "quoted",
	}, nil
}

func (q *echoQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, service, resolution string, quantity int64) (BillingSnapshot, error) {
	return q.Quote(ctx, service, resolution, quantity)
}

type perResolutionQuoter struct {
	prices map[string]int64
	calls  int
}

func (q *perResolutionQuoter) Quote(ctx context.Context, service, resolution string, quantity int64) (BillingSnapshot, error) {
	q.calls++
	unit, ok := q.prices[resolution]
	if !ok {
		return BillingSnapshot{}, errors.New("perResolutionQuoter: unpriced resolution " + resolution)
	}
	if quantity <= 0 {
		quantity = 1
	}
	return BillingSnapshot{
		SKU: "ai-image:" + service, DisplayName: service,
		PriceVersionID: 1, PriceVersion: 1, OutputResolution: resolution,
		UnitCredits: unit, Quantity: quantity, TotalCredits: unit * quantity,
		SettlementState: "quoted",
	}, nil
}

func (q *perResolutionQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, service, resolution string, quantity int64) (BillingSnapshot, error) {
	return q.Quote(ctx, service, resolution, quantity)
}

type availabilityTestEnv struct {
	store     *InMemoryStore
	txStore   *txTestStore
	holder    *fakeHolder
	queue     *fakeEnqueuer
	service   *Service
	saved     map[string]routing.RoutingSnapshot
	saveCalls int
}

func newAvailabilityTestEnv(quoter Quoter, pools map[string]routing.Pool) *availabilityTestEnv {
	env := &availabilityTestEnv{
		txStore: newTxTestStore(),
		holder:  newFakeHolder(),
		queue:   &fakeEnqueuer{},
		saved:   make(map[string]routing.RoutingSnapshot),
	}
	env.store = env.txStore.InMemoryStore
	seed := routing.SelectionSeed{9, 9, 9}
	resolver := routing.NewRoutingResolver(&stubPoolStore{pools: pools},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
	env.service = NewService(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.txStore, Quoter: quoter,
		Holder: &txTestHolder{fakeHolder: env.holder}, Queue: &txTestEnqueuer{fakeEnqueuer: env.queue},
		Resolver: resolver,
		SnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			env.saveCalls++
			env.saved[taskID] = snap
			return nil
		},
		Acceptance: &fakeSubmissionAcceptance{},
	})
	return env
}

// TestAvailabilityPriceInvariantAcrossModelChanges proves the billing key
// is Service + resolution + quantity only: two pools with disjoint Models
// (and different versions) quote and settle byte-identical prices.
func TestAvailabilityPriceInvariantAcrossModelChanges(t *testing.T) {
	ctx := context.Background()
	poolsA := map[string]routing.Pool{"render": routingPool(1,
		routingEntry("a/first", true, routing.Resolution1K, routing.Resolution2K),
	)}
	poolsB := map[string]routing.Pool{"render": routingPool(7,
		routingEntry("z/last", true, routing.Resolution1K, routing.Resolution2K, routing.Resolution4K),
		routingEntry("m/middle", true, routing.Resolution2K),
	)}
	envA := newAvailabilityTestEnv(&echoQuoter{unit: 12}, poolsA)
	envB := newAvailabilityTestEnv(&echoQuoter{unit: 12}, poolsB)

	quoteA, err := envA.service.Quote(ctx, QuoteParams{Service: "render", Resolution: "2k", Quantity: 3})
	if err != nil {
		t.Fatalf("Quote A: %v", err)
	}
	quoteB, err := envB.service.Quote(ctx, QuoteParams{Service: "render", Resolution: "2k", Quantity: 3})
	if err != nil {
		t.Fatalf("Quote B: %v", err)
	}
	for _, field := range [][2]int64{{quoteA.UnitCredits, quoteB.UnitCredits}, {quoteA.TotalCredits, quoteB.TotalCredits}, {quoteA.Quantity, quoteB.Quantity}} {
		if field[0] != field[1] {
			t.Fatalf("quote price moved with the pool: A=%+v B=%+v", quoteA, quoteB)
		}
	}
	if quoteA.SKU != "ai-image:render" || quoteB.SKU != "ai-image:render" ||
		quoteA.OutputResolution != "2k" || quoteB.OutputResolution != "2k" ||
		quoteA.UnitCredits != 12 || quoteA.TotalCredits != 36 {
		t.Fatalf("quote facts = %+v / %+v, want ai-image:render @2k x3 = 36", quoteA, quoteB)
	}

	createdA, err := envA.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "2k", Quantity: 3,
	}, "price-pin-a")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	createdB, err := envB.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "2k", Quantity: 3,
	}, "price-pin-b")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if createdA.BillingSnapshot.TotalCredits != createdB.BillingSnapshot.TotalCredits ||
		createdA.BillingSnapshot.UnitCredits != createdB.BillingSnapshot.UnitCredits ||
		createdA.BillingSnapshot.SKU != createdB.BillingSnapshot.SKU ||
		createdA.BillingSnapshot.OutputResolution != createdB.BillingSnapshot.OutputResolution {
		t.Fatalf("settled price moved with the pool: A=%+v B=%+v",
			createdA.BillingSnapshot, createdB.BillingSnapshot)
	}
	// The routing decision DID move with the pool (different snapshots),
	// while the price stayed fixed: routing and pricing are independent.
	if envA.saved[createdA.ID].SelectedModel == envB.saved[createdB.ID].SelectedModel &&
		envA.saved[createdA.ID].PoolVersion == envB.saved[createdB.ID].PoolVersion {
		t.Fatal("expected distinct routing snapshots across pool edits")
	}
}

// TestAvailabilityPricesIndependentPerServiceResolutionQuantity proves
// 1K/2K/4K prices configure independently per Service and scale with
// quantity on both the quote preflight and the authoritative submit.
func TestAvailabilityPricesIndependentPerServiceResolutionQuantity(t *testing.T) {
	ctx := context.Background()
	prices := map[string]int64{"1k": 5, "2k": 10, "4k": 20}
	pools := map[string]routing.Pool{
		"render": routingPool(1, routingEntry("a/first", true,
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K)),
		"cutout": routingPool(1, routingEntry("a/first", true, routing.Resolution1K)),
	}
	env := newAvailabilityTestEnv(&perResolutionQuoter{prices: prices}, pools)

	for resolution, unit := range prices {
		quote, err := env.service.Quote(ctx, QuoteParams{Service: "render", Resolution: resolution, Quantity: 2})
		if err != nil {
			t.Fatalf("Quote render/%s: %v", resolution, err)
		}
		if quote.UnitCredits != unit || quote.TotalCredits != unit*2 || quote.Quantity != 2 {
			t.Fatalf("quote render/%s = %+v, want unit %d total %d", resolution, quote, unit, unit*2)
		}
		created, err := env.service.Create(ctx, CreateInput{
			WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: resolution, Quantity: 2,
		}, "price-per-res-"+resolution)
		if err != nil {
			t.Fatalf("Create render/%s: %v", resolution, err)
		}
		if created.BillingSnapshot.UnitCredits != unit || created.BillingSnapshot.TotalCredits != unit*2 {
			t.Fatalf("settled render/%s = %+v, want unit %d total %d",
				resolution, created.BillingSnapshot, unit, unit*2)
		}
	}

	// Fixed-resolution Services canonicalize omitted input to 1k end to end.
	quote, err := env.service.Quote(ctx, QuoteParams{Service: "cutout"})
	if err != nil {
		t.Fatalf("Quote cutout (omitted resolution): %v", err)
	}
	if quote.OutputResolution != "1k" || quote.UnitCredits != 5 {
		t.Fatalf("quote cutout = %+v, want 1k variant unit 5", quote)
	}
	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "cutout",
	}, "price-default-variant")
	if err != nil {
		t.Fatalf("Create cutout (omitted resolution): %v", err)
	}
	if created.BillingSnapshot.OutputResolution != "1k" || created.BillingSnapshot.SKU != "ai-image:cutout" {
		t.Fatalf("settled cutout = %+v, want ai-image:cutout @1k", created.BillingSnapshot)
	}
	snap := env.saved[created.ID]
	if snap.RequestedResolution != routing.Resolution1K {
		t.Fatalf("pinned requested = %q, want 1k", snap.RequestedResolution)
	}
}

// TestAvailabilityEmptyResolutionUsesServicePolicyDefault proves an omitted
// resolution converges with the readiness plan default: every Service gates
// on canonical 1k (not the match-all default).
func TestAvailabilityEmptyResolutionUsesServicePolicyDefault(t *testing.T) {
	ctx := context.Background()

	// Scalable render with only a 2k-capable Model: the omitted resolution
	// means 1k, so the pool is unavailable (the old match-all default
	// would have admitted it).
	twoKOnly := newAvailabilityTestEnv(&echoQuoter{}, map[string]routing.Pool{
		"render": routingPool(1, routingEntry("b/second", true, routing.Resolution2K)),
	})
	if _, err := twoKOnly.service.Quote(ctx, QuoteParams{Service: "render"}); !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Quote render (omitted) err = %v, want ErrServiceResolutionUnavailable", err)
	}
	if _, err := twoKOnly.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "policy-default-unavail"); !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create render (omitted) err = %v, want ErrServiceResolutionUnavailable", err)
	}
	if count, _ := twoKOnly.store.CountTasks(ctx, "ws_abc"); count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if len(twoKOnly.holder.holds) != 0 || twoKOnly.queue.calls != 0 {
		t.Fatalf("unavailable omitted resolution touched credits/queue: holds=%v queued=%d",
			twoKOnly.holder.holds, twoKOnly.queue.calls)
	}

	// Scalable render with a 1k-capable Model: omitted resolves to 1k and
	// the pinned snapshot, billing snapshot, and quote all say 1k.
	oneK := newAvailabilityTestEnv(&echoQuoter{}, map[string]routing.Pool{
		"render": routingPool(2, routingEntry("a/first", true, routing.Resolution1K)),
	})
	quote, err := oneK.service.Quote(ctx, QuoteParams{Service: "render"})
	if err != nil {
		t.Fatalf("Quote render (omitted): %v", err)
	}
	if quote.OutputResolution != "1k" {
		t.Fatalf("quote resolution = %q, want 1k", quote.OutputResolution)
	}
	created, err := oneK.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "policy-default-1k")
	if err != nil {
		t.Fatalf("Create render (omitted): %v", err)
	}
	if created.BillingSnapshot.OutputResolution != "1k" || created.BillingSnapshot.SKU != "ai-image:render" {
		t.Fatalf("settled = %+v, want ai-image:render @1k", created.BillingSnapshot)
	}
	if snap := oneK.saved[created.ID]; snap.RequestedResolution != routing.Resolution1K {
		t.Fatalf("pinned requested = %q, want 1k", snap.RequestedResolution)
	}
}

// TestAvailabilitySnapshotsRetainServiceAndResolution proves billing
// snapshots carry the requested Service (SKU), resolution, and quantity on
// both the quote preflight and the settled task.
func TestAvailabilitySnapshotsRetainServiceAndResolution(t *testing.T) {
	ctx := context.Background()
	env := newAvailabilityTestEnv(&echoQuoter{}, map[string]routing.Pool{
		"variations": routingPool(1, routingEntry("a/first", true,
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K)),
	})
	quote, err := env.service.Quote(ctx, QuoteParams{Service: "variations", Resolution: "4K", Quantity: 2})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if quote.SKU != "ai-image:variations" || quote.OutputResolution != "4K" || quote.Quantity != 2 {
		t.Fatalf("quote = %+v, want ai-image:variations @4K x2", quote)
	}
	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "variations", Resolution: "4K", Quantity: 2,
	}, "snapshot-retention-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	retained := created.BillingSnapshot
	if retained.SKU != "ai-image:variations" || retained.OutputResolution != "4K" || retained.Quantity != 2 {
		t.Fatalf("settled snapshot = %+v, want ai-image:variations @4K x2", retained)
	}
	if created.Service != "variations" {
		t.Fatalf("task service = %q, want variations", created.Service)
	}
	loaded, err := env.store.GetTask(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.BillingSnapshot.SKU != "ai-image:variations" || loaded.BillingSnapshot.OutputResolution != "4K" {
		t.Fatalf("durable snapshot = %+v, want service+resolution retained", loaded.BillingSnapshot)
	}
}

// TestAvailabilityExplicitDefaultForScalableIsValidation enforces
// ADR-0012 rule 3 at ingress: explicit "default" is a deterministic caller
// bug (422 validation_failed on resolution), never a wildcard, and it
// reserves nothing.
func TestAvailabilityExplicitDefaultForScalableIsValidation(t *testing.T) {
	ctx := context.Background()
	quoter := &echoQuoter{}
	env := newAvailabilityTestEnv(quoter, map[string]routing.Pool{
		"render":  routingPool(1, routingEntry("a/first", true, routing.Resolution1K, routing.Resolution2K)),
		"upscale": routingPool(1, routingEntry("a/first", true, routing.Resolution1K)),
	})

	assertResolutionInvalid := func(name string, err error) {
		t.Helper()
		validationErr, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("%s err = %v, want *ValidationError", name, err)
		}
		if validationErr.Fields["resolution"] == "" {
			t.Fatalf("%s fields = %v, want a resolution entry", name, validationErr.Fields)
		}
	}
	if _, err := env.service.Quote(ctx, QuoteParams{Service: "render", Resolution: "default"}); err == nil {
		t.Fatal("Quote render @default must fail")
	} else {
		assertResolutionInvalid("Quote render @default", err)
	}
	if _, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "DEFAULT",
	}, "explicit-default-scalabale"); err == nil {
		t.Fatal("Create render @DEFAULT must fail")
	} else {
		assertResolutionInvalid("Create render @DEFAULT", err)
	}
	if quoter.calls != 0 {
		t.Fatalf("quoter calls = %d, want 0 (validation precedes quote)", quoter.calls)
	}
	if count, _ := env.store.CountTasks(ctx, "ws_abc"); count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if len(env.holder.holds) != 0 || env.queue.calls != 0 {
		t.Fatalf("explicit default touched credits/queue: holds=%v queued=%d", env.holder.holds, env.queue.calls)
	}

	// Fixed services also accept only the canonical 1k/2k/4k values.
	if _, err := env.service.Quote(ctx, QuoteParams{Service: "upscale"}); err != nil {
		t.Fatalf("Quote upscale (omitted): %v", err)
	}
	if _, err := env.service.Quote(ctx, QuoteParams{Service: "upscale", Resolution: "default"}); err == nil {
		t.Fatal("Quote upscale @default must fail")
	}
	// Omitted resolutions for scalable services keep the 1k policy
	// default and are unaffected by the explicit-default rejection.
	if _, err := env.service.Quote(ctx, QuoteParams{Service: "render"}); err != nil {
		t.Fatalf("Quote render (omitted): %v", err)
	}
}

// TestAvailabilityNeverReadsSingleModelMappings pins the clean cut at
// compile time: no production file in this settlement package may reach
// the old single-value feature -> model mapping.
func TestAvailabilityNeverReadsSingleModelMappings(t *testing.T) {
	archtest.AssertNoForbiddenText(t, ".", []string{"ResolveModel", "ModelStore", "modelMapping", "ai_gateway_model_"})
}
