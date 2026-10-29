package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidPoolInput reports a deterministic caller bug in pool write
// input (unknown service, blank model, bad resolution, duplicates,
// enabled entry without resolutions). It is never an availability
// outcome and never maps to service_resolution_unavailable.
var ErrInvalidPoolInput = errors.New("catalog: invalid model pool input")

// PoolVersionConflict reports a lost optimistic-concurrency race: the
// pool changed between the caller's read and write. Match with
// errors.As to render the expected/current versions (Admin maps it to
// a 409-style stale retry in #849); the write committed nothing.
type PoolVersionConflict struct {
	Service  string
	Expected uint64
	Current  uint64
}

func (e *PoolVersionConflict) Error() string {
	return fmt.Sprintf("catalog: service %q pool version conflict: expected %d, current %d", e.Service, e.Expected, e.Current)
}

// PoolEntryInput is one desired pool row for ReplacePool. Provider is
// the registry slug (lowercased at the write boundary; unknown slugs are
// accepted and synthesize the default custom-<slug> namespace at
// execution, never vanish) and Model is the bare model name (no provider
// prefix, no @path, no whitespace; nested slashes inside the bare name
// are preserved). Resolutions lists scalable capabilities (1k/2k/4k,
// case-insensitive); `default` is rejected here by validation
// (ADR-0012 rule 3). Disabled entries may carry an empty capability
// set; enabled entries must list at least one resolution.
type PoolEntryInput struct {
	Provider    string
	Model       string
	Enabled     bool
	Resolutions []string
}

// ReplacePoolResult reports the outcome of ReplacePool.
type ReplacePoolResult struct {
	// Version is the pool version after the call: bumped on effective
	// change, unchanged on no-op saves, 1 on first effective write.
	Version uint64
	// Changed reports whether the write took effect (and bumped the
	// version). False means the desired pool was already effective.
	Changed bool
	// Pool is the exact pool snapshot written or observed by this call.
	// It is built inside the same transaction as Version/Changed so callers
	// do not need a racy follow-up read to render the response.
	Pool routing.Pool
}

// Repository is the Service Model Pool catalog store. It implements
// routing.PoolStore (ListPool) so the RoutingResolver reads entries
// and version atomically together.
type Repository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewRepository builds the catalog repository over a pgx pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

// normalizeService canonicalizes a service slug for catalog reads and
// writes: lowercase/trim fold plus membership in the AI image service
// registry. Unknown slugs (including the reserved "vision" slug) are
// caller bugs, never availability outcomes.
func normalizeService(raw string) (string, error) {
	service, err := routing.NormalizeService(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidPoolInput, err)
	}
	if !workflow.IsAIImageFeature(service) {
		return "", fmt.Errorf("%w: unsupported service %q", ErrInvalidPoolInput, raw)
	}
	return service, nil
}

// normalizedEntry is a validated, canonical ReplacePool row.
type normalizedEntry struct {
	provider    string
	model       string
	enabled     bool
	resolutions []routing.Resolution
}

// normalizeProvider canonicalizes one pool entry provider: trimmed,
// lowercased registry slug, 1..64 chars, no slashes, @paths, or
// whitespace. Unknown slugs are accepted: they synthesize the default
// custom-<slug> namespace at execution (documented in the Control API
// contract), so writers never need a registry gate here.
func normalizeProvider(raw string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	if provider == "" {
		return "", fmt.Errorf("%w: pool entry has a blank provider", ErrInvalidPoolInput)
	}
	if len(provider) > 64 {
		return "", fmt.Errorf("%w: provider %q exceeds 64 characters", ErrInvalidPoolInput, raw)
	}
	if strings.ContainsAny(provider, "/@ \t\n") {
		return "", fmt.Errorf("%w: provider %q must be a bare registry slug", ErrInvalidPoolInput, raw)
	}
	return provider, nil
}

