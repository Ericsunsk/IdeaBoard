package httpapi

import "testing"

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
