package domain

import "fmt"

// CheckFundsGate applies the preflight funds decision (#917 parity):
// active restrictions deny first (403 restricted), then sufficiency
// (402 insufficient), matching submit Hold failures. Callers validate
// Wallet existence and lifecycle before invoking this balance check.
//
// Preflight (quote/readiness) calls CheckFundsGate on the resolved
// account Wallet before any persistence. Submit propagates the canonical
// Credits sentinels unchanged, so readiness and submit agree on 402 vs 403.
func CheckFundsGate(available, total int64, restricted bool) error {
	if restricted {
		return ErrWalletRestricted
	}
	if available < total {
		return fmt.Errorf("quote exceeds the resolved account balance: %w", ErrInsufficientCredits)
	}
	return nil
}
