package accounts

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestParseFeatureSlugFromParamsJSON(t *testing.T) {
	got := ParseFeatureSlugFromParamsJSON(`{"feature":"variations"}`)
	if got != workflow.FeatureSlugVariations {
		t.Fatalf("feature = %q", got)
	}
}
