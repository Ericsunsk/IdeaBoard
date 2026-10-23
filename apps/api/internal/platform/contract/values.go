package contract

import (
	"ideaboard/api/internal/platform/idempotency"
)

// Idempotency value objects (kernel view). The single implementation lives
// in platform/idempotency (values.go); these aliases keep the kernel's
// Parse/Hash/Scope vocabulary unified without duplicating validation,
// digest, or framing logic.
//
//   - IdempotencyKey is the validated header value (ParseIdempotencyKey).
//   - IdempotencyRequestHash is the reuse fingerprint (HashIdempotencyRequest).
//   - IdempotencyScopeValue binds principal + operation (IdempotencyScope).
//
// Authority rule (unchanged): the process-local IdempotencyStore is a
// same-process fast path only; Control mutations claim through the durable
// ControlStore (control_idempotency_records), which is the cross-process,
// restart-safe authority.
type (
	// IdempotencyKey is the validated opaque key value object.
	IdempotencyKey = idempotency.Key
	// IdempotencyRequestHash is the validated request fingerprint value object.
	IdempotencyRequestHash = idempotency.RequestHash
	// IdempotencyScopeValue is the validated dedup scope value object.
	IdempotencyScopeValue = idempotency.Scope
)

// ParseIdempotencyKeyValue validates a raw header value into a Key object.
func ParseIdempotencyKeyValue(raw string) (IdempotencyKey, error) {
	return idempotency.ParseKey(raw)
}

// HashIdempotencyRequestValue derives the reuse fingerprint object.
func HashIdempotencyRequestValue(parts ...string) IdempotencyRequestHash {
	return idempotency.HashRequests(parts...)
}

// NewIdempotencyScopeValue binds principal + operation into a Scope object.
func NewIdempotencyScopeValue(principal, operation string) IdempotencyScopeValue {
	return idempotency.NewScope(principal, operation)
}
