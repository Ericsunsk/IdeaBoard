package domain

import "errors"

var (
	ErrInvalidCommand             = errors.New("invalid credit command")
	ErrWalletNotFound             = errors.New("credit wallet not found")
	ErrWalletBalanceIntegrity     = errors.New("credit wallet balance projection is incomplete")
	ErrWalletFrozen               = errors.New("credit wallet is frozen")
	ErrWalletRestricted           = errors.New("credit wallet is restricted")
	ErrWalletRestrictionNotFound  = errors.New("credit wallet restriction not found")
	ErrInsufficientCredits        = errors.New("insufficient credits")
	ErrIdempotencyConflict        = errors.New("credit idempotency key conflicts with a different request")
	ErrHoldNotFound               = errors.New("credit hold not found")
	ErrHoldSettled                = errors.New("credit hold is already settled")
	ErrHoldSettlementExceeds      = errors.New("credit hold settlement exceeds remaining amount")
	ErrChargeNotFound             = errors.New("credit charge not found")
	ErrRefundExceedsCharge        = errors.New("credit refund exceeds remaining charge")
	ErrWelcomeIdentityKeyMismatch = errors.New("welcome identity key does not match the permanently bound key")
	ErrReconciliationUnavailable  = errors.New("credit reconciliation unavailable")
	ErrReconciliationNotFound     = errors.New("credit reconciliation run not found")
)
