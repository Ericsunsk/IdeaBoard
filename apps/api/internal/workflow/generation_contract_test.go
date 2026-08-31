package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestParseGenerationPrimitive(t *testing.T) {
	for _, primitive := range []GenerationPrimitive{GenerationPrimitiveTextToImage, GenerationPrimitiveImageToImage} {
		got, err := ParseGenerationPrimitive(string(primitive))
		if err != nil || got != primitive {
			t.Fatalf("primitive=%q got=(%q,%v)", primitive, got, err)
		}
	}
	for _, raw := range []string{"", " text-to-image ", "TEXT-TO-IMAGE", "fusion", "image-to-video"} {
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
	root := findWorkflowRepoRoot(t)
	frontend, err := os.ReadFile(filepath.Join(root, "apps/web/src/shared/domain/generationContract.ts"))
	if err != nil {
		t.Fatal(err)
	}
	openapi, err := os.ReadFile(filepath.Join(root, "apps/api/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"text-to-image", "image-to-image"}
	assertSameContractValues(t, "frontend generation primitives", want, extractTypeScriptStringObject(t, string(frontend), "GENERATION_PRIMITIVE"))
	assertSameContractValues(t, "OpenAPI generation primitives", want, extractOpenAPIStringEnum(t, string(openapi), "GenerationPrimitive"))
	if !regexp.MustCompile(`REQUESTED_OUTPUT_COUNT_OPTIONS\s*=\s*\[1, 2, 3, 4\]\s+as const`).Match(frontend) {
		t.Fatal("frontend output count contract mismatch")
	}
}

func assertSameContractValues(t *testing.T, label string, want, got []string) {
	t.Helper()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("contract mismatch: got=%v want=%v", got, want)
	}
}

func extractTypeScriptStringObject(t *testing.T, source, name string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)export\s+const\s+` + regexp.QuoteMeta(name) + `\s*=\s*\{(.*?)\}\s+as\s+const`).FindStringSubmatch(source)
	if len(m) < 2 {
		t.Fatalf("%s not found", name)
	}
	ms := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1)
	out := make([]string, 0, len(ms))
	for _, x := range ms {
		out = append(out, x[1])
	}
	return out
}

func TestValidateGenerationSubmitContract(t *testing.T) {
	for _, params := range []map[string]interface{}{
		nil,
		{"image_urls": []string{}},
		{"image_urls": []string{"one", "two", "three", "four"}},
	} {
		if err := ValidateGenerationSubmitContract(params); err != nil {
			t.Fatalf("valid params %#v: %v", params, err)
		}
	}

	for _, params := range []map[string]interface{}{
		{"mode": "text-to-image"},
		{"primitive": "image-to-image"},
		{"reference_image_urls": []string{"one"}},
		{"image_urls": []string{"one", "two", "three", "four", "five"}},
	} {
		if err := ValidateGenerationSubmitContract(params); err == nil || !strings.Contains(err.Error(), "generation") {
			t.Fatalf("invalid params %#v error=%v", params, err)
		}
	}
}
