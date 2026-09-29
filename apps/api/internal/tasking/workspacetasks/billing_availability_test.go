package workspacetasks

// Billing + readiness availability gate tests (#851, ADR-0012 rules 3, 8,
// 10, 11). #848 proved the resolver seam, snapshot pinning, and the
// unavailable-reserves-nothing property for the compensating and
// single-transaction paths; this file closes the #851 remainder:
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

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/archtest"
	"ideaboard/api/internal/generation/routing"

	"github.com/jackc/pgx/v5"
)

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

type availabilityTestEnv struct {
	store     *InMemoryStore
	holder    *fakeHolder
	queue     *fakeEnqueuer
	service   *Service
	saved     map[string]routing.RoutingSnapshot
	saveCalls int
}

func newAvailabilityTestEnv(quoter Quoter, pools map[string]routing.Pool) *availabilityTestEnv {
	env := &availabilityTestEnv{
		store:  NewInMemoryStoreForTests(),
		holder: newFakeHolder(),
		queue:  &fakeEnqueuer{},
		saved:  make(map[string]routing.RoutingSnapshot),
	}
	seed := routing.SelectionSeed{9, 9, 9}
	resolver := routing.NewRoutingResolver(&stubPoolStore{pools: pools},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
	env.service = NewTestServiceForTests(ServiceDeps{
		Tasks: env.store, Quoter: quoter, Holder: env.holder, Queue: env.queue,
		Resolver: resolver,
		SnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			env.saveCalls++
			env.saved[taskID] = snap
			return nil
		},
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
