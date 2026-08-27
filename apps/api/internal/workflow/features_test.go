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
		{name: "unsupported slug", raw: "unknown-workflow", want: "", ok: false},
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
		FeatureSlugPromptGeneration,
		FeatureSlugInpaint,
		FeatureSlugRender,
		FeatureSlugVariations,
		FeatureSlugCutout,
		FeatureSlugStyleFusion,
		FeatureSlugHDUpscale,
		FeatureSlugOutpaint,
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

func TestCanonicalFeatureRegistryMatchesProductionRouting(t *testing.T) {
	canonical := CanonicalFeatureSlugs()
	for _, slug := range canonical {
		t.Run(slug, func(t *testing.T) {
			if _, ok := ParseWorkflowFeatureSlug(slug); !ok {
				t.Fatalf("canonical feature %q is not routable", slug)
			}
			if !IsAIImageFeature(slug) {
				t.Fatalf("canonical feature %q is not billable", slug)
			}
		})
	}
}

func TestAIImageFeatureSlugsAreTheCanonicalWorkflowContract(t *testing.T) {
	got := AIImageFeatureSlugs()
	want := []string{
		FeatureSlugPromptGeneration,
		FeatureSlugInpaint,
		FeatureSlugRender,
		FeatureSlugVariations,
		FeatureSlugCutout,
		FeatureSlugStyleFusion,
		FeatureSlugHDUpscale,
		FeatureSlugOutpaint,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("AI image features=%v, want=%v", got, want)
	}
	supported := SupportedFeatureSlugs()
	for _, slug := range want {
		if !IsAIImageFeature(slug) {
			t.Fatalf("canonical workflow %q is not classified as AI image", slug)
		}
		if !slices.Contains(supported, slug) {
			t.Fatalf("AI image workflow %q is not supported", slug)
		}
	}
	for _, slug := range []string{"", "crop", "color-adjust", "mask-draw"} {
		if IsAIImageFeature(slug) {
			t.Fatalf("local operation %q must not be classified as AI image", slug)
		}
	}
}

func TestSupportedFeatureSlugsMatchFrontendOpenAPIAndAdminContracts(t *testing.T) {
	repoRoot := findWorkflowRepoRoot(t)
	frontendPath := filepath.Join(repoRoot, "apps", "web", "src", "shared", "domain", "workflowFeatures.ts")
	raw, err := os.ReadFile(frontendPath)
	if err != nil {
		t.Fatalf("read frontend workflow features: %v", err)
	}

	frontendSlugs := extractFrontendWorkflowFeatureSlugs(t, string(raw))
	backendSlugs := SupportedFeatureSlugs()
	assertSameFeatureSlugs(t, "frontend shared contract", backendSlugs, frontendSlugs)

	openAPIRaw, err := os.ReadFile(filepath.Join(repoRoot, "apps", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	assertSameFeatureSlugs(t, "OpenAPI", backendSlugs, extractOpenAPIWorkflowFeatureSlugs(t, string(openAPIRaw)))
	assertSameFeatureSlugs(
		t,
		"OpenAPI canonical contract",
		CanonicalFeatureSlugs(),
		extractOpenAPIStringEnum(t, string(openAPIRaw), "CanonicalWorkflowFeatureSlug"),
	)

	featureKeys := extractFrontendWorkflowFeatureKeys(t, string(raw))
	for label, relativePath := range map[string]string{
		"frontend execution": "apps/web/src/features/canvas/execution/contracts/executionContract.ts",
		"Admin registry":     "apps/web/src/features/admin/shared/lib/workflowFeatureAdminMeta.ts",
	} {
		contractRaw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s: %v", label, err)
		}
		assertSameFeatureSlugs(t, label, backendSlugs, extractWorkflowFeatureReferences(t, label, string(contractRaw), featureKeys))
	}
}

func assertSameFeatureSlugs(t *testing.T, label string, want, got []string) {
	t.Helper()
	want = append([]string(nil), want...)
	got = append([]string(nil), got...)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("%s workflow feature slugs mismatch\nbackend=%v\ncontract=%v", label, want, got)
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

func extractFrontendWorkflowFeatureKeys(t *testing.T, source string) map[string]string {
	t.Helper()
	objectRe := regexp.MustCompile(`(?s)export\s+const\s+WORKFLOW_FEATURE\s*=\s*\{(?P<body>.*?)\}\s+as\s+const`)
	match := objectRe.FindStringSubmatch(source)
	if match == nil {
		t.Fatal("WORKFLOW_FEATURE object not found")
	}
	body := match[objectRe.SubexpIndex("body")]
	entryRe := regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)\s*:\s*'([^']+)'\s*,?`)
	keys := make(map[string]string)
	for _, entry := range entryRe.FindAllStringSubmatch(body, -1) {
		keys[entry[1]] = entry[2]
	}
	return keys
}