// normalizePoolModel canonicalizes one pool entry bare model name against
// its provider: trimmed, case preserved, 1..128 chars, no @paths or
// whitespace; nested slashes inside the bare name (openrouter's
// vendor/model) are preserved. A redundant provider prefix
// (provider + "/" + ...) is rejected; native vendor segments are opaque
// even when their name also happens to be a registered gateway provider.
func normalizePoolModel(provider, raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" {
		return "", fmt.Errorf("%w: pool entry has a blank model", ErrInvalidPoolInput)
	}
	if len(model) > 128 {
		return "", fmt.Errorf("%w: model %q exceeds 128 characters", ErrInvalidPoolInput, raw)
	}
	if strings.ContainsAny(model, "@ \t\n") {
		return "", fmt.Errorf("%w: model %q must be a bare model name", ErrInvalidPoolInput, raw)
	}
	if strings.HasPrefix(model, "/") || strings.HasSuffix(model, "/") {
		return "", fmt.Errorf("%w: model %q must be a bare model name", ErrInvalidPoolInput, raw)
	}
	if i := strings.Index(model, "/"); i > 0 {
		first := strings.ToLower(model[:i])
		if first == provider {
			return "", fmt.Errorf("%w: model %q repeats its provider prefix; send the bare name", ErrInvalidPoolInput, raw)
		}
	}
	return model, nil
}

// normalizeEntries validates and canonicalizes one ReplacePool input:
// trimmed/lowercased providers with bare model names (case-insensitive
// duplicate detection on the provider/model pair), capability
// normalization with dedupe and sort, and the enabled-entry non-empty
// capability rule.
func normalizeEntries(inputs []PoolEntryInput) ([]normalizedEntry, error) {
	seen := make(map[string]struct{}, len(inputs))
	entries := make([]normalizedEntry, 0, len(inputs))
	for i, input := range inputs {
		provider, err := normalizeProvider(input.Provider)
		if err != nil {
			return nil, fmt.Errorf("%w: entry %d: %w", ErrInvalidPoolInput, i, err)
		}
		model, err := normalizePoolModel(provider, input.Model)
		if err != nil {
			return nil, fmt.Errorf("%w: entry %d: %w", ErrInvalidPoolInput, i, err)
		}
		key := provider + "\x00" + strings.ToLower(model)
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: entry %d duplicates model %q", ErrInvalidPoolInput, i, workflow.FormatPoolModel(provider, model))
		}
		seen[key] = struct{}{}
		resSet := make(map[routing.Resolution]struct{}, len(input.Resolutions))
		for _, raw := range input.Resolutions {
			res, err := routing.NormalizeResolution(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: entry %d model %q: %w", ErrInvalidPoolInput, i, workflow.FormatPoolModel(provider, model), err)
			}
			resSet[res] = struct{}{}
		}
		resolutions := make([]routing.Resolution, 0, len(resSet))
		for res := range resSet {
			resolutions = append(resolutions, res)
		}
		sort.Slice(resolutions, func(a, b int) bool { return resolutions[a] < resolutions[b] })
		if input.Enabled && len(resolutions) == 0 {
			return nil, fmt.Errorf("%w: entry %d model %q is enabled without resolutions", ErrInvalidPoolInput, i, workflow.FormatPoolModel(provider, model))
		}
		entries = append(entries, normalizedEntry{provider: provider, model: model, enabled: input.Enabled, resolutions: resolutions})
	}
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].provider != entries[b].provider {
			return entries[a].provider < entries[b].provider
		}
		return entries[a].model < entries[b].model
	})
	return entries, nil
}

