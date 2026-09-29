package application

import (
	"context"
	"strings"
	"testing"

	searchdomain "ideaboard/api/internal/searchcatalog/domain"
)

// TestVisionBillingOrchestrationHelpersAreDeterministic pins the failure
// taxonomy shared by the executor, settlement, and reconciliation paths.
// Classification lives in one place so provider outcomes map to stable
// operation failures without per-callsite literals.
func TestVisionBillingOrchestrationHelpersAreDeterministic(t *testing.T) {
	input := searchdomain.VisionInput{UserText: "  sunset   beach ", Images: []searchdomain.VisionImage{{Filename: "a.png", ContentType: "image/png", Data: []byte{1, 2, 3}}}}
	if got, want := hashVisionInput(input), hashVisionInput(input); got != want || len(got) != 64 {
		t.Fatalf("hashVisionInput unstable: %q", got)
	}
	other := input
	other.UserText = "mountain lake"
	if hashVisionInput(input) == hashVisionInput(other) {
		t.Fatal("hashVisionInput collides across distinct requests")
	}
	if err := validateVisionInput(searchdomain.VisionInput{}); err == nil {
		t.Fatal("empty vision input accepted")
	}
	if !isVisionUncertain(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded must be uncertain")
	}
	if isVisionUncertain(&searchdomain.VisionProviderError{Kind: searchdomain.VisionProviderFailureCredentials}) {
		t.Fatal("credential failure must not be uncertain")
	}
	code, _, status := visionFailureDetails(&searchdomain.VisionProviderError{Kind: searchdomain.VisionProviderFailureQuota})
	if code != "vision_search_quota" || status != 502 {
		t.Fatalf("quota failure = %q/%d, want vision_search_quota/502", code, status)
	}
	if got := httpStatusForVisionCode(visionFailureCodePricingUnavailable); got != 503 {
		t.Fatalf("pricing status = %d, want 503", got)
	}
	if got := httpStatusForVisionCode("vision_search_input_invalid"); got != 400 {
		t.Fatalf("input status = %d, want 400", got)
	}
	if !strings.HasPrefix(visionSearchSKU, "ai-search:") {
		t.Fatalf("vision SKU = %q, must stay in the ai-search namespace", visionSearchSKU)
	}
}
