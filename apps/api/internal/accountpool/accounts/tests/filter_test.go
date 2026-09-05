package tests

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestParseFeatureSlug(t *testing.T) {
	got := ParseFeatureSlug(`{"feature":"variations"}`)
	if got != workflow.FeatureSlugVariations {
		t.Fatalf("feature = %q", got)
	}
}

func TestFilterByCapability(t *testing.T) {
	accs := []*ApiAccount{
		{ID: 1, Capabilities: []string{"image_gen", "upscale"}},
		{ID: 2, Capabilities: []string{"image_gen"}},
	}
	filtered := FilterByCapability(accs, "upscale")
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("unexpected filtered accounts: %v", filtered)
	}
}

func TestFilterByResolution(t *testing.T) {
	accs := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "m1", Enabled: true, Supports2K: true}}},
		{ID: 2, Models: []AccountModel{{Name: "m2", Enabled: true, Supports2K: false}}},
		nil,
	}
	filtered := FilterByResolution(accs, workflow.OutputResolution2K)
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("unexpected filtered accounts: %v", filtered)
	}
}
