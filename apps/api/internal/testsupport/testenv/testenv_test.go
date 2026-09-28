package testenv

import (
	"testing"
)

func TestGetTrimsWhitespace(t *testing.T) {
	t.Setenv("IDEABOARD_TESTENV_GET", "  value  ")
	if got := Get("IDEABOARD_TESTENV_GET"); got != "value" {
		t.Fatalf("Get() = %q, want %q", got, "value")
	}
	if got := Get("IDEABOARD_TESTENV_UNSET"); got != "" {
		t.Fatalf("Get() = %q, want empty", got)
	}
}

func TestRequireReturnsValue(t *testing.T) {
	t.Setenv("IDEABOARD_TESTENV_REQUIRE", "present")
	if got := Require(t, "IDEABOARD_TESTENV_REQUIRE"); got != "present" {
		t.Fatalf("Require() = %q, want %q", got, "present")
	}
}

func TestGetOrSkipReturnsValue(t *testing.T) {
	t.Setenv("IDEABOARD_TESTENV_SKIP", "present")
	if got := GetOrSkip(t, "IDEABOARD_TESTENV_SKIP"); got != "present" {
		t.Fatalf("GetOrSkip() = %q, want %q", got, "present")
	}
}
