package accounts

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestFilterAccountsByFeatureKeepsUnrestrictedModels(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "gpt-image-1", Enabled: true}}},
		{ID: 2, Models: []AccountModel{{Name: "imagen-4-preview", Enabled: true}}},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 2 {
		t.Fatalf("expected unrestricted models to remain eligible, got=%d", len(got))
	}
}

func TestFilterAccountsByFeatureUsesModelFeatureSlugs(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "gpt-image-1", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugImageExtend}}}},
		{ID: 2, Models: []AccountModel{{Name: "imagen-4-preview", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugStyleFusion}}}},
		{ID: 3, Models: []AccountModel{{Name: "disabled", Enabled: false, FeatureSlugs: []string{workflow.FeatureSlugImageExtend}}}},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("unexpected account ids: got=%v, want [1]", accountIDsForTest(got))
	}
}

func TestParseFeatureSlugFromParamsJSONReturnsEmptyForInvalidJSON(t *testing.T) {
	if got := ParseFeatureSlugFromParamsJSON("{invalid-json"); got != "" {
		t.Fatalf("expected empty feature slug for invalid params json, got=%q", got)
	}
}
