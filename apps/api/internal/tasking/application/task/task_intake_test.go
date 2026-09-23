package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/prompts"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

type processingReadinessStub struct {
	err   error
	calls [][]int64
}

func (s *processingReadinessStub) ValidateProcessingInputReadiness(_ context.Context, _ int64, assetIDs []int64) error {
	s.calls = append(s.calls, append([]int64(nil), assetIDs...))
	return s.err
}

var _ taskmedia.ProcessingInputReadiness = (*processingReadinessStub)(nil)

type assetAwareMediaValidator struct{}

func (assetAwareMediaValidator) ValidateSubmitMediaRef(context.Context, int64, string) error {
	return nil
}
func (assetAwareMediaValidator) ValidateSubmitMediaAssetIDs(context.Context, int64, []int64) error {
	return nil
}

var _ taskmedia.Validator = assetAwareMediaValidator{}

func TestTaskIntakeReadinessRejectsMissingProcessingInputDerivative(t *testing.T) {
	readiness := &processingReadinessStub{err: errors.New("derivative pending")}
	intake := NewTaskIntake(TaskIntakeDeps{
		Models:         &fakeIntakeModelStore{},
		Media:          assetAwareMediaValidator{},
		MediaReadiness: readiness,
	})
	_, err := intake.CompileReadiness(t.Context(), 42, &tasks.SubmitTaskRequest{
		Feature: workflow.FeatureSlugVariations,
		Params: map[string]interface{}{
			"image_asset_ids": []int64{9},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "media_processing_input_not_ready") {
		t.Fatalf("expected processing input readiness error, got %v", err)
	}
	if len(readiness.calls) != 1 || len(readiness.calls[0]) != 1 || readiness.calls[0][0] != 9 {
		t.Fatalf("readiness calls = %#v", readiness.calls)
	}
}

func TestTaskIntakeCanonicalizesEveryAIImageWorkflowThroughItsPublicInterface(t *testing.T) {
	var intake = NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Prompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugInpaint:  {Prompt: "system"},
			workflow.FeatureSlugOutpaint: {Prompt: "system"},
		}},
		Media: testSubmitMediaValidator{},
	})

	tests := []struct {
		feature string
		images  []string
		params  map[string]interface{}
	}{
		{feature: workflow.FeatureSlugRender, images: []string{"https://example.com/sketch.png"}},
		{
			feature: workflow.FeatureSlugVariations,
			images:  []string{"https://example.com/style.png"},
			params:  map[string]interface{}{"variation": map[string]interface{}{"strength": 0.5}},
		},
		{
			feature: workflow.FeatureSlugInpaint,
			images:  []string{"https://example.com/inpaint.png"},
			params: map[string]interface{}{
				"mask_image_url":        "https://example.com/mask.png",
				"original_image_width":  1024,
				"original_image_height": 768,
				"custom_width":          1024,
				"custom_height":         768,
			},
		},
		{feature: workflow.FeatureSlugCutout, images: []string{"https://example.com/remove.png"}},
		{
			feature: workflow.FeatureSlugFusion,
			images:  []string{"https://example.com/a.png", "https://example.com/b.png"},
			params: map[string]interface{}{
				"fusion": map[string]interface{}{"input_weights": []float64{3, 1}},
			},
		},
		{feature: workflow.FeatureSlugGeneration},
		{
			feature: workflow.FeatureSlugUpscale,
			images:  []string{"https://example.com/upscale.png"},
			params:  map[string]interface{}{"upscale": map[string]interface{}{"scale": 2.0}},
		},
		{
			feature: workflow.FeatureSlugOutpaint,
			images:  []string{"https://example.com/extend.png"},
			params: map[string]interface{}{"outpaint": map[string]interface{}{
				"target_width": 1280, "target_height": 768,
				"image_offset_x": 0, "image_offset_y": 0,
			},
				"original_image_width":  1024,
				"original_image_height": 768,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			outputCount := 2
			if tt.feature == workflow.FeatureSlugCutout || tt.feature == workflow.FeatureSlugUpscale || tt.feature == workflow.FeatureSlugInpaint || tt.feature == workflow.FeatureSlugOutpaint {
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
			if tt.feature == workflow.FeatureSlugCutout || tt.feature == workflow.FeatureSlugUpscale || tt.feature == workflow.FeatureSlugInpaint || tt.feature == workflow.FeatureSlugOutpaint {
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
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})

	for _, outputCount := range []interface{}{0, 5, 8, 1.5, "2"} {
		t.Run(fmt.Sprint(outputCount), func(t *testing.T) {
			_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt:         "generate",
				Feature:        workflow.FeatureSlugGeneration,
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
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})

	for _, feature := range []string{workflow.FeatureSlugCutout, workflow.FeatureSlugUpscale} {
		t.Run(feature, func(t *testing.T) {
			params := map[string]interface{}{
				"output_count":      99,
				"output_resolution": "4k",
			}
			if feature == workflow.FeatureSlugUpscale {
				params["upscale"] = map[string]interface{}{"scale": 2.5}
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
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})

	if _, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Feature:        workflow.FeatureSlugCutout,
		ImageURLs:      []string{"https://example.com/source.png"},
		ClientActionID: "modeless-cutout",
	}); err != nil {
		t.Fatalf("model-less cutout account rejected: %v", err)
	}
}

func TestTaskIntakeReadinessDoesNotRequireExecutionMedia(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Prompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugInpaint: {Prompt: "system"},
		}},
	})

	for _, feature := range []string{workflow.FeatureSlugInpaint, workflow.FeatureSlugCutout} {
		t.Run(feature, func(t *testing.T) {
			params := map[string]interface{}{"output_count": 1}
			if feature == workflow.FeatureSlugInpaint {
				params["original_image_width"] = 1024
				params["original_image_height"] = 768
				params["custom_width"] = 1024
				params["custom_height"] = 768
			}
			compiled, err := intake.CompileReadiness(t.Context(), 42, &tasks.SubmitTaskRequest{
				Feature: feature,
				Params:  params,
			})
			if err != nil {
				t.Fatalf("CompileReadiness() error = %v", err)
			}
			if compiled.Feature != feature {
				t.Fatalf("feature = %q, want %q", compiled.Feature, feature)
			}
		})
	}
}

