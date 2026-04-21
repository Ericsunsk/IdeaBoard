package service

import "testing"

func TestTemplateValueDescriptions(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "viewpoint uses display label",
			got:  describeViewpointTemplateValue("top_45"),
			want: "45° 侧视",
		},
		{
			name: "lighting uses readable label",
			got:  describeViewpointLightingTemplateValue("hard_light"),
			want: "质感硬光",
		},
		{
			name: "wear tile mode uses readable label",
			got:  describeWearTileModeTemplateValue("seamless"),
			want: "平铺重复",
		},
		{
			name: "unknown values are preserved after trim",
			got:  describeWearTileModeTemplateValue(" custom-mode "),
			want: "custom-mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestParseWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical slug is valid", raw: "style-fusion", want: "style-fusion", ok: true},
		{name: "underscore alias canonicalized", raw: "style_fusion", want: "style-fusion", ok: true},
		{name: "unsupported slug is invalid", raw: "line-draft", want: "", ok: false},
		{name: "invalid chars are rejected", raw: "style fusion", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseWorkflowFeatureSlug(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("raw=%q got=(%q,%v), want=(%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestApplyFeatureSystemPromptUsesAliasKeysAndIgnoresLineDraftOverride(t *testing.T) {
	params := map[string]interface{}{
		"mask_url":           "https://mask.example/a.png",
		"mask_enabled":       true,
		"mode":               "enhancement",
		"line_draft_mode_id": 12,
	}

	got := applyFeatureSystemPrompt(
		"base prompt",
		"user prompt",
		"hd-upscale",
		params,
		func(slug string) (string, error) {
			return "SYS={{user_prompt}}|{{mask_image_url}}|{{use_mask}}|{{upscale_mode}}", nil
		},
	)

	want := "SYS=user prompt|https://mask.example/a.png|true|enhancement"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMaskedRequestDetectionUsesCanonicalAliases(t *testing.T) {
	hdUpscaleParams := map[string]interface{}{
		"mask_url": "https://mask.example/1.png",
	}
	if !isMaskedHDUpscaleRequest(hdUpscaleParams) {
		t.Fatalf("expected hd-upscale masked request to be detected by mask_url alias")
	}

	fabricParams := map[string]interface{}{
		"mask_urls": []interface{}{"https://mask.example/2.png"},
	}
	if !isMaskedFabricOnBodyRequest(fabricParams) {
		t.Fatalf("expected fabric-on-body masked request to be detected by mask_urls alias")
	}
}
