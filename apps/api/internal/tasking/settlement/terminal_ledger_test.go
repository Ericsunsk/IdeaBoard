package settlement_test

import (
	"errors"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/tasking/settlement"
)

// TestTerminalLedgerConflictPinsDurableFactMapping locks the 409-vs-503
// mapping both terminal seams share: IB006 (hold settled) and IB001
// (idempotency conflict) mean the ledger fact already converged (409),
// everything else stays retryable (503).
func TestTerminalLedgerConflictPinsDurableFactMapping(t *testing.T) {
	if !settlement.IsTerminalLedgerConflict(domain.ErrHoldSettled) {
		t.Fatal("ErrHoldSettled must be a terminal ledger conflict")
	}
	if !settlement.IsTerminalLedgerConflict(domain.ErrIdempotencyConflict) {
		t.Fatal("ErrIdempotencyConflict must be a terminal ledger conflict")
	}
	// Wrapped sentinels (the %w chains both terminal seams build) stay
	// conflicts.
	if !settlement.IsTerminalLedgerConflict(errors.Join(domain.ErrHoldSettled, errors.New("provider noise"))) {
		t.Fatal("wrapped ErrHoldSettled must stay a terminal ledger conflict")
	}
	for name, err := range map[string]error{
		"nil":                nil,
		"unrelated":          errors.New("boom"),
		"hold not found":     domain.ErrHoldNotFound,
		"insufficient funds": domain.ErrInsufficientCredits,
	} {
		if settlement.IsTerminalLedgerConflict(err) {
			t.Fatalf("%s must not be a terminal ledger conflict", name)
		}
	}
}
