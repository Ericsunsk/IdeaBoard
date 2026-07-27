package accounts

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestParseFeatureSlugFromParamsJSON(t *testing.T) {
	got := ParseFeatureSlugFromParamsJSON(`{"feature":"style-variation"}`)
	if got != workflow.FeatureSlugStyleVariation {
		t.Fatalf("feature = %q", got)
	}
}
