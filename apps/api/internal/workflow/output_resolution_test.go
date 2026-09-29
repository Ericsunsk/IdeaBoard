package workflow

import "testing"

func TestNormalizeOutputResolution(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: "", ok: false},
		{raw: "  ", want: "", ok: false},
		{raw: "default", ok: false},
		{raw: " 2K ", want: OutputResolution2K, ok: true},
		{raw: "4k", want: OutputResolution4K, ok: true},
		{raw: "8k", ok: false},
	}

	for _, test := range tests {
		got, ok := NormalizeOutputResolution(test.raw)
		if got != test.want || ok != test.ok {
			t.Fatalf("NormalizeOutputResolution(%q) = (%q, %v), want (%q, %v)", test.raw, got, ok, test.want, test.ok)
		}
	}
}

func TestCompileOutputResolutionOmittedUsesPolicyDefault(t *testing.T) {
	for _, feature := range []string{FeatureSlugRender, FeatureSlugCutout, FeatureSlugUpscale} {
		definition := imageFeatureExecutionDefinitions[feature]
		for _, raw := range []*string{nil, ptr("")} {
			got, err := compileOutputResolution(feature, definition.outputResolutionPolicy, raw)
			if err != nil || got != OutputResolution1K {
				t.Fatalf("compileOutputResolution(%s, omitted) = (%q, %v), want (1k, nil)", feature, got, err)
			}
		}
	}
}

func TestCompileOutputResolutionRejectsDefault(t *testing.T) {
	for _, feature := range []string{FeatureSlugRender, FeatureSlugCutout} {
		definition := imageFeatureExecutionDefinitions[feature]
		if _, err := compileOutputResolution(feature, definition.outputResolutionPolicy, ptr("default")); err == nil {
			t.Fatalf("compileOutputResolution(%s, default) must fail", feature)
		}
	}
}

func ptr(s string) *string { return &s }
