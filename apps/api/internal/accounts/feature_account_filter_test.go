package accounts

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestFilterAccountsByFeatureKeepsAccountsWithoutExplicitConfig(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: ""},
		{ID: 2, ExtraConfig: `{"timeout":120}`},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 2 {
		t.Fatalf("expected all accounts to remain eligible, got=%d", len(got))
	}
}

func TestFilterAccountsByFeatureSupportsExplicitAllowList(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: `{"features":["image-extend","hd-upscale"]}`},
		{ID: 2, ExtraConfig: `{"features":["style-fusion"]}`},
		{ID: 3, ExtraConfig: `{"timeout":120}`},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 2 {
		t.Fatalf("expected 2 eligible accounts, got=%d", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("unexpected account ids: got=[%d,%d], want=[1,3]", got[0].ID, got[1].ID)
	}
}

func TestFilterAccountsByFeatureSupportsExplicitBlockList(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: `{"disabled_features":["image-extend"]}`},
		{ID: 2, ExtraConfig: `{"disabled_features":"style-fusion, image-extend"}`},
		{ID: 3, ExtraConfig: `{"features":["image-extend"],"disabled_features":["style-fusion"]}`},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 1 {
		t.Fatalf("expected 1 eligible account, got=%d", len(got))
	}
	if got[0].ID != 3 {
		t.Fatalf("unexpected account id: got=%d, want=3", got[0].ID)
	}
}

func TestFilterAccountsByFeatureIgnoresMalformedFeatureConfig(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: `{"features":123}`},
		{ID: 2, ExtraConfig: `{"features":{"not-a-feature":true}}`},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugImageExtend)
	if len(got) != 2 {
		t.Fatalf("expected malformed feature config to be ignored, got=%d", len(got))
	}
}

func TestFilterAccountsByFeatureKeepsGooglePlatformsForReferenceFeatures(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ApiType: workflow.ProviderGoogleAI},
		{ID: 2, ApiType: workflow.ProviderOpenAICompatible},
	}

	got := FilterAccountsByFeature(accounts, workflow.FeatureSlugStyleFusion)
	if len(got) != 2 {
		t.Fatalf("expected google native platforms to remain compatible, got=%d", len(got))
	}
}

func TestParseFeatureSlugFromParamsJSONReturnsEmptyForInvalidJSON(t *testing.T) {
	if got := ParseFeatureSlugFromParamsJSON("{invalid-json"); got != "" {
		t.Fatalf("expected empty feature slug for invalid params json, got=%q", got)
	}
}