func extractOpenAPIWorkflowFeatureSlugs(t *testing.T, source string) []string {
	t.Helper()
	blockRe := regexp.MustCompile(`(?m)^    WorkflowFeatureSlug:\n      type: string\n      enum:\n((?:        - [a-z0-9-]+\n)+)`)
	match := blockRe.FindStringSubmatch(source)
	if match == nil {
		t.Fatal("OpenAPI WorkflowFeatureSlug enum not found")
	}
	entryRe := regexp.MustCompile(`(?m)^        - ([a-z0-9-]+)$`)
	entries := entryRe.FindAllStringSubmatch(match[1], -1)
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		slugs = append(slugs, entry[1])
	}
	return slugs
}

func extractOpenAPIStringEnum(t *testing.T, source, schemaName string) []string {
	t.Helper()
	blockRe := regexp.MustCompile(`(?m)^    ` + regexp.QuoteMeta(schemaName) + `:\n      type: string\n      enum:\n((?:        - [a-z0-9-]+\n)+)`)
	match := blockRe.FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("OpenAPI %s enum not found", schemaName)
	}
	entryRe := regexp.MustCompile(`(?m)^        - ([a-z0-9-]+)$`)
	entries := entryRe.FindAllStringSubmatch(match[1], -1)
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry[1])
	}
	return values
}

func extractWorkflowFeatureReferences(t *testing.T, label, source string, featureKeys map[string]string) []string {
	t.Helper()
	var blockPattern string
	switch label {
	case "frontend execution":
		blockPattern = `(?s)export\s+const\s+EXECUTION_FEATURE\s*=\s*\{(.*?)\}\s+as\s+const`
	case "Admin registry":
		blockPattern = `(?s)const\s+ADMIN_WORKFLOW_FEATURE_META.*?=\s*\{(.*?)\}\n\nexport\s+function`
	default:
		t.Fatalf("unsupported feature contract label %q", label)
	}
	match := regexp.MustCompile(blockPattern).FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("%s feature contract block not found", label)
	}
	references := regexp.MustCompile(`WORKFLOW_FEATURE\.([A-Za-z_][A-Za-z0-9_]*)`).FindAllStringSubmatch(match[1], -1)
	slugs := make([]string, 0, len(references))
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		key := reference[1]
		slug, ok := featureKeys[key]
		if !ok {
			t.Fatalf("%s references unknown WORKFLOW_FEATURE key %q", label, key)
		}
		if _, duplicate := seen[slug]; duplicate {
			t.Fatalf("%s duplicates workflow feature %q", label, slug)
		}
		seen[slug] = struct{}{}
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
		{name: "google cloud", apiType: ProviderGoogleCloud, want: ProviderGoogleCloud, wantOK: true},
		{name: "infer google", baseURL: "https://generativelanguage.googleapis.com", want: ProviderGoogleAI, wantOK: true},
		{name: "infer google cloud", baseURL: "https://aiplatform.googleapis.com", want: ProviderGoogleCloud, wantOK: true},
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
