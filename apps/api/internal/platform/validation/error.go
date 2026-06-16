// Package validation provides a lightweight typed validation error.
//
// Use validation.New(msg) to create errors that callers can
// distinguish from internal/server errors via validation.Is.
package validation

import "errors"

// Error is a sentinel-like validation error. Its Error method
// returns the message, so it is safe to expose to API consumers.
type Error string

func (e Error) Error() string { return string(e) }

// New returns a validation error with the given message.
func New(message string) error { return Error(message) }

// Is reports whether err is a validation error.
func Is(err error) bool {
	var target Error
	return errors.As(err, &target)
}
