package routing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ModelPoolEntry is one service pool row as seen by routing (ADR-0012
// rule 2). Provider is the registry slug (lowercased at the write
// boundary; unknown slugs synthesize the default custom-<slug> namespace
// at execution, never vanish) and Model is the bare model name (no
// provider prefix, no @path; nested slashes inside the bare name, e.g.
// openrouter's vendor/model, are preserved). Entries carry no priority,
// fallback, or weight (ADR-0012 rule 5).
//
// The canonical display form provider + "/" + model is derived only for
// selection sorting, snapshot pinning, and Admin display (see
// DisplayModel): it is never stored, and execution re-splits it
// strictly (bare models without a provider fail closed).
type ModelPoolEntry struct {
	// Provider is the registry slug of the candidate model.
	Provider string
	// Model is the bare model name of the candidate model.
	Model string
	// Enabled gates visibility: disabled entries are invisible to
	// routing but stay editable/displayable in Admin.
	Enabled bool
	// SupportedResolutions lists the scalable resolutions this entry
	// can serve. It never contains "default" (ADR-0012 rule 3); an
	// enabled entry must list at least one resolution.
	SupportedResolutions []Resolution
}

// DisplayModel renders the canonical provider/model display form of one
// entry: provider + "/" + bare model. Selection sorting, snapshot
// pinning, and Admin display derive this string from the stored columns;
// it is never stored, and execution re-splits it strictly.
func (e ModelPoolEntry) DisplayModel() string {
	return e.Provider + "/" + e.Model
}

// Supports reports whether the entry serves the requested canonical
// resolution. There is no wildcard or default capability.
func (e ModelPoolEntry) Supports(requested Resolution) bool {
	if !e.Enabled || len(e.SupportedResolutions) == 0 {
		return false
	}
	for _, res := range e.SupportedResolutions {
		if res == requested {
			return true
		}
	}
	return false
}

// Pool is one service's routable pool: the per-service monotonically
// increasing version (starting at 1, bumped only on effective entry,
// enabled-state, or capability changes) plus its entries.
type Pool struct {
	Service string
	Version uint64
	Entries []ModelPoolEntry
}

// PoolStore is the port the catalog must implement. It is intentionally
// one method: entries and version are read together so selection can
// never mix entries from one pool revision with the version of another.
// The sibling catalog agent implements this; routing tests run against
// an in-memory fake.
//
// Implementations must return entries for exactly the requested
// (already normalized) service and its current pool version. A service
// with zero enabled entries is reported as an empty entry list (or a
// zero-entry pool), never as an error: emptiness converges to the
// service_resolution_unavailable sentinel in the resolver, not in the
// store. Store transport failures are returned as errors and propagate
// unwrapped by availability semantics.
type PoolStore interface {
	ListPool(ctx context.Context, service string) (Pool, error)
}

// TransactionalPoolStore reads the same pool snapshot through an ambient
// PostgreSQL transaction. Submit settlement requires this seam so the
// routing snapshot cannot observe a different pool revision than billing,
// task insertion, or River enqueue.
type TransactionalPoolStore interface {
	ListPoolInTx(ctx context.Context, tx pgx.Tx, service string) (Pool, error)
}

// wrapStoreError annotates catalog failures without reclassifying them:
// a store outage is not an empty pool and must never render the
// availability sentinel.
func wrapStoreError(err error) error {
	return fmt.Errorf("routing: list pool: %w", err)
}
