package taskservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestTaskIntakeCanonicalizesEveryAIImageWorkflowThroughItsPublicInterface(t *testing.T) {
	var intake = NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:           101,
			Capabilities: []string{"background-removal", "image-upscale"},
			Models:       []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	tests := []struct {
		feature string
		images  []string
		params  map[string]interface{}
	}{
		{feature: workflow.FeatureSlugSketchToImage, images: []string{"https://example.com/sketch.png"}},
		{
			feature: workflow.FeatureSlugStyleVariation,
			images:  []string{"https://example.com/style.png"},
			params:  map[string]interface{}{"variation": map[string]interface{}{"strength": 0.5}},
		},
		{
			feature: workflow.FeatureSlugLocalInpaint,
			images:  []string{"https://example.com/inpaint.png"},
			params:  map[string]interface{}{"mask_image_url": "https://example.com/mask.png"},
		},
		{feature: workflow.FeatureSlugRemoveBg, images: []string{"https://example.com/remove.png"}},
		{
			feature: workflow.FeatureSlugStyleFusion,
			images:  []string{"https://example.com/a.png", "https://example.com/b.png"},
			params: map[string]interface{}{
				"fusion": map[string]interface{}{"input_weights": []float64{3, 1}},
			},
		},
		{feature: workflow.FeatureSlugPromptGeneration},
		{
			feature: workflow.FeatureSlugHDUpscale,
			images:  []string{"https://example.com/upscale.png"},
			params:  map[string]interface{}{"hd": map[string]interface{}{"scale": 2.0}},
		},
		{
			feature: workflow.FeatureSlugImageExtend,
			images:  []string{"https://example.com/extend.png"},
			params: map[string]interface{}{"extend": map[string]interface{}{
				"top_percent": 0, "right_percent": 25, "bottom_percent": 0, "left_percent": 0,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			outputCount := 2
			if tt.feature == workflow.FeatureSlugRemoveBg || tt.feature == workflow.FeatureSlugHDUpscale {
				outputCount = 1
			}
			requestParams := map[string]interface{}{"output_count": outputCount}
			for key, value := range tt.params {
				requestParams[key] = value
			}
			canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt:         "保留主体",
				Feature:        tt.feature,
				ImageURLs:      tt.images,
				Params:         requestParams,
				ClientActionID: "intake-" + tt.feature,
			})
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if canonical.Feature() != tt.feature {
				t.Fatalf("Feature() = %q, want %q", canonical.Feature(), tt.feature)
			}
			if canonical.UserID() != 42 {
				t.Fatalf("UserID() = %d, want authorized user 42", canonical.UserID())
			}

			wantQuantity := int64(2)
			if tt.feature == workflow.FeatureSlugRemoveBg || tt.feature == workflow.FeatureSlugHDUpscale {
				wantQuantity = 1
			}
			billingRequest := canonical.BillingRequest()
			if billingRequest == nil || billingRequest.Feature != tt.feature || billingRequest.Quantity != wantQuantity {
				t.Fatalf("BillingRequest() = %#v", billingRequest)
			}

			var params map[string]interface{}
			if err := json.Unmarshal([]byte(canonical.Params()), &params); err != nil {
				t.Fatalf("decode canonical params: %v", err)
			}
			if _, exists := params["model"]; exists {
				t.Fatalf("canonical params must not snapshot model: %#v", params)
			}
			if _, exists := params["account_ids"]; exists {
				t.Fatalf("canonical params must not snapshot account ids: %#v", params)
			}
			typed, err := tasks.UnmarshalParams(canonical.Params())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := workflow.DecodeFeatureExecutionPlan(typed.ExecutionPlan)
			if err != nil || plan.Feature() != tt.feature {
				t.Fatalf("execution plan feature=%q err=%v", plan.Feature(), err)
			}
		})
	}
}

func TestTaskIntakeRejectsOutputCountOutsideCanonicalContract(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	for _, outputCount := range []interface{}{0, 5, 8, 1.5, "2"} {
		t.Run(fmt.Sprint(outputCount), func(t *testing.T) {
			_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt:         "generate",
				Feature:        workflow.FeatureSlugPromptGeneration,
				ClientActionID: "invalid-output-count",
				Params:         map[string]interface{}{"output_count": outputCount},
			})
			if err == nil || !strings.Contains(err.Error(), "output_count") {
				t.Fatalf("output_count=%v error=%v", outputCount, err)
			}
		})
	}
}

func TestTaskIntakeRejectsDedicatedImageTransformGenerationControls(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:           101,
			Capabilities: []string{"background-removal", "image-upscale"},
			Models:       []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	for _, feature := range []string{workflow.FeatureSlugRemoveBg, workflow.FeatureSlugHDUpscale} {
		t.Run(feature, func(t *testing.T) {
			params := map[string]interface{}{
				"output_count":      99,
				"output_resolution": "4k",
			}
			if feature == workflow.FeatureSlugHDUpscale {
				params["hd"] = map[string]interface{}{"scale": 2.5}
			}
			_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt: "ignored", Feature: feature, ClientActionID: "ignored-controls-" + feature, Params: params,
			})
			if err == nil || !strings.Contains(err.Error(), "output_count") {
				t.Fatalf("expected fixed output contract error, got %v", err)
			}
		})
	}
}

func TestTaskIntakeAllowsModelessRemoveBackgroundAccount(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:           101,
			Capabilities: []string{"background-removal"},
		}}},
		Media: testSubmitMediaValidator{},
	})

	if _, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Feature:        workflow.FeatureSlugRemoveBg,
		ImageURLs:      []string{"https://example.com/source.png"},
		ClientActionID: "modeless-remove-bg",
	}); err != nil {
		t.Fatalf("model-less remove-bg account rejected: %v", err)
	}
}

