package mediaerrors

import (
	"errors"
	"fmt"
	"time"
)

// Error is the structured error type the media subsystem returns to its
// callers. It carries the stable Code, a Blame classification, an
// operation label (e.g. "media.derivative.process"), and a small set of
// facts that the runbook and dashboards need to triage a failure.
//
// The Cause field is the wrapped underlying error; never serialize it
// into HTTP responses or persistent logs without redacting.
type Error struct {
	Code      Code
	Blame     Blame
	Operation string
	Message   string
	Cause     error
	// Facts carry correlation ids, retry counts, and storage
	// profile ids. They are safe to log and to return to the
	// admin read model. They MUST NOT carry the underlying
	// object's URL or any signed token.
	Facts Facts
	// OccurredAt is the wall-clock time the error was
	// constructed. River uses it to drive retryable delay
	// calculations.
	OccurredAt time.Time
}

// Facts are the structured fields attached to a media error. They are
// deliberately bounded; unbounded fields belong in logs, not in the
// error envelope.
type Facts struct {
	AssetID         string `json:"asset_id,omitempty"`
	DerivativeID    string `json:"derivative_id,omitempty"`
	StorageProfile  string `json:"storage_profile_id,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	ProviderAttempt string `json:"provider_attempt_id,omitempty"`
	CorrelationID   string `json:"correlation_id,omitempty"`
	Attempt         int    `json:"attempt,omitempty"`
	MaxAttempts     int    `json:"max_attempts,omitempty"`
}

// New constructs a new *Error with the given code, operation, and
// cause. The blame is filled in from the code's classification; callers
// that need a different blame should use NewWithBlame.
func New(code Code, operation, message string, cause error) *Error {
	return NewWithBlame(code, code.Blame(), operation, message, cause)
}

// NewWithBlame is like New but allows the caller to override the
// classified blame. Use it only when the policy diverges from the
// default (e.g. a provider failure that the operator has flagged as a
// storage transient for retry).
func NewWithBlame(code Code, blame Blame, operation, message string, cause error) *Error {
	return &Error{
		Code:       code,
		Blame:      blame,
		Operation:  operation,
		Message:    message,
		Cause:      cause,
		OccurredAt: time.Now().UTC(),
	}
}

// Error implements the error interface. The Message field is the
// user-safe description; the cause is appended for log readers.
func (e *Error) Error() string {
	if e == nil {
		return "<nil media error>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s", e.Code, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause so errors.Is/As can walk the chain.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CodeOf returns the code of err if it is a *Error, or empty string
// otherwise. It is intended for logging, never for control flow.
func CodeOf(err error) Code {
	var m *Error
	if !errors.As(err, &m) {
		return ""
	}
	return m.Code
}

// IsRetryable reports whether err represents a transient failure the
// caller should retry. Non-Media errors return false; only the
// structured taxonomy knows about retryability.
func IsRetryable(err error) bool {
	var m *Error
	if !errors.As(err, &m) {
		return false
	}
	return m.Code.IsRetryable()
}

// WithFacts fills the Facts envelope with the standard correlation
// identity. It is a convenience used by the reconciler and the
// readiness validator; it returns the receiver so it composes inside
// a single expression. Pass empty strings to leave a field unchanged.
func (e *Error) WithFacts(assetID, derivativeID, storageProfile, taskID, providerAttempt, correlationID string) *Error {
	if e == nil {
		return nil
	}
	if assetID != "" {
		e.Facts.AssetID = assetID
	}
	if derivativeID != "" {
		e.Facts.DerivativeID = derivativeID
	}
	if storageProfile != "" {
		e.Facts.StorageProfile = storageProfile
	}
	if taskID != "" {
		e.Facts.TaskID = taskID
	}
	if providerAttempt != "" {
		e.Facts.ProviderAttempt = providerAttempt
	}
	if correlationID != "" {
		e.Facts.CorrelationID = correlationID
	}
	return e
}
