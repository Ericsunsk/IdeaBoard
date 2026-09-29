package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/routing"

	"github.com/jackc/pgx/v5"
)

// ErrRoutingSnapshotMissing reports a task row with no pinned snapshot:
// either the task does not exist or routing_snapshot is NULL (pre-pool
// rows, or a submit path that bypassed SaveRoutingSnapshot). Callers
// must fail closed on it: executing without a snapshot would re-read
// the mutable live pool (ADR-0012 rule 9).
var ErrRoutingSnapshotMissing = errors.New("catalog: task has no routing snapshot")

// ErrRoutingSnapshotCorrupt reports a task row whose pinned snapshot
// exists but cannot be trusted: malformed JSON or a snapshot that no
// longer replays via RoutingSnapshot.Verify. Like a missing snapshot it
// fails closed deterministically (hold released, no retry burn); it is
// a distinct sentinel so workers can separate "never pinned" from
// "pinned but invalid" without string-matching error text, while store
// transport failures (returned unwrapped below) stay River-backoff
// eligible.
var ErrRoutingSnapshotCorrupt = errors.New("catalog: stored routing snapshot is corrupt")

// validateSnapshot enforces the snapshot contract before persist and
// after load: normalized service and resolution, plus Verify (non-empty
// sorted eligible set with a replayed selection match), so a stored
// snapshot always replays without the live pool.
func validateSnapshot(snap routing.RoutingSnapshot) error {
	if _, err := routing.NormalizeService(snap.Service); err != nil {
		return fmt.Errorf("catalog: invalid snapshot service: %w", err)
	}
	if _, err := routing.NormalizeResolution(string(snap.RequestedResolution)); err != nil {
		return fmt.Errorf("catalog: invalid snapshot resolution: %w", err)
	}
	if err := snap.Verify(); err != nil {
		return fmt.Errorf("catalog: invalid routing snapshot: %w", err)
	}
	return nil
}

// SaveRoutingSnapshot persists the immutable RoutingSnapshot JSONB on
// the task row. Wave 2 submit calls it inside the submit transaction
// (Credit Hold + task insert + River enqueue) with the transaction's
// *dbsqlc.Queries, so the snapshot pins atomically; outside a
// transaction pass dbsqlc.New(pool). The snapshot is validated
// (Verify) before write: a snapshot that cannot replay is a caller
// bug and fails here, never as a corrupt row. A zero-row update
// (unknown task) returns ErrRoutingSnapshotMissing. Snapshots are
// write-once by contract: overwriting a pinned snapshot is rejected
// by the caller, not here — this function performs the plain pinned
// write the submit transaction needs.
func SaveRoutingSnapshot(ctx context.Context, q *dbsqlc.Queries, taskPublicID string, snap routing.RoutingSnapshot) error {
	if q == nil {
		return errors.New("catalog: snapshot queries are unavailable")
	}
	if taskPublicID == "" {
		return fmt.Errorf("%w: blank task id", ErrRoutingSnapshotMissing)
	}
	if err := validateSnapshot(snap); err != nil {
		return err
	}
	encoded, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("catalog: encode routing snapshot: %w", err)
	}
	rows, err := q.SetTaskRoutingSnapshot(ctx, dbsqlc.SetTaskRoutingSnapshotParams{
		PublicID:        taskPublicID,
		RoutingSnapshot: encoded,
	})
	if err != nil {
		return fmt.Errorf("catalog: persist routing snapshot: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: task %q", ErrRoutingSnapshotMissing, taskPublicID)
	}
	return nil
}

// LoadRoutingSnapshot reads and validates the pinned snapshot for a
// task. The worker (wave 2) consumes only this: it must not read the
// live pool for an accepted task. Missing rows and NULL snapshots
// return ErrRoutingSnapshotMissing; corrupt JSON or a snapshot that
// no longer replays returns a wrapped validation error — both fail
// closed, never fall back to live-pool reconstruction.
func LoadRoutingSnapshot(ctx context.Context, q *dbsqlc.Queries, taskPublicID string) (routing.RoutingSnapshot, error) {
	if q == nil {
		return routing.RoutingSnapshot{}, errors.New("catalog: snapshot queries are unavailable")
	}
	if taskPublicID == "" {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: blank task id", ErrRoutingSnapshotMissing)
	}
	raw, err := q.GetTaskRoutingSnapshot(ctx, taskPublicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return routing.RoutingSnapshot{}, fmt.Errorf("%w: task %q", ErrRoutingSnapshotMissing, taskPublicID)
		}
		return routing.RoutingSnapshot{}, fmt.Errorf("catalog: load routing snapshot: %w", err)
	}
	if len(raw) == 0 {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: task %q", ErrRoutingSnapshotMissing, taskPublicID)
	}
	var snap routing.RoutingSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: %w", ErrRoutingSnapshotCorrupt, err)
	}
	if err := validateSnapshot(snap); err != nil {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: %w", ErrRoutingSnapshotCorrupt, err)
	}
	return snap, nil
}
