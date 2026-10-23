// Idempotency value objects: Parse/Hash/Scope live here as the single home.
//
//   - Key is the validated opaque Idempotency-Key header value (Parse).
//   - RequestHash is the SHA-256 request fingerprint compared on key reuse
//     (HashRequest). Only the digest is persisted, never raw fields.
//   - Scope binds principal + operation into the dedup tuple (Scope).
//
// contract.* wrappers (ParseIdempotencyKey, HashIdempotencyRequest,
// IdempotencyScope) delegate here so the kernel and this package cannot
// drift. Behavior is unchanged: validation, digest stability (including
// the NUL-containing exceptional framing), and scope framing stay
// byte-identical.
//
// Authority rule: the process-local Store is a same-process fast-path
// replay cache only. Control mutations additionally claim through
// ControlStore (durable.go) into control_idempotency_records, which is the
// cross-process, restart-safe authority; task submit relies on its own
// durable task_submission_idempotency fact, never on this Store.
package idempotency

import "strings"

// Key is a validated idempotency key value object.
type Key string

// ParseKey validates a raw header value into a Key.
func ParseKey(raw string) (Key, error) {
	parsed, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return Key(parsed), nil
}

// String returns the opaque key string.
func (k Key) String() string { return string(k) }

// RequestHash is a validated request fingerprint value object.
type RequestHash string

// HashRequests derives the fingerprint compared on key reuse.
func HashRequests(parts ...string) RequestHash {
	return RequestHash(HashRequest(parts...))
}

// String returns the hex digest string.
func (h RequestHash) String() string { return string(h) }

// Scope is the validated dedup scope value object (principal + operation).
type Scope string

// NewScope binds principal (e.g. usr_...) and operation (e.g. "task.create")
// into the dedup scope. The full tuple is scope + key.
func NewScope(principal, operation string) Scope {
	return Scope(operation + "\x00" + principal)
}

// String returns the scope string.
func (s Scope) String() string { return string(s) }

// SplitScope recovers principal and operation for logging. It reports false
// when the scope was not built by NewScope.
func SplitScope(s Scope) (principal, operation string, ok bool) {
	parts := strings.SplitN(string(s), "\x00", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[1], parts[0], true
}
