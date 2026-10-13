package routing

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"ideaboard/api/internal/platform/telemetry"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
)

// seedGenerator mints selection seeds. It is a field (defaulting to
// NewSelectionSeed) so tests pin deterministic seeds without touching
// crypto randomness.
type seedGenerator func() (SelectionSeed, error)

// RoutingResolver is the sole authoritative routing decision point
// (ADR-0012 rule 10). Readiness, quote, and submit call it; worker
// execution consumes only its persisted snapshot and never performs a
// second live model lookup.
type RoutingResolver struct {
	store   PoolStore
	newSeed seedGenerator
}

var (
	ErrPoolStoreUnavailable              = errors.New("routing: pool store unavailable")
	ErrTransactionalPoolStoreUnavailable = errors.New("routing: transactional pool store unavailable")
)

// ResolverOption customizes a RoutingResolver.
type ResolverOption func(*RoutingResolver)

// WithSeedGenerator overrides seed minting. Production code must not
// use it; tests use it for deterministic replay.
func WithSeedGenerator(fn func() (SelectionSeed, error)) ResolverOption {
	return func(r *RoutingResolver) {
		if fn != nil {
			r.newSeed = fn
		}
	}
}

// NewRoutingResolver builds the resolver over the catalog's PoolStore.
// The store must be non-nil.
func NewRoutingResolver(store PoolStore, opts ...ResolverOption) *RoutingResolver {
	r := &RoutingResolver{store: store, newSeed: NewSelectionSeed}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func normalizeRoutingRequest(service, resolution string) (string, Resolution, error) {
	normalizedService, err := NormalizeService(service)
	if err != nil {
		return "", "", err
	}
	requested, err := NormalizeResolution(resolution)
	if err != nil {
		return "", "", err
	}
	return normalizedService, requested, nil
}

// Resolve runs eligibility, deterministic selection, and snapshot
// construction for one normalized request. It returns the immutable
// RoutingSnapshot on success, *ResolutionUnavailableError (matching
// ErrServiceResolutionUnavailable) when no entry is eligible, or
// *InvalidRoutingInputError / store errors otherwise.
//
// Eligibility is exactly: enabled entries whose supported resolutions
// contain the requested canonical resolution. Eligible model references are
// sorted lexicographically before selection.
func (r *RoutingResolver) Resolve(ctx context.Context, service, resolution string) (snapshot RoutingSnapshot, err error) {
	if r == nil || r.store == nil {
		return RoutingSnapshot{}, ErrPoolStoreUnavailable
	}
	if err := ctx.Err(); err != nil {
		return RoutingSnapshot{}, err
	}
	normalizedService, requested, err := normalizeRoutingRequest(service, resolution)
	if err != nil {
		return RoutingSnapshot{}, err
	}
	ctx, span := telemetry.Start(ctx, "generation.routing.resolve",
		attribute.String("service", normalizedService), attribute.String("resolution", string(requested)))
	defer func() { telemetry.End(span, err) }()
	pool, err := r.store.ListPool(ctx, normalizedService)
	if err != nil {
		return RoutingSnapshot{}, wrapStoreError(err)
	}
	return r.buildSnapshot(pool, normalizedService, requested)
}

// ResolveInTx performs the submit-time routing decision against the caller's
// settlement transaction. A non-transactional store is rejected instead of
// silently falling back to a separate connection and snapshot.
func (r *RoutingResolver) ResolveInTx(ctx context.Context, tx pgx.Tx, service, resolution string) (snapshot RoutingSnapshot, err error) {
	if r == nil || r.store == nil {
		return RoutingSnapshot{}, ErrTransactionalPoolStoreUnavailable
	}
	if tx == nil {
		return RoutingSnapshot{}, fmt.Errorf("%w: transaction is nil", ErrTransactionalPoolStoreUnavailable)
	}
	store, ok := r.store.(TransactionalPoolStore)
	if !ok {
		return RoutingSnapshot{}, ErrTransactionalPoolStoreUnavailable
	}
	if err := ctx.Err(); err != nil {
		return RoutingSnapshot{}, err
	}
	normalizedService, requested, err := normalizeRoutingRequest(service, resolution)
	if err != nil {
		return RoutingSnapshot{}, err
	}
	ctx, span := telemetry.Start(ctx, "generation.routing.resolve_in_tx",
		attribute.String("service", normalizedService), attribute.String("resolution", string(requested)))
	defer func() { telemetry.End(span, err) }()
	pool, err := store.ListPoolInTx(ctx, tx, normalizedService)
	if err != nil {
		return RoutingSnapshot{}, wrapStoreError(err)
	}
	return r.buildSnapshot(pool, normalizedService, requested)
}

func (r *RoutingResolver) buildSnapshot(pool Pool, normalizedService string, requested Resolution) (RoutingSnapshot, error) {

	eligible := make([]string, 0, len(pool.Entries))
	for _, entry := range pool.Entries {
		if entry.Supports(requested) {
			eligible = append(eligible, entry.Model)
		}
	}
	if len(eligible) == 0 {
		return RoutingSnapshot{}, &ResolutionUnavailableError{Service: normalizedService, Resolution: requested}
	}
	sort.Strings(eligible)

	seed, err := r.newSeed()
	if err != nil {
		return RoutingSnapshot{}, err
	}
	selected, retry := SelectCandidate(seed, pool.Version, normalizedService, requested, eligible)

	return RoutingSnapshot{
		Service:             normalizedService,
		RequestedResolution: requested,
		EligibleModels:      eligible,
		SelectedModel:       selected,
		RetryCandidate:      retry,
		SelectionSeed:       seed,
		PoolVersion:         pool.Version,
	}, nil
}
