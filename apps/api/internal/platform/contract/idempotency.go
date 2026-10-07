package contract

import (
	"ideaboard/api/internal/platform/apperror"
	"ideaboard/api/internal/platform/idempotency"

	"github.com/gin-gonic/gin"
)

// Idempotency contract (Baseline 5.6), thin wrapper over
// platform/idempotency:
//
//   - Header-only: the kernel reads ONLY the Idempotency-Key request
//     header via IdempotencyKeyFromHeader. There is no body-key
//     fallback on any route; new v1 routes must not accept a second
//     body-level idempotency field.
//   - Scope: the dedup tuple is principal + operation + key. Scope()
//     binds principal + operation; the key travels in the header. The
//     process-local Store additionally takes the internal numeric user id
//     where available. Control mutations extend this with the durable
//     fact (control_idempotency_records via ControlStore): the durable
//     scope is operator + operation + target + key, the canonical
//     request hash pins method + route template + target + body, and the
//     durable row is the cross-process authority while this store stays
//     a same-process fast path (durable uniqueness facts remain the
//     source of truth per DESIGN 3; neither store replaces them).
//   - Replay vs conflict: reusing a key with the same request hash replays
//     the first result within the TTL; reuse with a different hash is a
//     409 (CodeIdempotencyConflict, or CodeTaskIdempotencyConflict for
//     task operations).
//   - TTL: first results replay for 24h (IdempotencyReplayTTL); expired
//     keys miss and the caller re-executes.
const (
	// IdempotencyKeyHeader is the only wire carrier for idempotency keys.
	IdempotencyKeyHeader = idempotency.Header
	// IdempotencyReplayTTL bounds how long a first result is replayed.
	IdempotencyReplayTTL = idempotency.ReplayTTL
	// IdempotencyMaxKeyLength bounds the opaque key.
	IdempotencyMaxKeyLength = idempotency.MaxKeyLength
)

// IdempotencyStore is the process-local first-result replay cache for
// writes without a durable idempotency fact. Alias (not a copy) so the
// kernel and platform/idempotency share one implementation.
type IdempotencyStore = idempotency.Store

// IdempotencyStoreOptions configures an IdempotencyStore; zero values
// take production defaults (24h TTL, capped entries).
type IdempotencyStoreOptions = idempotency.StoreOptions

// NewIdempotencyStore builds a replay store from options.
func NewIdempotencyStore(opts IdempotencyStoreOptions) *IdempotencyStore {
	return idempotency.NewStore(opts)
}

// DefaultIdempotencyStoreOptions returns the production replay policy.
func DefaultIdempotencyStoreOptions() IdempotencyStoreOptions {
	return idempotency.DefaultStoreOptions()
}

// ParseIdempotencyKey validates a raw header value: non-empty after trim,
// within length. Missing/invalid keys render 400
// (CodeIdempotencyKeyRequired).
func ParseIdempotencyKey(raw string) (string, error) {
	return idempotency.Parse(raw)
}

// IdempotencyKeyFromHeader reads ONLY the Idempotency-Key header (no body
// fallback). Missing or malformed keys return a classified 400 error.
func IdempotencyKeyFromHeader(c *gin.Context) (string, error) {
	if c == nil || c.Request == nil {
		return "", idempotency.MissingError()
	}
	return idempotency.Parse(c.GetHeader(IdempotencyKeyHeader))
}

// IdempotencyScope binds principal (public id, e.g. usr_...) and operation
// (stable contract name, e.g. "task.create") into the store scope. The
// full dedup tuple is scope + key: callers pass the header key separately
// to Lookup/StoreResult/Do.
func IdempotencyScope(principal, operation string) string {
	return operation + "\x00" + principal
}

// HashIdempotencyRequest derives the request fingerprint compared on key
// reuse. Only the digest is persisted, never raw fields.
func HashIdempotencyRequest(parts ...string) string {
	return idempotency.HashRequest(parts...)
}

// TaskIdempotencyConflict classifies an Idempotency-Key reused with a
// different task request: 409 with CodeTaskIdempotencyConflict.
func TaskIdempotencyConflict(message string) *apperror.Error {
	return apperror.Conflict(apperror.Code(CodeTaskIdempotencyConflict), message)
}

// IsIdempotencyConflict reports whether err is a key-reuse conflict
// (generic or task-scoped), as opposed to missing/invalid keys.
func IsIdempotencyConflict(err error) bool {
	if idempotency.IsConflict(err) {
		return true
	}
	return apperror.CodeOf(err) == apperror.Code(CodeTaskIdempotencyConflict)
}
