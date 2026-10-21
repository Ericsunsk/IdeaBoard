package workspacebudgets

import (
	"testing"
	"time"
)

func TestMonthBoundsUTC(t *testing.T) {
	// A mid-month afternoon in UTC stays in its calendar month.
	start, end := CurrentMonthBounds(time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC))
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("period start = %v, want %v", start, want)
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("period end = %v, want %v", end, want)
	}
	// A non-UTC zone still resolves to UTC calendar bounds.
	in := time.Date(2026, 10, 21, 23, 30, 0, 0, time.FixedZone("UTC+8", 8*3600))
	start, end = CurrentMonthBounds(in)
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("zoned period start = %v, want %v", start, want)
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("zoned period end = %v, want %v", end, want)
	}
	// December rolls into January of the next year.
	start, end = CurrentMonthBounds(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("december start = %v, want %v", start, want)
	}
	if want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("december end = %v, want %v", end, want)
	}
}

func TestValidateSet(t *testing.T) {
	limit := func(v int64) *int64 { return &v }
	cases := []struct {
		name   string
		period string
		limit  *int64
		wantOK bool
	}{
		{"monthly with limit", PeriodMonthly, limit(100), true},
		{"monthly at ceiling", PeriodMonthly, limit(MaxLimitCredits), true},
		{"monthly zero", PeriodMonthly, limit(0), false},
		{"monthly negative", PeriodMonthly, limit(-5), false},
		{"monthly missing", PeriodMonthly, nil, false},
		{"monthly over ceiling", PeriodMonthly, limit(MaxLimitCredits + 1), false},
		{"unlimited clean", PeriodUnlimited, nil, true},
		{"unlimited with limit", PeriodUnlimited, limit(100), false},
		{"empty period clean", "", nil, true},
		{"unknown period", "weekly", nil, false},
		{"unknown period with limit", "weekly", limit(100), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSet(tc.period, tc.limit)
			if tc.wantOK && err != nil {
				t.Fatalf("ValidateSet(%q) error = %v, want nil", tc.period, err)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("ValidateSet(%q) = nil, want error", tc.period)
			}
		})
	}
}

func TestIsUnlimited(t *testing.T) {
	limit := func(v int64) *int64 { return &v }
	if !IsUnlimited("", nil) {
		t.Fatal("empty period must read unlimited")
	}
	if !IsUnlimited(PeriodUnlimited, nil) {
		t.Fatal("unlimited period must read unlimited")
	}
	if !IsUnlimited(PeriodUnlimited, limit(100)) {
		t.Fatal("unlimited period with a stale limit must still read unlimited")
	}
	if IsUnlimited(PeriodMonthly, limit(100)) {
		t.Fatal("monthly with a limit must not read unlimited")
	}
	if !IsUnlimited(PeriodMonthly, nil) {
		t.Fatal("monthly without a limit must fail safe to unlimited reads")
	}
}
