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
		{name: "underscore alias", raw: "style_fusion", want: FeatureSlugStyleFusion, ok: true},
		{name: "mixed case", raw: "  HD_Upscale  ", want: FeatureSlugHDUpscale, ok: true},
		{name: "legacy feature is not in allowlist", raw: "legacy-feature", want: "", ok: false},
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

func TestNormalizeWorkflowFeatureSlugOrFallback(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{name: "returns canonical when valid", raw: "canvas_extend", fallback: "fallback", want: FeatureSlugCanvasExtend},
		{name: "returns fallback when invalid", raw: "not-supported", fallback: FeatureSlugStyleFusion, want: FeatureSlugStyleFusion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeWorkflowFeatureSlugOrFallback(tt.raw, tt.fallback)
			if got != tt.want {
				t.Fatalf("raw=%q fallback=%q got=%q, want=%q", tt.raw, tt.fallback, got, tt.want)
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
		{name: "feature alias key", params: map[string]interface{}{"feature_slug": "STYLE_FUSION"}, want: FeatureSlugStyleFusion, wantErr: false},
		{name: "unsupported feature returns error", params: map[string]interface{}{"feature": "legacy-feature"}, want: "", wantErr: true},
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

func TestIsStabilityTextToImagePlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     bool
	}{
		{name: "stability exact", platform: "stability", want: true},
		{name: "stability ai exact", platform: "Stability AI", want: true},
		{name: "stable diffusion contains", platform: "custom stable diffusion proxy", want: true},
		{name: "openai", platform: "openai", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStabilityTextToImagePlatform(tt.platform); got != tt.want {
				t.Fatalf("platform=%q got=%v, want=%v", tt.platform, got, tt.want)
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
