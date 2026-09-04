package domain

import "errors"

var (
	ErrUserNotFound                      = errors.New("user not found")
	ErrUserDeletionInProgress            = errors.New("user deletion is already in progress")
	ErrUserDeletionBlockedByPaymentState = errors.New("user deletion is blocked by unsettled payment state")
	ErrUserDeletionAuthUnavailable       = errors.New("user deletion Auth deleter is not configured")
	ErrUserDeletionStorageUnavailable    = errors.New("user deletion storage cleaner is not configured")
	ErrInvalidStorageUID                 = errors.New("invalid storage UID")
)
