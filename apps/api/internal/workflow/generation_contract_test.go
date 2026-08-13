package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestParseGenerationPrimitive(t *testing.T) {
	for _, primitive := range []GenerationPrimitive{
		GenerationPrimitiveTextToImage,
		GenerationPrimitiveImageToImage,
	} {
		got, err := ParseGenerationPrimitive(string(primitive))
		if err != nil || got != primitive {
			t.Fatalf("primitive=%q got=(%q,%v)", primitive, got, err)
		}
	}
	for _, raw := range []string{"", " text-to-image ", "TEXT-TO-IMAGE", "style-fusion", "image-to-video"} {
		if _, err := ParseGenerationPrimitive(raw); err == nil {
			t.Fatalf("primitive %q should be rejected", raw)
		}
	}
}

func TestParseRequestedOutputCountAcceptsOnlyOneThroughFour(t *testing.T) {
	for value := 1; value <= 4; value++ {
		count, err := ParseRequestedOutputCount(value)
		if err != nil || count.Int() != value {
			t.Fatalf("value=%d got=(%d,%v)", value, count, err)
		}
	}
	for _, value := range []int{-1, 0, 5, 8} {
		if _, err := ParseRequestedOutputCount(value); err == nil {
			t.Fatalf("output count %d should be rejected", value)
		}
	}
}

func TestGenerationContractMatchesFrontendAndOpenAPI(t *testing.T) {
	repoRoot := findWorkflowRepoRoot(t)
	frontendRaw, err := os.ReadFile(filepath.Join(repoRoot, "apps", "web", "src", "shared", "domain", "generationContract.ts"))
	if err != nil {
		t.Fatalf("read frontend generation contract: %v", err)
	}
	openAPIRaw, err := os.ReadFile(filepath.Join(repoRoot, "apps", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI generation contract: %v", err)
	}

	wantPrimitives := []string{string(GenerationPrimitiveTextToImage), string(GenerationPrimitiveImageToImage)}
	assertSameContractValues(t, "frontend generation primitives", wantPrimitives, extractTypeScriptStringObject(t, string(frontendRaw), "GENERATION_PRIMITIVE"))
	assertSameContractValues(t, "OpenAPI generation primitives", wantPrimitives, extractOpenAPIStringEnum(t, string(openAPIRaw), "GenerationPrimitive"))

	if !regexp.MustCompile(`REQUESTED_OUTPUT_COUNT_OPTIONS\s*=\s*\[1, 2, 3, 4\]\s+as const`).Match(frontendRaw) {
		t.Fatal("frontend requested output count options must be exactly [1, 2, 3, 4]")
	}
	if !regexp.MustCompile(`(?m)^    RequestedOutputCount:\n      type: integer\n      enum: \[1, 2, 3, 4\]$`).Match(openAPIRaw) {
		t.Fatal("OpenAPI RequestedOutputCount enum must be exactly [1, 2, 3, 4]")
	}
}

func extractTypeScriptStringObject(t *testing.T, source, constName string) []string {
	t.Helper()
	objectRe := regexp.MustCompile(`(?s)export\s+const\s+` + regexp.QuoteMeta(constName) + `\s*=\s*\{(?P<body>.*?)\}\s+as\s+const`)
	match := objectRe.FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("frontend %s object not found", constName)
	}
	body := match[objectRe.SubexpIndex("body")]
	entryRe := regexp.MustCompile(`(?m)[A-Za-z_][A-Za-z0-9_]*\s*:\s*'([^']+)'\s*,?`)
	entries := entryRe.FindAllStringSubmatch(body, -1)
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry[1])
	}
	return values
}

func assertSameContractValues(t *testing.T, label string, want, got []string) {
	t.Helper()
	want = append([]string(nil), want...)
	got = append([]string(nil), got...)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("%s mismatch: got=%v want=%v", label, got, want)
	}
}
