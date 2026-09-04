package domain

import "errors"

var (
	ErrInvalidCode         = errors.New("invalid reward code")
	ErrCodeCollision       = errors.New("reward Code is not unique")
	ErrRewardCodeConflict  = errors.New("reward Code idempotency key conflicts with another payload")
	ErrCodeUnavailable     = errors.New("reward Code is invalid or unavailable")
	ErrRewardCodeNotFound  = errors.New("reward Code not found")
	ErrCapBelowRedemptions = errors.New("reward Code cap is below its redemption count")
	ErrRewardCodeRedeemed  = errors.New("redeemed reward Code cannot be deleted")
	ErrRedemptionNotFound  = errors.New("reward Redemption not found")
	ErrWalletFrozen        = errors.New("reward wallet is frozen")
)
