// Package workspacebudgets owns the Workspace shared consumption budget
// (#945): a cumulative cap on settled + pending consumption in the current
// UTC period, not a wallet, grant, or balance reservation.
//
// A Billing Account holds the unified balance; a Workspace records the
// consumption location. The budget caps how much one Workspace's members
// may jointly consume (paid Task generations + Vision Search) per period.
// There are no sub-wallets, no transfers, and no reservations: adjusting
// the limit never clears usage, and a monthly reset starts a new UTC
// calendar month without deleting history.
//
// Periods: monthly (UTC calendar month) or unlimited (no cap, the
// default). Attribution locks to the submit-time period (task snapshot
// quoted_at / vision operation created_at); linked consumption refunds
// correct the original period, so a cross-period refund never grants
// current-period quota. Actual settlement never exceeds the pre-authorized
// Hold (the Hold is the cap), so terminal Capture/Release needs no
// re-check and in-flight Holds converge on their submit-time facts.
package workspacebudgets

import (
	"errors"
	"time"
)

const (
	// PeriodMonthly caps consumption per UTC calendar month.
	PeriodMonthly = "monthly"
	// PeriodUnlimited disables the cap (the default).
	PeriodUnlimited = "unlimited"

	// MaxLimitCredits bounds a monthly cap to the Credits per-command
	// ceiling (9_000_000_000_000_000 indivisible integer units).
	MaxLimitCredits = int64(9000000000000000)
)

var (
	// ErrBudgetExhausted reports that a consumption request would exceed
	// the Workspace's remaining budget for the current period. Callers
	// render it distinctly from balance insufficiency (402) and other
	// funds restrictions (403 billing_wallet_restricted).
	ErrBudgetExhausted = errors.New("workspacebudgets: workspace budget exhausted")
	// ErrBudgetInvalid reports a malformed budget set request (unknown
	// period, non-positive monthly limit, over-ceiling limit).
	ErrBudgetInvalid = errors.New("workspacebudgets: workspace budget is invalid")
	// ErrBudgetNotFound reports a hidden workspace or a caller without
	// membership. Handlers render workspace_not_found (never a budget
	// status), per anti-enumeration.
	ErrBudgetNotFound = errors.New("workspacebudgets: workspace is not visible")
)

// IsUnlimited reports whether no cap applies (absent row or unlimited
// period). Unlimited budgets still report usage; they never exhaust.
func IsUnlimited(period string, limitCredits *int64) bool {
	if period == "" || period == PeriodUnlimited {
		return true
	}
	return limitCredits == nil || *limitCredits <= 0
}

// ValidateSet fails closed on malformed set input: period must be monthly
// or unlimited; monthly requires 1..MaxLimitCredits; unlimited requires
// no limit.
func ValidateSet(period string, limitCredits *int64) error {
	switch period {
	case PeriodMonthly:
		if limitCredits == nil || *limitCredits <= 0 || *limitCredits > MaxLimitCredits {
			return ErrBudgetInvalid
		}
		return nil
	case PeriodUnlimited, "":
		if limitCredits != nil {
			return ErrBudgetInvalid
		}
		return nil
	default:
		return ErrBudgetInvalid
	}
}

// MonthStartUTC returns the UTC calendar-month start containing t.
func MonthStartUTC(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// MonthEndUTC returns the exclusive UTC calendar-month end after start.
func MonthEndUTC(start time.Time) time.Time {
	utc := start.UTC()
	return time.Date(utc.Year(), utc.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// CurrentMonthBounds returns the [start, end) UTC bounds of the month
// containing now.
func CurrentMonthBounds(now time.Time) (time.Time, time.Time) {
	start := MonthStartUTC(now)
	return start, MonthEndUTC(start)
}
