// Package errors defines the Canonical Media Object structured error
// taxonomy. It is the single source of truth for the error code constants
// referenced by the media ingestion, derivative, reconciler, repair, and
// provider paths (see issue #661).
//
// Every public code has a fixed string, retryability, and blame. The blame
// classifies who is responsible for the failure (Media, Storage, Provider,
// Caller) so that retry/alert policies can stay uniform. Callers that need
// to surface a stable user-visible code wrap a `*Error` and `errors.As`
// against `*Error` to read the code; never use a raw string for comparison.
package errors

import "slices"

// Code is the stable identifier for a media error. The wire form is the
// underlying string constant; do not rename a constant without coordinating
// the front-end and the runbook.
type Code string

const (
	// CodeMediaNotFound is returned when the durable Media Asset id does
	// not resolve. It is a terminal caller error: a missing id cannot be
	// recovered without an explicit retry with a different id.
	CodeMediaNotFound Code = "media_not_found"

	// CodeObjectMissing is returned when the durable Media Asset id
	// resolves but the underlying storage object is not present. The
	// reconciler can flip ready → missing and a bounded repair can
	// re-issue the derivative; a submit-time error of this code is
	// transient from the caller perspective.
	CodeObjectMissing Code = "object_missing"

	// CodeObjectInvalid is returned when the underlying object exists
	// but its content does not match the recorded checksum, MIME, or
	// size. It is non-retryable without an explicit repair command.
	CodeObjectInvalid Code = "object_invalid"

	// CodeMediaProcessing is returned when the requested derivative is
	// still being rendered. Submit-time callers can retry; the
	// readiness validator surfaces this code when the processing-input
	// derivative is not yet ready.
	CodeMediaProcessing Code = "media_processing"

	// CodeStorageTransient is returned for retryable storage failures
	// (timeouts, network errors, transient credential errors). Caller
	// backoff is appropriate.
	CodeStorageTransient Code = "storage_transient"

	// CodeProviderRejected is returned when the upstream provider
	// rejected the request (non-2xx response, rate limit, missing
	// capability). Retry is only appropriate after the provider status
	// changes; the error code must carry the upstream's status.
	CodeProviderRejected Code = "provider_rejected"

	// CodeRepairNotPossible is returned by the bounded repair flow
	// when the configured terminal attempt limit is exhausted. It
	// requires an explicit human re-enable command to retry.
	CodeRepairNotPossible Code = "repair_not_possible"
)

// Blame classifies who is responsible for a given failure. It is the
// signal the reconciler / alerting layer uses to route the error.
type Blame string

const (
	// BlameCaller means the user supplied an invalid input. The
	// caller-visible HTTP status is 4xx and no retry is appropriate.
	BlameCaller Blame = "caller"
	// BlameMedia means the media subsystem failed in a way the
	// caller cannot influence (ingestion bug, derivative queue
	// failure, etc). The system should retry within bounds.
	BlameMedia Blame = "media"
	// BlameStorage means a configured storage profile returned an
	// error. The system retries; the operator may need to look at
	// the storage profile if the error persists.
	BlameStorage Blame = "storage"
	// BlameProvider means the upstream provider rejected the
	// request. The system does not retry without an explicit signal.
	BlameProvider Blame = "provider"
)

// AllCodes returns every defined code. Tests use this to assert no code
// is missing from the runbook.
func AllCodes() []Code {
	return []Code{
		CodeMediaNotFound,
		CodeObjectMissing,
		CodeObjectInvalid,
		CodeMediaProcessing,
		CodeStorageTransient,
		CodeProviderRejected,
		CodeRepairNotPossible,
	}
}

// IsRetryable returns true if the code represents a transient failure
// the caller (or the reconciler) should retry. The taxonomy is
// deliberately conservative: when in doubt, return false and let the
// operator re-trigger.
func (c Code) IsRetryable() bool {
	switch c {
	case CodeObjectMissing, CodeMediaProcessing, CodeStorageTransient:
		return true
	default:
		return false
	}
}

// Blame returns the blame classification for the code. It is the
// authoritative mapping for the alerting and retry policy.
func (c Code) Blame() Blame {
	switch c {
	case CodeMediaNotFound, CodeObjectInvalid, CodeRepairNotPossible:
		return BlameMedia
	case CodeObjectMissing, CodeMediaProcessing, CodeStorageTransient:
		return BlameStorage
	case CodeProviderRejected:
		return BlameProvider
	default:
		return BlameMedia
	}
}

// Valid reports whether c is a known code. Use it to validate a value
// read from a database column or a remote payload.
func (c Code) Valid() bool {
	return slices.Contains(AllCodes(), c)
}
