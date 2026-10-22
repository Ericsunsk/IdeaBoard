// Package memberquotas owns the member cross-workspace consumption
// quota (#946): a cumulative cap on one member's settled + pending
// consumption across every Workspace sharing one Billing Account, not a
// wallet, grant, or balance reservation.
//
// A Billing Account holds the unified balance; a Workspace records the
// consumption location and the User records the real operator. The quota
// caps how much one member (Billing Account + User) may jointly consume
// (paid Task generations + Vision Search) per period. There are no
// sub-wallets, no transfers, and no reservations: adjusting the limit
// never clears usage, unused periodic quota never rolls into the next
// period, and joining or rejoining never re-issues (rows and immutable
// settlement history survive membership changes).
//
// Periods: unlimited (no cap, the default), daily / weekly / monthly (UTC
// calendar day, Monday-start week, calendar month) or fixed (lifetime
// cumulative cap). Attribution locks to the submit-time period (task
// snapshot quoted_at / vision operation created_at); linked consumption
// refunds correct the original period, so a cross-period refund returns to
// the account balance without widening the current-period quota. Actual
// settlement never exceeds the pre-authorized Hold (the Hold is the cap),
// so terminal Capture/Release needs no re-check and in-flight Holds
// converge on their submit-time facts.
package memberquotas

import (
	"errors"
	"time"
)

const (
	// PeriodDaily caps consumption per UTC calendar day.
	PeriodDaily = "daily"
	// PeriodWeekly caps consumption per UTC week starting Monday.
	PeriodWeekly = "weekly"
	// PeriodMonthly caps consumption per UTC calendar month.
	PeriodMonthly = "monthly"
	// PeriodFixed caps lifetime cumulative consumption.
	PeriodFixed = "fixed"
	// PeriodUnlimited disables the cap (the default).
	PeriodUnlimited = "unlimited"

	// MaxLimitCredits bounds a quota cap to the Credits per-command
	// ceiling (9_000_000_000_000_000 indivisible integer units).
	MaxLimitCredits = int64(9000000000000000)

	// MaxListQuotas bounds one account quota listing.
	MaxListQuotas = 500
)

var (
	// ErrQuotaExhausted reports that a consumption request would exceed
	// the member's remaining quota for the current period. Callers
	// render it distinctly from balance insufficiency (402) and other
	// funds restrictions (403 billing_wallet_restricted).
	ErrQuotaExhausted = errors.New("memberquotas: member quota exhausted")
	// ErrQuotaInvalid reports a malformed quota set request (unknown
	// period, non-positive capped limit, over-ceiling limit, or a limit
	// carried on unlimited).
	ErrQuotaInvalid = errors.New("memberquotas: member quota is invalid")
	// ErrQuotaNotFound reports a hidden workspace/account or a caller
	// without membership. Handlers render workspace_not_found (never a
	// quota status), per anti-enumeration.
	ErrQuotaNotFound = errors.New("memberquotas: member quota is not visible")
)

// IsUnlimited reports whether no cap applies (absent row or unlimited
// period). Unlimited quotas still report usage; they never exhaust.
func IsUnlimited(period string, limitCredits *int64) bool {
	if period == "" || period == PeriodUnlimited {
		return true
	}
	return limitCredits == nil || *limitCredits <= 0
}

// ValidateSet fails closed on malformed set input: period must be one of
// the five quota words; every capped period requires 1..MaxLimitCredits;
// unlimited requires no limit.
func ValidateSet(period string, limitCredits *int64) error {
	switch period {
	case PeriodDaily, PeriodWeekly, PeriodMonthly, PeriodFixed:
		if limitCredits == nil || *limitCredits <= 0 || *limitCredits > MaxLimitCredits {
			return ErrQuotaInvalid
		}
		return nil
	case PeriodUnlimited, "":
		if limitCredits != nil {
			return ErrQuotaInvalid
		}
		return nil
	default:
		return ErrQuotaInvalid
	}
}

// DayStartUTC returns the UTC calendar-day start containing t.
func DayStartUTC(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// DayEndUTC returns the exclusive UTC calendar-day end after start.
func DayEndUTC(start time.Time) time.Time {
	return start.UTC().Add(24 * time.Hour)
}

// WeekStartUTC returns the UTC Monday 00:00 starting the week containing
// t. Weeks start Monday and the account timezone is fixed UTC.
func WeekStartUTC(t time.Time) time.Time {
	day := DayStartUTC(t)
	// time.Monday is 1; Sunday (0) belongs to the week starting six days earlier.
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

// WeekEndUTC returns the exclusive UTC week end after start.
func WeekEndUTC(start time.Time) time.Time {
	return start.UTC().AddDate(0, 0, 7)
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

// CurrentDayBounds returns the [start, end) UTC bounds of the day
// containing now.
func CurrentDayBounds(now time.Time) (time.Time, time.Time) {
	start := DayStartUTC(now)
	return start, DayEndUTC(start)
}

// CurrentWeekBounds returns the [start, end) UTC bounds of the
// Monday-start week containing now.
func CurrentWeekBounds(now time.Time) (time.Time, time.Time) {
	start := WeekStartUTC(now)
	return start, WeekEndUTC(start)
}

// CurrentMonthBounds returns the [start, end) UTC bounds of the month
// containing now.
func CurrentMonthBounds(now time.Time) (time.Time, time.Time) {
	start := MonthStartUTC(now)
	return start, MonthEndUTC(start)
}

// CurrentBounds returns the [start, end) UTC window for a capped period
// at now. Fixed quotas are unbounded: both bounds are nil (all time).
// Unlimited returns nil bounds and ok=false.
func CurrentBounds(period string, now time.Time) (start, end *time.Time, ok bool) {
	var s, e time.Time
	switch period {
	case PeriodDaily:
		s, e = CurrentDayBounds(now)
	case PeriodWeekly:
		s, e = CurrentWeekBounds(now)
	case PeriodMonthly:
		s, e = CurrentMonthBounds(now)
	case PeriodFixed:
		return nil, nil, true
	default:
		return nil, nil, false
	}
	return &s, &e, true
}