func TestTaskIntakePersistsProviderIndependentGenerationSnapshot(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101, Models: []accounts.AccountModel{{Name: "runtime-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})
	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw a chair", Feature: workflow.FeatureSlugSketchToImage,
		ImageURLs:      []string{"https://example.com/sketch.png"},
		ClientActionID: "snapshot-intake", Params: map[string]interface{}{"output_count": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	params, err := tasks.UnmarshalParams(canonical.Params())
	if err != nil {
		t.Fatal(err)
	}
	if params.Generation == nil {
		t.Fatal("expected generation snapshot in canonical params")
	}
	snapshot := params.Generation
	if snapshot.Feature != workflow.FeatureSlugSketchToImage || snapshot.UserPrompt != "draw a chair" || snapshot.CompiledPrompt != canonical.Prompt() {
		t.Fatalf("generation snapshot=%+v canonical prompt=%q", snapshot, canonical.Prompt())
	}
	if snapshot.RequestedOutputCount != 2 || snapshot.TemplateFingerprint == "" || snapshot.TemplateVersion != "" {
		t.Fatalf("generation snapshot missing frozen facts=%+v", snapshot)
	}
	if canonical.GenerationSnapshot() == nil {
		t.Fatal("expected canonical request generation snapshot")
	}
}

func TestTaskIntakePrepareMediaPreservesCompiledExecutionPlan(t *testing.T) {
	preparer := &recordingGenerationMediaPreparer{}
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101, Models: []accounts.AccountModel{{Name: "runtime-model", Enabled: true}},
		}}},
		Media:         testSubmitMediaValidator{},
		MediaPreparer: preparer,
	})
	canonical, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw from reference", Feature: workflow.FeatureSlugSketchToImage,
		ImageURLs: []string{"https://example.com/reference.png"}, ClientActionID: "plan-media-prepare",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := tasks.UnmarshalParams(canonical.Params())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := intake.PrepareMedia(t.Context(), 42, canonical)
	if err != nil {
		t.Fatal(err)
	}
	after, err := tasks.UnmarshalParams(prepared.Params())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.ExecutionPlan, after.ExecutionPlan) {
		t.Fatalf("execution plan changed during media preparation\nbefore=%s\nafter=%s", before.ExecutionPlan, after.ExecutionPlan)
	}
	if after.Generation == nil || len(after.Generation.Media) != 1 || after.Generation.Media[0].AssetID == 0 || after.Generation.Media[0].Ref == before.Generation.Media[0].Ref {
		t.Fatalf("prepared generation media=%+v", after.Generation)
	}
}

func TestTaskIntakeCompilesPromptGenerationPrimitiveAndOrderedReferences(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101, Models: []accounts.AccountModel{{Name: "runtime-model", Enabled: true}},
		}}},
		Prompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugPromptGeneration: {
				Prompt:                "Create an image. {{user_prompt}}",
				UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(workflow.FeatureSlugPromptGeneration),
			},
		}},
		Media: testSubmitMediaValidator{},
	})

	textRequest, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Feature: workflow.FeatureSlugPromptGeneration, ClientActionID: "prompt-text",
		Params: map[string]interface{}{"output_count": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot := textRequest.GenerationSnapshot(); snapshot == nil || snapshot.Primitive != "text-to-image" || snapshot.RequestedOutputCount != 2 || snapshot.CompiledPrompt != "Create an image." {
		t.Fatalf("text snapshot=%+v", snapshot)
	}

	refs := []string{"https://example.com/one.png", "https://example.com/two.png", "https://example.com/three.png"}
	imageRequest, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt: "keep this order", Feature: workflow.FeatureSlugPromptGeneration,
		ImageURLs: refs, ClientActionID: "prompt-image",
		Params: map[string]interface{}{"output_count": 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := imageRequest.GenerationSnapshot()
	if snapshot == nil || snapshot.Primitive != "image-to-image" || snapshot.RequestedOutputCount != 4 || len(snapshot.Media) != len(refs) {
		t.Fatalf("image snapshot=%+v", snapshot)
	}
	for index, ref := range refs {
		if snapshot.Media[index].InputIndex != index || snapshot.Media[index].Ref != ref || snapshot.Media[index].Weight != 0 {
			t.Fatalf("ordered media[%d]=%+v", index, snapshot.Media[index])
		}
	}
	billing := imageRequest.BillingRequest()
	if billing == nil || billing.Feature != workflow.FeatureSlugPromptGeneration || billing.Quantity != 4 {
		t.Fatalf("billing=%+v", billing)
	}
}

func TestTaskIntakeAllowsMissingSystemPrompt(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugPromptGeneration,
		ClientActionID: "missing-system-prompt",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if canonical.Prompt() != "repeatable textile pattern" {
		t.Fatalf("Prompt = %q, want user prompt unchanged", canonical.Prompt())
	}
}

func TestTaskIntakePreservesPromptCauseBehindStableError(t *testing.T) {
	cause := errors.New("database connection refused")
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Prompts: fakeFeaturePromptStore{err: cause},
		Media:   testSubmitMediaValidator{},
	})

	_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugPromptGeneration,
		ClientActionID: "prompt-cause",
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Prepare() error = %v, want wrapped cause", err)
	}
	var submissionErr *SubmissionError
	if !errors.As(err, &submissionErr) {
		t.Fatalf("Prepare() error = %T, want SubmissionError", err)
	}
	if submissionErr.Code != "prompt_unavailable" || submissionErr.SafeMessage != "系统提示词暂不可用" {
		t.Fatalf("SubmissionError = %#v", submissionErr)
	}
}
