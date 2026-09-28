package workspacetasks

import (
	"context"
	"errors"
	"strings"
	"testing"
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

func allowTestRouting(context.Context, string, string) error { return nil }

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
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Routing: allowTestRouting,
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
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Routing: allowTestRouting,
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
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Routing: allowTestRouting,
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
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: newFakeHolder(), Queue: &fakeEnqueuer{}, Assets: assets, Routing: allowTestRouting,
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
