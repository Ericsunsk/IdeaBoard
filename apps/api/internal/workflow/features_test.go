package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"testing"
)

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
		FeatureSlugSketchToImage,
		FeatureSlugStyleVariation,
		FeatureSlugPartialEdit,
		FeatureSlugRemoveBg,
		FeatureSlugStyleFusion,
		FeatureSlugPatternGen,
		FeatureSlugHDUpscale,
		FeatureSlugImageExtend,
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

func TestSupportedFeatureSlugsMatchFrontendWorkflowFeatures(t *testing.T) {
	repoRoot := findWorkflowRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot, "apps", "web", "src", "shared", "domain", "workflowFeatures.ts"))
	if err != nil {
		t.Fatalf("read frontend workflow features: %v", err)
	}

	frontendSlugs := extractFrontendWorkflowFeatureSlugs(t, string(raw))
	backendSlugs := SupportedFeatureSlugs()
	slices.Sort(frontendSlugs)
	slices.Sort(backendSlugs)

	if !slices.Equal(backendSlugs, frontendSlugs) {
		t.Fatalf("workflow feature slugs mismatch\nbackend=%v\nfrontend=%v", backendSlugs, frontendSlugs)
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

func findWorkflowRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)
	for {
		if fileExists(filepath.Join(dir, "apps", "api", "internal", "workflow", "features.go")) &&
			fileExists(filepath.Join(dir, "apps", "web", "src", "shared", "domain", "workflowFeatures.ts")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", filename)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func extractFrontendWorkflowFeatureSlugs(t *testing.T, source string) []string {
	t.Helper()
	if !regexp.MustCompile(`export\s+const\s+WORKFLOW_FEATURE_SLUGS\s*=\s*Object\.values\(WORKFLOW_FEATURE\)\s+as\s+readonly\s+WorkflowFeature\[\]`).MatchString(source) {
		t.Fatal("WORKFLOW_FEATURE_SLUGS must be derived from Object.values(WORKFLOW_FEATURE)")
	}

	objectRe := regexp.MustCompile(`(?s)export\s+const\s+WORKFLOW_FEATURE\s*=\s*\{(?P<body>.*?)\}\s+as\s+const`)
	match := objectRe.FindStringSubmatch(source)
	if match == nil {
		t.Fatal("WORKFLOW_FEATURE object not found")
	}
	body := match[objectRe.SubexpIndex("body")]
	entryRe := regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)\s*:\s*'([^']+)'\s*,?`)
	matches := entryRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		t.Fatal("WORKFLOW_FEATURE has no string literal values")
	}
	slugs := make([]string, 0, len(matches))
	seenKeys := make(map[string]struct{}, len(matches))
	seenSlugs := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		key := match[1]
		slug := match[2]
		if _, exists := seenKeys[key]; exists {
			t.Fatalf("duplicate WORKFLOW_FEATURE key %q", key)
		}
		if _, exists := seenSlugs[slug]; exists {
			t.Fatalf("duplicate WORKFLOW_FEATURE slug %q", slug)
		}
		seenKeys[key] = struct{}{}
		seenSlugs[slug] = struct{}{}
		slugs = append(slugs, slug)
	}
	return slugs
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
		{feature: FeatureSlugRemoveBg, want: false},
		{feature: FeatureSlugImageExtend, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			if got := IsReferenceImageFeature(tt.feature); got != tt.want {
				t.Fatalf("feature=%q got=%v, want=%v", tt.feature, got, tt.want)
			}
		})
	}
}
