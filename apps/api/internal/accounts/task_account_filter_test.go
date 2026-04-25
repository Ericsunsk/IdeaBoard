package accounts

import (
	"testing"

	"api/internal/workflow"
)

func TestFilterAccountsByFeatureKeepsAccountsWithoutExplicitConfig(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: ""},
		{ID: 2, ExtraConfig: `{"timeout":120}`},
	}

	got := filterAccountsByFeature(accounts, workflow.FeatureSlugCanvasExtend)
	if len(got) != 2 {
		t.Fatalf("expected all accounts to remain eligible, got=%d", len(got))
	}
}

func TestFilterAccountsByFeatureSupportsExplicitAllowList(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: `{"features":["canvas-extend","hd-upscale"]}`},
		{ID: 2, ExtraConfig: `{"features":["style-fusion"]}`},
		{ID: 3, ExtraConfig: `{"enabled_features":{"canvas_extend":true,"style-fusion":false}}`},
	}

	got := filterAccountsByFeature(accounts, workflow.FeatureSlugCanvasExtend)
	if len(got) != 2 {
		t.Fatalf("expected 2 eligible accounts, got=%d", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("unexpected account ids: got=[%d,%d], want=[1,3]", got[0].ID, got[1].ID)
	}
}

func TestFilterAccountsByFeatureSupportsExplicitBlockList(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ExtraConfig: `{"disabled_features":["canvas-extend"]}`},
		{ID: 2, ExtraConfig: `{"feature_blacklist":"style-fusion, canvas-extend"}`},
		{ID: 3, ExtraConfig: `{"supported_features":["canvas-extend"],"disabled_features":["style-fusion"]}`},
	}

	got := filterAccountsByFeature(accounts, workflow.FeatureSlugCanvasExtend)
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
		{ID: 2, ExtraConfig: `{"supported_features":{"not-a-feature":true}}`},
	}

	got := filterAccountsByFeature(accounts, workflow.FeatureSlugCanvasExtend)
	if len(got) != 2 {
		t.Fatalf("expected malformed feature config to be ignored, got=%d", len(got))
	}
}

func TestFilterAccountsByFeatureFiltersStabilityPlatformsForIncompatibleFeatures(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Platform: "stability"},
		{ID: 2, Platform: "stable diffusion"},
		{ID: 3, Platform: "Stability AI"},
		{ID: 4, Platform: "openai"},
	}

	testCases := []struct {
		name    string
		feature string
	}{
		{name: "style-fusion", feature: workflow.FeatureSlugStyleFusion},
		{name: "face-swap", feature: workflow.FeatureSlugFaceSwap},
		{name: "fabric-on-body", feature: workflow.FeatureSlugFabricOnBody},
		{name: "multi-angle", feature: workflow.FeatureSlugMultiAngle},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterAccountsByFeature(accounts, tc.feature)
			if len(got) != 1 {
				t.Fatalf("expected only non-stability accounts, got=%d", len(got))
			}
			if got[0].ID != 4 {
				t.Fatalf("unexpected account id: got=%d, want=4", got[0].ID)
			}
		})
	}
}

func TestFilterAccountsByFeatureKeepsStabilityPlatformsForRemoveBg(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Platform: "stability"},
		{ID: 2, Platform: "stable diffusion"},
		{ID: 3, Platform: "Stability AI"},
		{ID: 4, Platform: "openai"},
	}

	got := filterAccountsByFeature(accounts, workflow.FeatureSlugRemoveBg)
	if len(got) != 4 {
		t.Fatalf("expected remove-bg to stay platform-compatible, got=%d", len(got))
	}
}

func TestParseTaskFeatureSlugReturnsEmptyForInvalidJSON(t *testing.T) {
	if got := parseTaskFeatureSlug("{invalid-json"); got != "" {
		t.Fatalf("expected empty feature slug for invalid params json, got=%q", got)
	}
}
