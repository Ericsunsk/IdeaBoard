package submission

import (
	"encoding/json"
	"testing"

	taskbilling "ideaboard/api/internal/tasking/application/billing"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func canonicalExecutionPlanForTest(t *testing.T, feature string, count int) (workflow.FeatureExecutionPlan, string) {
	t.Helper()
	plan, params, err := canonicalExecutionPlanFixture(feature, count)
	if err != nil {
		t.Fatal(err)
	}
	return plan, params
}

func canonicalExecutionPlanFixture(feature string, count int) (workflow.FeatureExecutionPlan, string, error) {
	resolution := ""
	imageInputCount := 1
	switch feature {
	case workflow.FeatureSlugGeneration:
		imageInputCount = 0
	case workflow.FeatureSlugFusion:
		imageInputCount = 2
	}
	plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: feature,
		Params: workflow.FeatureExecutionParams{
			RequestedOutputCount: &count,
			OutputResolution:     &resolution,
			ImageInputCount:      imageInputCount,
			HasMaskInput:         feature == workflow.FeatureSlugInpaint,
		},
	})
	if err != nil {
		return workflow.FeatureExecutionPlan{}, "", err
	}
	encoded, err := workflow.EncodeFeatureExecutionPlan(plan)
	if err != nil {
		return workflow.FeatureExecutionPlan{}, "", err
	}
	params, err := json.Marshal(tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       feature,
		ExecutionPlan: encoded,
	})
	if err != nil {
		return workflow.FeatureExecutionPlan{}, "", err
	}
	return plan, string(params), nil
}

func TestNewCanonicalRequestRejectsMismatchedBillingProduct(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:         42,
		Feature:        "variations",
		ClientActionID: "action-1",
		BillingRequest: &taskbilling.Request{
			Feature:          "generation",
			OutputResolution: "1k",
			Quantity:         1,
		},
	})
	if err == nil {
		t.Fatal("expected mismatched workflow and Billing product to be rejected")
	}
}

func TestNewCanonicalRequestRejectsInvalidPriority(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:   42,
		Feature:  "variations",
		Priority: tasks.TaskPriorityLowest + 1,
	})
	if err == nil {
		t.Fatal("expected invalid priority to be rejected")
	}
}

func TestNewCanonicalRequestRejectsInvalidUserID(t *testing.T) {
	for _, userID := range []int64{0, -1} {
		_, err := NewCanonicalRequest(CanonicalRequestInput{
			UserID:  userID,
			Feature: "variations",
		})
		if err == nil {
			t.Fatalf("expected userID %d to be rejected", userID)
		}
	}
}

func TestNewCanonicalRequestRejectsMissingBillingForBillableWorkflow(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:  42,
		Feature: "variations",
	})
	if err == nil {
		t.Fatal("expected billable workflow without Billing request to be rejected")
	}
}

func TestNewCanonicalRequestRejectsMissingExecutionPlanForAIImageWorkflow(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Feature: workflow.FeatureSlugGeneration,
		ClientActionID: "missing-plan",
		BillingRequest: &taskbilling.Request{
			Feature: workflow.FeatureSlugGeneration, OutputResolution: "1k", Quantity: 1,
		},
	})
	if err == nil {
		t.Fatal("expected AI image request without execution plan to be rejected")
	}
}

func TestNewCanonicalRequestRejectsPersistedExecutionPlanMismatch(t *testing.T) {
	plan, _ := canonicalExecutionPlanForTest(t, workflow.FeatureSlugGeneration, 1)
	_, styleParams := canonicalExecutionPlanForTest(t, workflow.FeatureSlugVariations, 1)
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Feature: workflow.FeatureSlugGeneration, Params: styleParams,
		ClientActionID: "mismatched-plan", ExecutionPlan: &plan,
		BillingRequest: &taskbilling.Request{
			Feature: workflow.FeatureSlugGeneration, OutputResolution: "1k", Quantity: 1,
		},
	})
	if err == nil {
		t.Fatal("expected persisted execution plan mismatch to be rejected")
	}
}

func TestCanonicalRequestOwnsBillingFacts(t *testing.T) {
	inputBilling := &taskbilling.Request{Feature: "variations", OutputResolution: "1k", Quantity: 1}
	plan, params := canonicalExecutionPlanForTest(t, inputBilling.Feature, 1)
	request, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:         42,
		Feature:        inputBilling.Feature,
		Params:         params,
		ClientActionID: "action-1",
		BillingRequest: inputBilling,
		ExecutionPlan:  &plan,
	})
	if err != nil {
		t.Fatal(err)
	}

	inputBilling.Feature = "generation"
	inputBilling.Quantity = 9
	billingRequest := request.BillingRequest()
	if billingRequest == nil || billingRequest.Feature != "variations" || billingRequest.Quantity != 1 {
		t.Fatalf("Billing request=%+v, want immutable variations quantity 1", billingRequest)
	}
}

func TestCanonicalRequestRejectsPreparedMediaThatDivergesFromExecutionPlan(t *testing.T) {
	plan, params := canonicalExecutionPlanForTest(t, workflow.FeatureSlugVariations, 1)
	request, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Feature: workflow.FeatureSlugVariations, Params: params,
		ClientActionID: "prepare-media-contract", ExecutionPlan: &plan,
		BillingRequest: &taskbilling.Request{Feature: workflow.FeatureSlugVariations, OutputResolution: "1k", Quantity: 1},
		Generation: &tasks.GenerationRequestSnapshot{
			Feature: workflow.FeatureSlugVariations, Primitive: string(workflow.GenerationPrimitiveImageToImage),
			TemplateFingerprint: "fp", RequestedOutputCount: 1,
			Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: "input", Source: "source"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string][]tasks.GenerationMediaIntent{
		"missing image": nil,
		"extra image": {
			{InputIndex: 0, Role: "input", Source: "source"},
			{InputIndex: 1, Role: "reference", Source: "extra"},
		},
		"injected mask": {
			{InputIndex: 0, Role: "mask", Source: "mask"},
		},
	}
	for name, media := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := *request.GenerationSnapshot()
			snapshot.Media = media
			if _, err := request.WithGenerationSnapshot(snapshot); err == nil {
				t.Fatal("expected prepared media divergence to be rejected")
			}
		})
	}
}

func TestValidatePreparedGenerationMediaRequiresAssetIDs(t *testing.T) {
	snapshot := &tasks.GenerationRequestSnapshot{
		Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "/uploads/source.png"}},
	}
	if err := ValidatePreparedGenerationMedia(snapshot); err == nil {
		t.Fatal("expected prepared media without an asset id to be rejected")
	}
	snapshot.Media[0].AssetID = 101
	if err := ValidatePreparedGenerationMedia(snapshot); err != nil {
		t.Fatalf("ValidatePreparedGenerationMedia() error = %v", err)
	}
}

func TestNewCanonicalRequestRejectsBillingQuantityMismatchedWithGeneration(t *testing.T) {
	count := 2
	plan, params := canonicalExecutionPlanForTest(t, workflow.FeatureSlugVariations, count)
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Feature: "variations", Params: params, ClientActionID: "action-1", ExecutionPlan: &plan,
		BillingRequest: &taskbilling.Request{Feature: "variations", OutputResolution: "1k", Quantity: 1},
		Generation: &tasks.GenerationRequestSnapshot{
			Feature: "variations", Primitive: "image-to-image", TemplateFingerprint: "fp", RequestedOutputCount: count,
		},
	})
	if err == nil {
		t.Fatal("expected billing quantity to match generation output count")
	}
}