// ReplacePool atomically replaces one Service's entire pool.
// Semantics:
//   - expectedVersion is the caller's last observed version (0 =
//     expect-no-pool-yet). A mismatch fails with *PoolVersionConflict
//     and writes nothing.
//   - The write holds a per-service advisory transaction lock, so
//     concurrent replaces serialize; the loser sees the conflict.
//   - An effective-equal desired pool is a successful no-op: it
//     returns the current version with Changed=false and does NOT
//     bump (ADR-0012 rule 9).
//   - Otherwise entries and capabilities are fully replaced and the
//     version becomes 1 (first effective write) or current+1.
//   - An empty entry list clears the pool (the Service reads as
//     unavailable until repopulated); clearing an already-empty pool
//     is a no-op.
//
// The version bump, entry replacement, and capability replacement
// commit in one transaction. Wave 2 reuses this inside Admin saves;
// submit-path snapshot pinning never calls it.
func (r *Repository) ReplacePool(ctx context.Context, service string, expectedVersion uint64, inputs []PoolEntryInput) (ReplacePoolResult, error) {
	if r == nil || r.db == nil || r.q == nil {
		return ReplacePoolResult{}, errors.New("catalog: repository is unavailable")
	}
	normalizedService, err := normalizeService(service)
	if err != nil {
		return ReplacePoolResult{}, err
	}
	desired, err := normalizeEntries(inputs)
	if err != nil {
		return ReplacePoolResult{}, err
	}
	var result ReplacePoolResult
	err = dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, _ pgx.Tx) error {
		if err := q.LockServiceModelPool(ctx, normalizedService); err != nil {
			return fmt.Errorf("catalog: lock service pool: %w", err)
		}
		current, err := poolVersion(ctx, q, normalizedService)
		if err != nil {
			return err
		}
		if current != expectedVersion {
			return &PoolVersionConflict{Service: normalizedService, Expected: expectedVersion, Current: current}
		}
		stored, err := loadEntries(ctx, q, normalizedService)
		if err != nil {
			return err
		}
		if entriesEqual(stored, desired) {
			result = ReplacePoolResult{Version: current, Changed: false, Pool: poolFromEntries(normalizedService, current, stored)}
			return nil
		}
		if err := q.DeleteServiceModelPoolResolutions(ctx, normalizedService); err != nil {
			return fmt.Errorf("catalog: replace pool resolutions: %w", err)
		}
		if err := q.DeleteServiceModelPoolEntries(ctx, normalizedService); err != nil {
			return fmt.Errorf("catalog: replace pool entries: %w", err)
		}
		for _, entry := range desired {
			row, err := q.InsertServiceModelPoolEntry(ctx, dbsqlc.InsertServiceModelPoolEntryParams{
				Service:  normalizedService,
				Provider: entry.provider,
				ModelID:  entry.model,
				Enabled:  entry.enabled,
			})
			if err != nil {
				return fmt.Errorf("catalog: insert pool entry: %w", err)
			}
			for _, res := range entry.resolutions {
				if err := q.InsertServiceModelPoolResolution(ctx, dbsqlc.InsertServiceModelPoolResolutionParams{
					PoolEntryID: row.ID,
					Resolution:  string(res),
				}); err != nil {
					return fmt.Errorf("catalog: insert pool resolution: %w", err)
				}
			}
		}
		next := current + 1
		if current == 0 {
			next = 1
		}
		if _, err := q.UpsertServiceModelPoolVersion(ctx, dbsqlc.UpsertServiceModelPoolVersionParams{
			Service: normalizedService,
			Version: int64(next),
		}); err != nil {
			return fmt.Errorf("catalog: upsert pool version: %w", err)
		}
		result = ReplacePoolResult{Version: next, Changed: true, Pool: poolFromEntries(normalizedService, next, desired)}
		return nil
	})
	if err != nil {
		return ReplacePoolResult{}, err
	}
	return result, nil
}

func poolFromEntries(service string, version uint64, stored []normalizedEntry) routing.Pool {
	entries := make([]routing.ModelPoolEntry, 0, len(stored))
	for _, entry := range stored {
		resolutions := make([]routing.Resolution, len(entry.resolutions))
		copy(resolutions, entry.resolutions)
		entries = append(entries, routing.ModelPoolEntry{
			Provider:             entry.provider,
			Model:                entry.model,
			Enabled:              entry.enabled,
			SupportedResolutions: resolutions,
		})
	}
	return routing.Pool{Service: service, Version: version, Entries: entries}
}

// poolVersion reads the stored version, returning 0 when the Service
// was never effectively written. 0 is a read-model value only; the
// table CHECK (version >= 1) guarantees it is never stored.
func poolVersion(ctx context.Context, q *dbsqlc.Queries, service string) (uint64, error) {
	version, err := q.GetServiceModelPoolVersion(ctx, service)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("catalog: read pool version: %w", err)
	}
	if version < 0 {
		return 0, fmt.Errorf("catalog: service %q has negative pool version %d", service, version)
	}
	return uint64(version), nil
}

// GetVersion returns the current pool version for a Service: 0 when
// never effectively written, otherwise the monotonic uint64 pinned
// into snapshots. Reads take no lock.
func (r *Repository) GetVersion(ctx context.Context, service string) (uint64, error) {
	if r == nil || r.db == nil || r.q == nil {
		return 0, errors.New("catalog: repository is unavailable")
	}
	normalizedService, err := normalizeService(service)
	if err != nil {
		return 0, err
	}
	return poolVersion(ctx, r.q, normalizedService)
}

