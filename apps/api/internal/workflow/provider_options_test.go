package workflow

import (
	"testing"
)

func TestParseProviderOptions(t *testing.T) {
	if got, err := ParseProviderOptions(""); err != nil || got != nil {
		t.Fatalf("ParseProviderOptions(empty) = %v, %v; want nil, nil", got, err)
	}
	if got, err := ParseProviderOptions("{}"); err != nil || got != nil {
		t.Fatalf("ParseProviderOptions({}) = %v, %v; want nil, nil", got, err)
	}
	// Non-object JSON is ignored (never an error): same rule as execution.
	for _, raw := range []string{"[1,2]", `"str"`, "42", "null"} {
		if got, err := ParseProviderOptions(raw); err != nil || got != nil {
			t.Fatalf("ParseProviderOptions(%q) = %v, %v; want nil, nil", raw, got, err)
		}
	}
	got, err := ParseProviderOptions(`{"quality":"high"}`)
	if err != nil || got["quality"] != "high" {
		t.Fatalf("ParseProviderOptions(object) = %v, %v; want map, nil", got, err)
	}
	if _, err := ParseProviderOptions("{invalid"); err == nil {
		t.Fatal("ParseProviderOptions(malformed) = nil error, want fail-closed")
	}
}
