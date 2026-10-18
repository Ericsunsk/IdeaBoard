// Shared application error taxonomy for the API process: every user-facing failure carries a stable machine code, the
// HTTP status its adapters must render, and a retryability verdict for queue
// runners.
//
// Rules:
//   - Code is wire-stable. Do not rename a code without coordinating the
//     frontend and the runbook that consume it.
//   - The user-facing message lives on the sentinel. Adapters render
//     err.Error() (or a fixed safe message where noted) and must never
//     interpolate provider payloads or secrets.
//   - Retryable answers "would retrying the same job help". Validation and
//     terminal state conflicts are never retryable; transient provider and
//     platform outages are.
//   - Keep the taxonomy flat: one Code per stable outcome. Blame only routes
//     future alerting and stays out of response rendering.
package errorrender

import (
	"errors"
	"net/http"
)

// Code is the stable machine identifier of a failure.
type Code string

// Blame classifies who is responsible for a failure. It informs alerting
// and retry policy; adapters must not branch responses on it.
type Blame string

const (
	BlameCaller   Blame = "caller"
	BlameProvider Blame = "provider"
	BlamePlatform Blame = "platform"
)

// Error is a classified application failure. The zero value is invalid;
// always build through New or one of the status helpers.
type Error struct {
	code      Code
	msg       string
	status    int
	blame     Blame
	retryable bool
	cause     error
}

// New builds a classified error with an explicit HTTP status.
func New(code Code, status int, blame Blame, msg string) *Error {
	return &Error{code: code, msg: msg, status: status, blame: blame}
}

// Wrap attaches a cause to a classified error, preserving the taxonomy while
// keeping the underlying failure inspectable via errors.Is/As.
func Wrap(code Code, status int, blame Blame, cause error, msg string) *Error {
	return &Error{code: code, msg: msg, status: status, blame: blame, cause: cause}
}

// WithRetryable returns a copy marked with the retry verdict. Validation and
// terminal state conflicts stay non-retryable; transient outages opt in.
func (e *Error) WithRetryable(retryable bool) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.retryable = retryable
	return &clone
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Code returns the stable machine code, or "" when err carries no taxonomy.
func (e *Error) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}

// HTTPStatus returns the status adapters must render, or 0 when err carries
// no taxonomy. Callers decide their own default for unclassified errors.
func (e *Error) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.status
}

// Blame returns who is responsible for the failure.
func (e *Error) Blame() Blame {
	if e == nil {
		return ""
	}
	return e.blame
}

// Retryable reports whether retrying the same job could succeed.
func (e *Error) Retryable() bool {
	return e != nil && e.retryable
}

// Status helpers for the common outcomes. Codes and messages stay with the
// domain sentinel declarations; these only fix the wire status.

func BadRequest(code Code, msg string) *Error {
	return New(code, http.StatusBadRequest, BlameCaller, msg)
}

func Conflict(code Code, msg string) *Error {
	return New(code, http.StatusConflict, BlameCaller, msg)
}

func ServiceUnavailable(code Code, blame Blame, msg string) *Error {
	return New(code, http.StatusServiceUnavailable, blame, msg)
}

// As extracts the outermost classified error, traversing errors.Join trees
// and %w chains. It reports false when err carries no taxonomy.
func As(err error) (*Error, bool) {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// CodeOf returns the outermost taxonomy code, or "" for unclassified errors.
func CodeOf(err error) Code {
	if appErr, ok := As(err); ok {
		return appErr.Code()
	}
	return ""
}

// HTTPStatusOf returns the outermost taxonomy status, or 0 for unclassified
// errors. Callers apply their own endpoint default when it returns 0.
func HTTPStatusOf(err error) int {
	if appErr, ok := As(err); ok {
		return appErr.HTTPStatus()
	}
	return 0
}

// RetryableOf reports the outermost taxonomy verdict. Unclassified errors
// (database and credit failures included) default to retryable, matching the
// reconciliation runner's fail-closed-toward-retry policy.
func RetryableOf(err error) bool {
	if appErr, ok := As(err); ok {
		return appErr.Retryable()
	}
	return true
}