func TestTaskIntakePersistsProviderIndependentGenerationSnapshot(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})
	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw a chair", Feature: workflow.FeatureSlugRender,
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
	if snapshot.Feature != workflow.FeatureSlugRender || snapshot.UserPrompt != "draw a chair" || snapshot.CompiledPrompt != canonical.Prompt() {
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
		Models:        &fakeIntakeModelStore{},
		Media:         testSubmitMediaValidator{},
		MediaPreparer: preparer,
		MediaManifest: recordingTaskInputManifestBuilder{},
	})
	canonical, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw from reference", Feature: workflow.FeatureSlugRender,
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
	if after.Generation == nil || len(after.Generation.Media) != 1 || after.Generation.Media[0].AssetID == 0 || after.Generation.Media[0].Source != "" {
		t.Fatalf("prepared generation media=%+v", after.Generation)
	}
	if after.Media == nil || len(after.Media.Inputs) != 1 || after.Media.Inputs[0].AssetID != after.Generation.Media[0].AssetID || after.Media.Inputs[0].Source != "" {
		t.Fatalf("prepared typed media=%+v, generation=%+v", after.Media, after.Generation)
	}
	for _, legacyField := range []string{`"url"`, `"ref"`, `"image_urls"`, `"mask_image_url"`, `"mask_image_urls"`, `"original_image_url"`} {
		if strings.Contains(prepared.Params(), legacyField) {
			t.Fatalf("prepared params retained legacy media field %s: %s", legacyField, prepared.Params())
		}
	}
}

