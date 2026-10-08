package httpapi

import (
	"context"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"

	"github.com/jackc/pgx/v5"
)

// Control mutation durable idempotency (#891).
//
// Durable scope: (operator, operation, target, key). Operator is the
// public subject (usr_* for human sessions, the stable subject string
// otherwise; numeric ids never enter the fact). Operation is the stable
// control route operation (e.g. "users.ban"). Target is the real audit
// target (target_type + target_id, empty when the route has none): the
// same key against a different target is a different request and must
// 409, never replay. Key is the header-only Idempotency-Key.
//
// Canonical request hash: HashIdempotencyRequest(method, route template,
// target_type, target_id, raw body). The route template (not the
// concrete path) keeps the hash stable; the target columns carry the
// concrete resource identity. The operator reason stays out of the
// identity on purpose: it is audit metadata, so retrying the same
// business request with a new note replays instead of conflicting.
// Only the digest is persisted, never raw fields.
//
// Retryable vs retained: the same key + same hash replays the first
// 2xx result within ReplayTTL (24h), including after a restart, across
// instances, and after a commit-after-response-loss retry. The same key
// with a different hash is a 409 conflict (idempotency_conflict) before
// any side effect runs. Business 4xx/5xx outcomes are never retained:
// the claim is abandoned in the same transaction, so the retry
// re-executes. Infrastructure failures (scope/audit/claim/commit
// errors) render 503 service_unavailable. Atomic failures are retryable;
// failed detached settlement retains uncertainty so retries cannot repeat
// an external effect that may already have succeeded.
//
// External sagas (user deletion across Auth/Storage/tombstone phases,
// Stripe/payment provider calls, NATS/River publication) cannot join
// the guard transaction by construction, but they reuse this same
// durable operation identity: the guard claim serializes concurrent
// same-key sagas on the primary key before the first external call, the
// winner's 2xx response replays to every retry, and domain-owned
// uniqueness facts underneath (ledger idempotency keys, deletion
// request phases) stay the source of truth for their own writes.

// ControlIdempotencyDurable is the narrow durable seam the guard needs
// (atomic + detached). *idempotency.ControlStore is the production
// implementation; tests substitute fakes or pool-backed stores.
//
// Transaction discipline differs by path:
//   - Atomic: every write is a participant of the caller-owned
//     guard transaction tx: they never begin or commit.
//   - Detached: the guard drives the same Claim/Complete/Abandon
//     through SHORT independent transactions (one for the claim,
//     one for the outcome), never holding a long transaction across
//     the external side effect. TryAbandonStale runs in the same
//     short-transaction discipline, fenced by the observed token, for
//     claims with no dispatch intent. MarkUnknown precedes external I/O
//     so an uncertain external result can never be reclaimed by the lease.
type ControlIdempotencyDurable interface {
	// Claim inserts the in-progress fact in tx and reports whether the
	// caller won. Losers observe Existing (replay on same hash, 409 on
	// mismatch) without running any side effect.
	Claim(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim) (idempotency.ClaimResult, error)
	// Complete flips the winner's claim to completed with the 2xx
	// response in tx. Only 2xx outcomes are retained.
	Complete(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim, result idempotency.ControlResult) error
	// Abandon deletes the winner's in-progress claim in tx so the retry
	// re-executes.
	Abandon(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim) error
	// MarkUnknown persists dispatch intent before an external side effect.
	MarkUnknown(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim) error
	// TryAbandonStale deletes a stale in-progress claim older than
	// before in tx (detached winner-lease recovery). True means one
	// orphan was removed and the caller may reclaim; false means the
	// in-progress row is fresh (active winner) or already gone, so
	// the caller must not execute unfenced.
	TryAbandonStale(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim, before time.Time) (bool, error)
	// PurgeExpired reclaims expired completed rows in autocommit,
	// best-effort after a successful commit. It never fails the call.
	PurgeExpired(ctx context.Context) (int64, error)
}

// controlDurableClaim builds the durable claim for one guarded
// mutation: operator/operation/target/key scope plus the canonical
// request hash. Target comes from the already-resolved audit record so
// the real resource always enters the request identity.
func controlDurableClaim(p principal.Principal, operation, key string, record *ControlAuditRecord, method, route string, body []byte) idempotency.ControlClaim {
	_, operatorID := controlOperatorIdentity(p)
	var targetType, targetID string
	if record != nil {
		targetType, targetID = record.TargetType, record.TargetID
	}
	return idempotency.ControlClaim{
		Operator:    operatorID,
		Operation:   operation,
		TargetType:  targetType,
		TargetID:    targetID,
		Key:         key,
		RequestHash: contract.HashIdempotencyRequest(method, route, targetType, targetID, string(body)),
	}
}