// loadEntries reads one Service's stored entries with capabilities,
// sorted by model id with resolutions sorted ascending.
func loadEntries(ctx context.Context, q *dbsqlc.Queries, service string) ([]normalizedEntry, error) {
	rows, err := q.ListServiceModelPoolEntries(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("catalog: list pool entries: %w", err)
	}
	resRows, err := q.ListServiceModelPoolResolutions(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("catalog: list pool resolutions: %w", err)
	}
	byEntry := make(map[int64][]routing.Resolution, len(rows))
	for _, resRow := range resRows {
		byEntry[resRow.PoolEntryID] = append(byEntry[resRow.PoolEntryID], routing.Resolution(resRow.Resolution))
	}
	entries := make([]normalizedEntry, 0, len(rows))
	for _, row := range rows {
		resolutions := byEntry[row.ID]
		sort.Slice(resolutions, func(a, b int) bool { return resolutions[a] < resolutions[b] })
		entries = append(entries, normalizedEntry{
			provider:    row.Provider,
			model:       row.ModelID,
			enabled:     row.Enabled,
			resolutions: resolutions,
		})
	}
	return entries, nil
}

// entriesEqual compares stored rows against normalized desired input.
// Both sides are sorted by (provider, model) with sorted resolution sets.
func entriesEqual(stored, desired []normalizedEntry) bool {
	if len(stored) != len(desired) {
		return false
	}
	for i := range stored {
		if stored[i].provider != desired[i].provider || stored[i].model != desired[i].model || stored[i].enabled != desired[i].enabled {
			return false
		}
		if len(stored[i].resolutions) != len(desired[i].resolutions) {
			return false
		}
		for j := range stored[i].resolutions {
			if stored[i].resolutions[j] != desired[i].resolutions[j] {
				return false
			}
		}
	}
	return true
}

// ListPool returns one Service's routable pool with its current
// version. It implements routing.PoolStore: entries and version are
// read together (under no lock; writers serialize) so selection never
// mixes revisions. A Service with no (or no enabled) entries reports
// an empty entry list with its version — never an error. Emptiness
// converges to service_resolution_unavailable in the resolver, not
// here; transport failures propagate as errors and must never render
// the availability sentinel.
func (r *Repository) ListPool(ctx context.Context, service string) (routing.Pool, error) {
	if r == nil || r.db == nil || r.q == nil {
		return routing.Pool{}, errors.New("catalog: repository is unavailable")
	}
	normalizedService, err := normalizeService(service)
	if err != nil {
		return routing.Pool{}, err
	}
	var pool routing.Pool
	err = dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, _ pgx.Tx) error {
		// ReplacePool serializes writes with this same per-Service advisory
		// lock. Holding it across every read keeps version, entries, and
		// capabilities from being assembled from different revisions under
		// PostgreSQL's default READ COMMITTED isolation.
		if err := q.LockServiceModelPool(ctx, normalizedService); err != nil {
			return fmt.Errorf("catalog: lock service pool for read: %w", err)
		}
		version, err := poolVersion(ctx, q, normalizedService)
		if err != nil {
			return err
		}
		stored, err := loadEntries(ctx, q, normalizedService)
		if err != nil {
			return err
		}
		pool = poolFromEntries(normalizedService, version, stored)
		return nil
	})
	return pool, err
}

// ListPoolInTx reads the pool and version through the caller's transaction.
// It is the submit-time catalog seam; no nested transaction or separate pool
// connection is opened here.
func (r *Repository) ListPoolInTx(ctx context.Context, tx pgx.Tx, service string) (routing.Pool, error) {
	if r == nil || tx == nil {
		return routing.Pool{}, errors.New("catalog: transactional repository is unavailable")
	}
	normalizedService, err := normalizeService(service)
	if err != nil {
		return routing.Pool{}, err
	}
	q := dbsqlc.New(tx)
	// The submit transaction must take the same lock as ReplacePool before
	// reading version plus the entry/capability rows. Without this, separate
	// READ COMMITTED statements could mix revisions into one snapshot.
	if err := q.LockServiceModelPool(ctx, normalizedService); err != nil {
		return routing.Pool{}, fmt.Errorf("catalog: lock service pool for read: %w", err)
	}
	version, err := poolVersion(ctx, q, normalizedService)
	if err != nil {
		return routing.Pool{}, err
	}
	stored, err := loadEntries(ctx, q, normalizedService)
	if err != nil {
		return routing.Pool{}, err
	}
	return poolFromEntries(normalizedService, version, stored), nil
}
