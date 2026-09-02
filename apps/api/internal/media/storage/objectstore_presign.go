package storage

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// PresignRequest is the typed input to ObjectStore.Presign. It is
// the only way the worker and the runtime resolver can ask the
// storage adapter for a signed URL; the storage package refuses to
// expose the underlying SignedURL method to non-storage callers.
//
// The request is immutable; the storage adapter is allowed to
// extend the TTL only by clamping it to the configured cap.
type PresignRequest struct {
	// StorageProfileID is the immutable storage profile the
	// target object was written through. The Presign port
	// MUST refuse to sign a URL against any other profile; a
	// profile rotation cannot silently let a manifest point
	// at a new object.
	StorageProfileID string

	// ObjectKey is the canonical storage key, not a public URL.
	// The Presign port refuses to sign any path that does not
	// look like a content object key.
	ObjectKey string

	// TTL is the requested lifetime. The adapter clamps to the
	// configured cap; the caller MUST be ready to consume the
	// URL within (TTL minus the worker overhead) and the
	// provider call timeout MUST be less than the URL TTL.
	TTL time.Duration

	// ExpectedMIMEType is an optional integrity guard. When
	// set, the adapter refuses to sign a URL whose object's
	// current MIME does not match. Use empty string to skip
	// the check.
	ExpectedMIMEType string

	ExpectedChecksumSHA256 string

	// ExpectedSizeBytes is an optional integrity guard. When
	// set, the adapter refuses to sign a URL whose object's
	// current size does not match. Use 0 to skip the check.
	ExpectedSizeBytes int64

	// ExpectedObjectGeneration is the immutable generation recorded in the
	// TaskInputManifest. It is an application fact verified with the object
	// integrity facts before the URL is issued.
	ExpectedObjectGeneration int64

	// Caller is a small free-form tag the worker passes
	// through to the audit log. It MUST NOT contain
	// credentials, tokens, or any PII; the runbook pins it as
	// the only safe field on the request envelope.
	Caller string
}

// PresignedURL is the typed output of ObjectStore.Presign. The
// worker uses the URL to build the provider call and the other
// fields to build the structured log line. The URL is intentionally
// the last field so a JSON marshal cannot accidentally print it
// in the wrong order; the runbook pins the position as a
// redactor-invariant.
type PresignedURL struct {
	URL string

	// ExpiresAt is the wall-clock time the URL stops being
	// valid. The worker uses it to refuse calls that would
	// race the deadline.
	ExpiresAt time.Time

	// ObjectGeneration is the immutable generation of the
	// underlying object. The worker records it in the
	// structured error so a future generation mismatch is
	// traceable back to the provider call.
	ObjectGeneration int64

	// StorageProfileID is the immutable storage profile the
	// signed URL was issued against. The worker uses it to
	// assert the call did not silently follow a profile
	// rotation.
	StorageProfileID string

	// PresignID is a fresh identifier the storage adapter
	// stamps on every Presign call. The runtime uses it as
	// the only safe correlation id in any log line that
	// mentions a signed URL.
	PresignID string
}

// DefaultPresignTTL is the bound applied when the caller asks
// for a longer TTL. The cap is enforced by every Presign
// implementation; raising it requires a runbook update.
const DefaultPresignTTL = 5 * time.Minute

// MaxPresignTTL is the absolute upper bound. The Presign port
// refuses to sign a URL with a longer TTL even if the caller
// asks explicitly; the worker must be re-configured to use a
// shorter provider timeout if a longer URL TTL is needed.
const MaxPresignTTL = 5 * time.Minute

// ErrPresignInvalidProfile is returned when the requested
// storage profile id is empty or unknown. The worker treats it
// as a permanent misconfiguration.
var ErrPresignInvalidProfile = errors.New("presign: storage profile id is invalid")

// ErrPresignInvalidObjectKey is returned when the requested
// object key is empty or does not look like a content object
// key. The worker treats it as a caller bug.
var ErrPresignInvalidObjectKey = errors.New("presign: object key is invalid")

// ErrPresignTTLTooLong is returned when the requested TTL
// exceeds MaxPresignTTL. The worker falls back to MaxPresignTTL
// and continues; the runbook pins this as the safe failure mode.
var ErrPresignTTLTooLong = errors.New("presign: ttl exceeds maximum cap")

// ErrPresignUnsupportedProvider means a storage profile cannot provide a
// third-party-safe, short-lived object URL. Provider execution must fail
// closed instead of leaking a same-origin application delivery URL.
var ErrPresignUnsupportedProvider = errors.New("presign: storage provider does not support provider hand-off")

// ObjectStore is the typed port every non-storage package
// consumes. It is the only public surface that issues signed
// URLs; the underlying S3 / R2 / local adapters stay internal.
// Implementations live in the storage package; tests stub this
// interface to assert URL TTL, MIME/size guards, and the
// profile immutability contract.
type ObjectStore interface {
	// Presign issues a short-lived signed URL for a single
	// object. The implementation MUST clamp TTL to
	// MaxPresignTTL, MUST refuse unknown profiles, and
	// MUST return a PresignedURL with all five fields
	// populated. The PresignID is opaque; the runtime
	// surfaces it in log lines instead of the URL itself.
	Presign(ctx context.Context, req PresignRequest) (PresignedURL, error)
}

// clampTTL returns the effective TTL after applying the cap. The
// helper is exported as a small utility so test stubs can reuse
// the same logic.
func clampTTL(requested time.Duration) (time.Duration, error) {
	if requested <= 0 {
		return DefaultPresignTTL, nil
	}
	if requested > MaxPresignTTL {
		return MaxPresignTTL, fmt.Errorf("%w (requested %s, capped to %s)", ErrPresignTTLTooLong, requested, MaxPresignTTL)
	}
	return requested, nil
}
