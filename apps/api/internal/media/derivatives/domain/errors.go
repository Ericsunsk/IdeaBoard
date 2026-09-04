package domain

import (
	"errors"
	"time"
)

const MaxAttempts = 5

const (
	MaxSourceBytes = 100 * 1024 * 1024
	MaxOutputBytes = 20 * 1024 * 1024
	RenderDeadline = 2 * time.Minute
)

const (
	RetryDelayInitial = 5 * time.Second
	RetryDelayMax     = 2 * time.Minute
)

var ErrRepairNotPossible = errors.New("media derivative repair is not possible")

type FailureRecordedError struct {
	Err error
}

func (e *FailureRecordedError) Error() string { return e.Err.Error() }

func (e *FailureRecordedError) Unwrap() error { return e.Err }

func NewFailureRecordedError(err error) error {
	return &FailureRecordedError{Err: err}
}

func IsRetryableDerivativeError(code string) bool {
	switch code {
	case "source_read_failed", "render_failed", "destination_write_failed", "integrity_transient":
		return true
	default:
		return false
	}
}

func DerivativeRetryDelay(attemptCount int32) time.Duration {
	if attemptCount <= 1 {
		return RetryDelayInitial
	}
	delay := RetryDelayInitial
	for attempt := int32(1); attempt < attemptCount; attempt++ {
		delay *= 3
		if delay >= RetryDelayMax {
			return RetryDelayMax
		}
	}
	return delay
}
