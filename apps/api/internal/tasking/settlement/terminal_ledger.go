package settlement

import (
	"errors"

	"ideaboard/api/internal/credits/domain"
)

// IsTerminalLedgerConflict reports whether err is a ledger rejection that
// landed after the terminal fact already converged.
//
// Same-key replays already return Replayed with no error, so any rejection
// at this point means the fact is durable. Both classes below converge to
// the same caller semantic:
//
//   - IB006 (domain.ErrHoldSettled: hold already settled / settlement
//     command already recorded).
//   - IB001 (domain.ErrIdempotencyConflict: same key, different amount or
//     BusinessReference).
//
// The task must then surface its state-conflict sentinel (409), never a
// retryable 503 that would roll the settlement back and re-enter the
// provider. Callers keep their own wrapping (message prefix + sentinel);
// this predicate is the single truth for the 409-vs-503 mapping, so a new
// durable-fact class is added here once instead of at every terminal seam.
func IsTerminalLedgerConflict(err error) bool {
	return errors.Is(err, domain.ErrHoldSettled) ||
		errors.Is(err, domain.ErrIdempotencyConflict)
}
