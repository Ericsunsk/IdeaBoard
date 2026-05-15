package workflow

import "testing"

func TestParseWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical slug", raw: FeatureSlugStyleFusion, want: FeatureSlugStyleFusion, ok: true},
		{name: "underscore slug is rejected", raw: "style_fusion", want: "", ok: false},
		{name: "mixed case is rejected", raw: "  HD_Upscale  ", want: "", ok: false},
		{name: "unknown feature is not in allowlist", raw: "unknown-feature", want: "", ok: false},
		{name: "unsupported slug", raw: "line-draft", want: "", ok: false},
		{name: "invalid characters", raw: "style fusion", want: "", ok: false},
		{name: "empty", raw: "", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseWorkflowFeatureSlug(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("raw=%q got=(%q,%v), want=(%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseWorkflowFeatureSlugSupportsCanonicalAllowlist(t *testing.T) {
	canonical := []string{
		FeatureSlugSketchToDesign,
		FeatureSlugStyleInnovate,
		FeatureSlugPartialEdit,
		FeatureSlugBgSwap,
		FeatureSlugRemoveBg,
		FeatureSlugFaceSwap,
		FeatureSlugStyleFusion,
		FeatureSlugSeriesColor,
		FeatureSlugDirectedRecolor,
		FeatureSlugFabricOnBody,
		FeatureSlugMultiAngle,
		FeatureSlugVirtual3D,
		FeatureSlugPatternGen,
		FeatureSlugHDUpscale,
		FeatureSlugCanvasExtend,
	}

	for _, slug := range canonical {
		t.Run(slug, func(t *testing.T) {
			got, ok := ParseWorkflowFeatureSlug(slug)
			if !ok || got != slug {
				t.Fatalf("slug=%q got=(%q,%v), want=(%q,true)", slug, got, ok, slug)
			}
		})
	}
}

func TestResolveWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]interface{}
		want    string
		wantErr bool
	}{
		{name: "missing feature returns empty", params: map[string]interface{}{}, want: "", wantErr: false},
		{name: "canonical feature key", params: map[string]interface{}{"feature": FeatureSlugStyleFusion}, want: FeatureSlugStyleFusion, wantErr: false},
		{name: "unsupported feature returns error", params: map[string]interface{}{"feature": "unknown-feature"}, want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveWorkflowFeatureSlug(tt.params)
			if (err != nil) != tt.wantErr {
				t.Fatalf("params=%v err=%v, wantErr=%v", tt.params, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("params=%v got=%q, want=%q", tt.params, got, tt.want)
			}
		})
	}
}

func TestValidateProviderAPIType(t *testing.T) {
	tests := []struct {
		name    string
		apiType string
		baseURL string
		want    string
		wantOK  bool
	}{
		{name: "openai compatible", apiType: ProviderOpenAICompatible, want: ProviderOpenAICompatible, wantOK: true},
		{name: "google ai", apiType: ProviderGoogleAI, want: ProviderGoogleAI, wantOK: true},
		{name: "infer google", baseURL: "https://generativelanguage.googleapis.com", want: ProviderGoogleAI, wantOK: true},
		{name: "infer openai compatible", baseURL: "https://api.example.com/v1", want: ProviderOpenAICompatible, wantOK: true},
		{name: "reject empty without base url", wantOK: false},
		{name: "reject unsupported explicit type", apiType: "unsupported-provider", baseURL: "https://api.example.com/v1", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ValidateProviderAPIType(tt.apiType, tt.baseURL)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsReferenceImageFeature(t *testing.T) {
	tests := []struct {
		feature string
		want    bool
	}{
		{feature: FeatureSlugStyleFusion, want: true},
		{feature: FeatureSlugFaceSwap, want: true},
		{feature: FeatureSlugFabricOnBody, want: true},
		{feature: FeatureSlugMultiAngle, want: true},
		{feature: FeatureSlugRemoveBg, want: false},
		{feature: FeatureSlugCanvasExtend, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			if got := IsReferenceImageFeature(tt.feature); got != tt.want {
				t.Fatalf("feature=%q got=%v, want=%v", tt.feature, got, tt.want)
			}
		})
	}
}