func TestTaskIntakePrepareMediaFailsClosedWithoutPreparer(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})
	canonical, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw from reference", Feature: workflow.FeatureSlugRender,
		ImageURLs: []string{"https://example.com/reference.png"}, ClientActionID: "missing-media-preparer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := intake.PrepareMedia(t.Context(), 42, canonical); err == nil {
		t.Fatal("expected media preparation to fail without a preparer")
	}
}

func TestTaskIntakeCompilesGenerationPrimitiveAndOrderedReferences(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Prompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugGeneration: {
				Prompt:                "Create an image. {{user_prompt}}",
				UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(workflow.FeatureSlugGeneration),
			},
		}},
		Media: testSubmitMediaValidator{},
	})

	textRequest, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Feature: workflow.FeatureSlugGeneration, ClientActionID: "prompt-text",
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
		Prompt: "keep this order", Feature: workflow.FeatureSlugGeneration,
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
		if snapshot.Media[index].InputIndex != index || snapshot.Media[index].Source != ref || snapshot.Media[index].Weight != 0 {
			t.Fatalf("ordered media[%d]=%+v", index, snapshot.Media[index])
		}
	}
	billing := imageRequest.BillingRequest()
	if billing == nil || billing.Feature != workflow.FeatureSlugGeneration || billing.Quantity != 4 {
		t.Fatalf("billing=%+v", billing)
	}
}

func TestTaskIntakeAllowsMissingSystemPrompt(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})

	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugGeneration,
		ClientActionID: "missing-system-prompt",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if canonical.Prompt() != "repeatable textile pattern" {
		t.Fatalf("Prompt = %q, want user prompt unchanged", canonical.Prompt())
	}
}

func TestTaskIntakeRequiresImageFillSystemPrompt(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
	})
	tests := []struct {
		feature string
		params  map[string]interface{}
	}{
		{
			feature: workflow.FeatureSlugInpaint,
			params: map[string]interface{}{
				"original_image_width": 1024, "original_image_height": 768,
				"custom_width": 1024, "custom_height": 768,
			},
		},
		{
			feature: workflow.FeatureSlugOutpaint,
			params: map[string]interface{}{
				"original_image_width": 1024, "original_image_height": 768,
				"outpaint": map[string]interface{}{
					"target_width": 1280, "target_height": 768,
					"image_offset_x": 0, "image_offset_y": 0,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.feature, func(t *testing.T) {
			_, err := intake.CompileReadiness(t.Context(), 42, &tasks.SubmitTaskRequest{
				Feature: test.feature,
				Params:  test.params,
			})
			if err == nil || !strings.Contains(err.Error(), "未配置 System Prompt") {
				t.Fatalf("CompileReadiness() error = %v", err)
			}
		})
	}
}

func TestTaskIntakePreservesPromptCauseBehindStableError(t *testing.T) {
	cause := errors.New("database connection refused")
	intake := NewTaskIntake(TaskIntakeDeps{
		Models:  &fakeIntakeModelStore{},
		Prompts: fakeFeaturePromptStore{err: cause},
		Media:   testSubmitMediaValidator{},
	})

	_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugGeneration,
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

func TestTaskIntakeReadinessFailsFastWithoutModelMapping(t *testing.T) {
	// T09 pin: an unmapped feature fails closed at readiness with
	// provider_unavailable, before any billing or submission work.
	intake := NewTaskIntake(TaskIntakeDeps{
		Models:  &fakeIntakeModelStore{err: errors.New("AI gateway model mapping is not configured for this feature")},
		Prompts: fakeFeaturePromptStore{},
		Media:   testSubmitMediaValidator{},
	})

	_, err := intake.CompileReadiness(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugGeneration,
		ClientActionID: "unmapped-feature",
	})
	if err == nil {
		t.Fatal("CompileReadiness succeeded without a model mapping, want provider_unavailable")
	}
	var submissionErr *SubmissionError
	if !errors.As(err, &submissionErr) {
		t.Fatalf("readiness error = %T, want SubmissionError", err)
	}
	if submissionErr.Code != "provider_unavailable" {
		t.Fatalf("readiness code = %q, want provider_unavailable", submissionErr.Code)
	}
}
