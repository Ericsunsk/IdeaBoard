package errorrender

import "errors"

// ValidationError is a sentinel-like validation error. Its Error method
// returns the message, so it is safe to expose to API consumers. (Moved here
// from platform/validation so taxonomy, rendering, and the validation
// sentinel share one home; platform/validation re-exports this type.)
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// NewValidationError returns a validation error with the given message.
func NewValidationError(message string) error { return ValidationError(message) }

// IsValidationError reports whether err is a validation error.
func IsValidationError(err error) bool {
	var target ValidationError
	return errors.As(err, &target)
}
