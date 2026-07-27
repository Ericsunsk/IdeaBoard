package taskbilling

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/workflow"
)

func TestAIImageWorkflowSKUContract(t *testing.T) {
	expected := map[string]string{
		workflow.FeatureSlugSketchToImage:  "ai-image:sketch-to-image",
		workflow.FeatureSlugStyleVariation: "ai-image:style-variation",
		workflow.FeatureSlugPartialEdit:    "ai-image:partial-edit",
		workflow.FeatureSlugRemoveBg:       "ai-image:remove-bg",
		workflow.FeatureSlugStyleFusion:    "ai-image:style-fusion",
		workflow.FeatureSlugPatternGen:     "ai-image:pattern-gen",
		workflow.FeatureSlugHDUpscale:      "ai-image:hd-upscale",
		workflow.FeatureSlugImageExtend:    "ai-image:image-extend",
	}

	for _, feature := range workflow.AIImageFeatureSlugs() {
		t.Run(feature, func(t *testing.T) {
			request, err := AIImageRequest(feature)
			if err != nil {
				t.Fatal(err)
			}
			sku, err := ResolveSKU(request)
			if err != nil {
				t.Fatal(err)
			}
			if string(sku) != expected[feature] {
				t.Fatalf("sku=%q, want %q", sku, expected[feature])
			}
			if !RequiresBilling(feature) {
				t.Fatalf("feature %q must require billing facts", feature)
			}
		})
	}
}

func TestAIImagePricingIdentityAndFingerprintAreFeatureScoped(t *testing.T) {
	first, err := AIImageRequest(workflow.FeatureSlugPartialEdit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AIImageRequest(workflow.FeatureSlugPartialEdit)
	if err != nil {
		t.Fatal(err)
	}
	firstSKU, err := ResolveSKU(first)
	if err != nil {
		t.Fatal(err)
	}
	secondSKU, err := ResolveSKU(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstSKU != secondSKU {
		t.Fatalf("model changed pricing identity: %q != %q", firstSKU, secondSKU)
	}
	firstFingerprint, err := RequestFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := RequestFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint {
		t.Fatal("identical feature requests must share an idempotency fingerprint")
	}
}

func TestAIImageWorkflowRequestRejectsNonWorkflowProducts(t *testing.T) {
	for _, feature := range []string{"", "crop", "color-adjust", "mask-draw", "unknown"} {
		t.Run(feature, func(t *testing.T) {
			_, err := AIImageRequest(feature)
			if !errors.Is(err, ErrUnsupportedProduct) {
				t.Fatalf("err=%v, want ErrUnsupportedProduct", err)
			}
			if RequiresBilling(feature) {
				t.Fatalf("local/non-workflow feature %q must not require billing", feature)
			}
		})
	}
}

func TestTaskBillingRequestValidationFailsClosed(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		request Request
		wantErr error
	}{
		{name: "zero quantity", request: Request{Feature: workflow.FeatureSlugPartialEdit}, wantErr: billing.ErrInvalidQuantity},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := validateRequest(testCase.request); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateRequest() error=%v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestSettleTerminalRejectsInvalidInputsBeforeDatabaseAccess(t *testing.T) {
	if err := SettleTerminal(context.Background(), nil, "task", TerminalOutcomeSucceeded); err == nil {
		t.Fatal("nil SQL transaction was accepted")
	}
	if err := SettleTerminal(context.Background(), new(sql.Tx), "task", TerminalOutcome("unexpected")); err == nil {
		t.Fatal("invalid terminal outcome was accepted")
	}
}
