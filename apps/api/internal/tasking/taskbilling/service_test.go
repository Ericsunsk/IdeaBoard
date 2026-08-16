package taskbilling

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/workflow"
)

func TestAIImageWorkflowBillingConsumesCompiledPlan(t *testing.T) {
	plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: workflow.FeatureSlugRemoveBg,
		Params:  workflow.FeatureExecutionParams{ImageInputCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := AIImageRequest(
		plan.Feature(),
		plan.OutputResolution(),
		int64(plan.RequestedOutputCount().Int()),
	)
	if err != nil {
		t.Fatal(err)
	}
	sku, err := ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(sku) != "ai-image:remove-bg" || !RequiresBilling(plan.Feature()) {
		t.Fatalf("compiled plan billing identity: sku=%q request=%+v", sku, request)
	}
}

func TestAIImageRequestConsumesExplicitCompiledResolution(t *testing.T) {
	request, err := AIImageRequest(workflow.FeatureSlugRemoveBg, "default", 5)
	if err != nil || request.OutputResolution != billing.OutputResolutionDefault {
		t.Fatalf("default resolution request=%+v err=%v", request, err)
	}
	if request.Quantity != 5 {
		t.Fatalf("billing quantity=%d, want explicit quantity 5", request.Quantity)
	}
	if _, err := AIImageRequest(workflow.FeatureSlugRemoveBg, "", 1); !errors.Is(err, billing.ErrInvalidOutputResolution) {
		t.Fatalf("empty resolution error=%v", err)
	}
}

func TestAIImagePricingIdentityAndFingerprintAreFeatureScoped(t *testing.T) {
	first, err := AIImageRequest(workflow.FeatureSlugPartialEdit, "default", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AIImageRequest(workflow.FeatureSlugPartialEdit, "default", 1)
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
			_, err := AIImageRequest(feature, "1k", 1)
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
	if err := SettleTerminal(context.Background(), nil, "task"); err == nil {
		t.Fatal("nil SQL transaction was accepted")
	}
	if err := SettleTerminal(context.Background(), new(sql.Tx), ""); err == nil {
		t.Fatal("blank task id was accepted")
	}
}
