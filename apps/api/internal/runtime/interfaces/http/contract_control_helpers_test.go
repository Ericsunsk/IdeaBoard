package httpapi

import "testing"

// Shared control helper semantics: body-carried values win, trimmed;
// blanks fall back without changing wire codes.
func TestControlReasonOr(t *testing.T) {
	ctx := controlContext{Reason: "header-reason"}
	cases := []struct {
		name   string
		reason string
		want   string
	}{
		{"body wins", "body-reason", "body-reason"},
		{"body trimmed", "  body-reason  ", "body-reason"},
		{"blank falls back", "", "header-reason"},
		{"whitespace falls back", "   ", "header-reason"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := controlReasonOr(ctx, tc.reason); got != tc.want {
				t.Fatalf("controlReasonOr(%q) = %q, want %q", tc.reason, got, tc.want)
			}
		})
	}
	if got := controlReasonOr(controlContext{}, ""); got != "" {
		t.Fatalf("controlReasonOr empty = %q, want empty", got)
	}
}

func TestNonEmptyControlMessage(t *testing.T) {
	if got := nonEmptyControlMessage("  boom  ", "fallback"); got != "boom" {
		t.Fatalf("got %q, want boom", got)
	}
	for _, blank := range []string{"", "   "} {
		if got := nonEmptyControlMessage(blank, "fallback"); got != "fallback" {
			t.Fatalf("blank %q got %q, want fallback", blank, got)
		}
	}
}
