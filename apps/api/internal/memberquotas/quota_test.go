package memberquotas

import (
	"testing"
	"time"
)

func TestValidateSetAcceptsFivePeriods(t *testing.T) {
	limit := int64(100)
	for _, period := range []string{PeriodDaily, PeriodWeekly, PeriodMonthly, PeriodFixed} {
		if err := ValidateSet(period, &limit); err != nil {
			t.Fatalf("ValidateSet(%q) = %v, want nil", period, err)
		}
	}
	if err := ValidateSet(PeriodUnlimited, nil); err != nil {
		t.Fatalf("ValidateSet(unlimited) = %v, want nil", err)
	}
	if err := ValidateSet("", nil); err != nil {
		t.Fatalf("ValidateSet(empty) = %v, want nil", err)
	}
}

func TestValidateSetRejectsMalformed(t *testing.T) {
	limit := int64(100)
	zero := int64(0)
	over := MaxLimitCredits + 1
	for _, tc := range []struct {
		name   string
		period string
		limit  *int64
	}{
		{"daily without limit", PeriodDaily, nil},
		{"weekly zero", PeriodWeekly, &zero},
		{"monthly over ceiling", PeriodMonthly, &over},
		{"fixed without limit", PeriodFixed, nil},
		{"unlimited with limit", PeriodUnlimited, &limit},
		{"unknown period", "yearly", &limit},
		{"unknown period bare", "weekly ", &limit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSet(tc.period, tc.limit); err == nil {
				t.Fatalf("ValidateSet(%q) = nil, want ErrQuotaInvalid", tc.period)
			}
		})
	}
}

func TestIsUnlimited(t *testing.T) {
	limit := int64(100)
	if !IsUnlimited("", nil) || !IsUnlimited(PeriodUnlimited, nil) || !IsUnlimited(PeriodUnlimited, &limit) {
		t.Fatal("empty/unlimited must read as unlimited")
	}
	if !IsUnlimited(PeriodDaily, nil) {
		t.Fatal("capped period without a limit must read as unlimited")
	}
	if IsUnlimited(PeriodDaily, &limit) || IsUnlimited(PeriodFixed, &limit) {
		t.Fatal("capped period with a limit must not read as unlimited")
	}
}

func TestDayBoundsUTC(t *testing.T) {
	now := time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC)
	start, end := CurrentDayBounds(now)
	wantStart := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("day bounds = (%v, %v), want (%v, %v)", start, end, wantStart, wantEnd)
	}
}

func TestWeekBoundsStartMonday(t *testing.T) {
	// 2026-10-21 is a Wednesday; the Monday-start week opens 2026-10-19.
	wednesday := time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC)
	start, end := CurrentWeekBounds(wednesday)
	wantStart := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("week bounds = (%v, %v), want (%v, %v)", start, end, wantStart, wantEnd)
	}
	// Sunday still belongs to the week starting the previous Monday.
	sunday := time.Date(2026, 10, 25, 23, 59, 59, 0, time.UTC)
	sundayStart, _ := CurrentWeekBounds(sunday)
	if !sundayStart.Equal(wantStart) {
		t.Fatalf("sunday week start = %v, want %v", sundayStart, wantStart)
	}
	// Monday opens a new week.
	monday := time.Date(2026, 10, 26, 0, 0, 1, 0, time.UTC)
	mondayStart, _ := CurrentWeekBounds(monday)
	if !mondayStart.Equal(wantEnd) {
		t.Fatalf("monday week start = %v, want %v", mondayStart, wantEnd)
	}
}

func TestMonthBoundsUTC(t *testing.T) {
	now := time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC)
	start, end := CurrentMonthBounds(now)
	wantStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("month bounds = (%v, %v), want (%v, %v)", start, end, wantStart, wantEnd)
	}
}

func TestCurrentBoundsWindows(t *testing.T) {
	now := time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC)
	for _, period := range []string{PeriodDaily, PeriodWeekly, PeriodMonthly} {
		start, end, ok := CurrentBounds(period, now)
		if !ok || start == nil || end == nil {
			t.Fatalf("CurrentBounds(%q) = (%v, %v, %v), want a bounded window", period, start, end, ok)
		}
		if !(!start.After(now) && now.Before(*end)) {
			t.Fatalf("CurrentBounds(%q) window (%v, %v) does not contain now", period, start, end)
		}
	}
	// Fixed quotas are unbounded (all time); unlimited carries no window.
	if start, end, ok := CurrentBounds(PeriodFixed, now); !ok || start != nil || end != nil {
		t.Fatalf("CurrentBounds(fixed) = (%v, %v, %v), want (nil, nil, true)", start, end, ok)
	}
	if _, _, ok := CurrentBounds(PeriodUnlimited, now); ok {
		t.Fatal("CurrentBounds(unlimited) = true, want false")
	}
	if _, _, ok := CurrentBounds("yearly", now); ok {
		t.Fatal("CurrentBounds(yearly) = true, want false")
	}
}
