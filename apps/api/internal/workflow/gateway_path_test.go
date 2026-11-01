package workflow

import (
	"testing"
)

func TestFormatPoolModel(t *testing.T) {
	if got := FormatPoolModel("OpenAI", "gpt-image-1"); got != "openai/gpt-image-1" {
		t.Fatalf("FormatPoolModel = %q, want openai/gpt-image-1", got)
	}
	// Nested-slash preservation is covered once by TestSplitPoolDisplay.
}

func TestSplitPoolDisplay(t *testing.T) {
	provider, model, err := SplitPoolDisplay("openai/gpt-image-1")
	if err != nil || provider != "openai" || model != "gpt-image-1" {
		t.Fatalf("SplitPoolDisplay = %q/%q, %v; want openai/gpt-image-1, nil", provider, model, err)
	}
	// Prefix/bare separation keeps nested vendor segments in the bare name.
	provider, model, err = SplitPoolDisplay("openrouter/google/gemini-3-flash")
	if err != nil || provider != "openrouter" || model != "google/gemini-3-flash" {
		t.Fatalf("SplitPoolDisplay nested = %q/%q, %v; want openrouter/google/gemini-3-flash, nil", provider, model, err)
	}
	// Bare models without a provider fail closed: execution must never
	// infer a provider from a prefix-less string. One representative per
	// reject branch (missing slash, empty segment, whitespace); the rest
	// share the same fail-closed path.
	for _, bare := range []string{"gpt-image-1", "openai/", "a b/c"} {
		if _, _, err := SplitPoolDisplay(bare); err == nil {
			t.Fatalf("SplitPoolDisplay(%q) = nil error, want fail-closed", bare)
		}
	}
}

func TestParseExecutionModelRef(t *testing.T) {
	provider, model, err := ParseExecutionModelRef("openai/dall-e-3")
	if err != nil || provider != "openai" || model != "dall-e-3" {
		t.Fatalf("ParseExecutionModelRef = %q/%q, %v; want openai/dall-e-3, nil", provider, model, err)
	}
	// Unknown slugs stay open (custom-<slug> namespace downstream), and
	// bare models without a provider fail closed. Whitespace rejection is
	// covered once by TestSplitPoolDisplay.
	if _, _, err := ParseExecutionModelRef("dall-e-3"); err == nil {
		t.Fatal("ParseExecutionModelRef(bare) = nil error, want fail-closed")
	}
	// The legacy @path override has no Worker target: fail closed.
	if _, _, err := ParseExecutionModelRef("myco/custom-model@v1/custom"); err == nil {
		t.Fatal("ParseExecutionModelRef(@path) = nil error, want fail-closed")
	}
}
