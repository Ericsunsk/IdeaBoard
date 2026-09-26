package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func TestCompileWorkspaceParamsPersistsDurableExecutionFacts(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "durability probe", map[string]interface{}{
		"output_count": 2,
	})
	if err != nil {
		t.Fatalf("compile workspace params: %v", err)
	}
	params, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("decode workspace params: %v", err)
	}
	if params.Generation == nil {
		t.Fatal("workspace params missing generation snapshot")
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(params.ExecutionPlan)
	if err != nil {
		t.Fatalf("decode execution plan: %v", err)
	}
	if plan.Feature() != workflow.FeatureSlugGeneration || plan.RequestedOutputCount().Int() != 2 {
		t.Fatalf("plan=%+v", plan)
	}
	if params.Controls == nil || params.Controls.OutputCount != 2 || params.Generation.RequestedOutputCount != 2 {
		t.Fatalf("controls=%+v generation=%+v", params.Controls, params.Generation)
	}
}

func TestCompileWorkspaceParamsCarriesCanonicalWorkspaceMedia(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugRender, "", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input123"},
	})
	if err != nil {
		t.Fatalf("compile workspace params: %v", err)
	}
	params, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("decode workspace params: %v", err)
	}
	if params.Media == nil || len(params.Media.Inputs) != 1 || params.Media.Inputs[0].PublicAssetID != "asset_input123" {
		t.Fatalf("media=%+v, want one public workspace input", params.Media)
	}
	if params.Generation == nil || len(params.Generation.Media) != 1 || params.Generation.Media[0].PublicAssetID != "asset_input123" {
		t.Fatalf("generation=%+v, want one public workspace input", params.Generation)
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(params.ExecutionPlan)
	if err != nil {
		t.Fatalf("decode execution plan: %v", err)
	}
	if plan.RuntimeInputs().ImageInputCount() != 1 || plan.RuntimeInputs().GenerationPrimitive() != workflow.GenerationPrimitiveImageToImage {
		t.Fatalf("runtime inputs=%+v, want one image-to-image input", plan.RuntimeInputs())
	}
	if _, exists := params.Extra[CompilerWorkspaceMediaAssetIDsParam]; exists {
		t.Fatalf("compiler-only key leaked into Extra: %+v", params.Extra)
	}
}

// The canonical intake ordering is map → typed → plan → persisted JSON with
// plan-derived facts stamped before the typed conversion. These tests lock
// that sequence at the stage level so future edits cannot silently reorder it.

func TestCanonicalSubmitOrderingStampsPlanFactsBeforeTypedConversion(t *testing.T) {
	t.Run("plan defaults fill omitted caller values", func(t *testing.T) {
		params := map[string]interface{}{"feature": workflow.FeatureSlugGeneration}
		if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugGeneration); err != nil {
			t.Fatal(err)
		}
		plan, err := compileSubmitExecutionPlan(params, workflow.FeatureSlugGeneration)
		if err != nil {
			t.Fatal(err)
		}
		typed, err := finalizeSubmitTypedParams(params, workflow.FeatureSlugGeneration, plan)
		if err != nil {
			t.Fatal(err)
		}
		if plan.RequestedOutputCount().Int() != 1 {
			t.Fatalf("plan output count=%d, want default 1", plan.RequestedOutputCount().Int())
		}
		if typed.Controls == nil || typed.Controls.OutputCount != 1 {
			t.Fatalf("typed controls=%+v, want output_count 1 from plan", typed.Controls)
		}
		if params["output_count"] != 1 {
			t.Fatalf("params output_count=%v, want plan-stamped 1", params["output_count"])
		}
		decoded, err := workflow.DecodeFeatureExecutionPlan(typed.ExecutionPlan)
		if err != nil {
			t.Fatal(err)
		}
		wantPlan, err := workflow.EncodeFeatureExecutionPlan(plan)
		if err != nil {
			t.Fatal(err)
		}
		gotPlan, err := workflow.EncodeFeatureExecutionPlan(decoded)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotPlan) != string(wantPlan) {
			t.Fatalf("attached plan drifted:\n got=%s\nwant=%s", gotPlan, wantPlan)
		}
	})

	t.Run("caller output values flow through plan into typed params", func(t *testing.T) {
		params := map[string]interface{}{
			"feature":           workflow.FeatureSlugGeneration,
			"output_count":      3,
			"output_resolution": "2k",
		}
		if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugGeneration); err != nil {
			t.Fatal(err)
		}
		plan, err := compileSubmitExecutionPlan(params, workflow.FeatureSlugGeneration)
		if err != nil {
			t.Fatal(err)
		}
		typed, err := finalizeSubmitTypedParams(params, workflow.FeatureSlugGeneration, plan)
		if err != nil {
			t.Fatal(err)
		}
		if typed.Controls == nil || typed.Controls.OutputCount != 3 {
			t.Fatalf("typed controls=%+v, want output_count 3 from plan", typed.Controls)
		}
		if plan.OutputResolution() != "2k" {
			t.Fatalf("plan resolution=%q, want 2k", plan.OutputResolution())
		}
	})
}

func TestCanonicalSubmitOrderingStripsModelAndStampsFeature(t *testing.T) {
	params := map[string]interface{}{
		"model":             "gpt-image-1",
		"provider_specific": "keep-me",
	}
	if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugRender); err != nil {
		t.Fatal(err)
	}
	if params["feature"] != workflow.FeatureSlugRender {
		t.Fatalf("feature=%v, want stamped render", params["feature"])
	}
	if _, exists := params["model"]; exists {
		t.Fatalf("model survived stage 1: %#v", params)
	}
	typed, err := workflowTaskParamsFromMap(params, workflow.FeatureSlugRender)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := typed.Extra["model"]; exists {
		t.Fatalf("model leaked into typed extra: %#v", typed.Extra)
	}
	if _, exists := typed.Extra["feature"]; exists {
		t.Fatalf("feature leaked into typed extra: %#v", typed.Extra)
	}
	if got := typed.Extra["provider_specific"]; got != "keep-me" {
		t.Fatalf("provider_specific=%v, want keep-me", got)
	}
}

func TestCanonicalSubmitOrderingNormalizesFusionBeforeMediaWeights(t *testing.T) {
	params := map[string]interface{}{
		"feature":    workflow.FeatureSlugFusion,
		"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
		"fusion": map[string]interface{}{
			"input_weights": []interface{}{3.0, 1.0},
		},
	}
	if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugFusion); err != nil {
		t.Fatal(err)
	}
	plan, err := compileSubmitExecutionPlan(params, workflow.FeatureSlugFusion)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := finalizeSubmitTypedParams(params, workflow.FeatureSlugFusion, plan)
	if err != nil {
		t.Fatal(err)
	}
	if typed.Fusion == nil || typed.Fusion.Mode != "weighted_reference" {
		t.Fatalf("typed fusion=%+v, want normalized weighted_reference", typed.Fusion)
	}
	if len(typed.Fusion.InputWeights) != 2 || typed.Fusion.InputWeights[0] != 0.75 || typed.Fusion.InputWeights[1] != 0.25 {
		t.Fatalf("fusion weights=%v, want normalized [0.75 0.25]", typed.Fusion.InputWeights)
	}
	if typed.Media == nil || len(typed.Media.Inputs) != 2 {
		t.Fatalf("media inputs=%+v, want two weighted inputs", typed.Media)
	}
	if typed.Media.Inputs[0].Weight != 0.75 || typed.Media.Inputs[1].Weight != 0.25 {
		t.Fatalf("media weights=%v %v, want fusion-normalized weights",
			typed.Media.Inputs[0].Weight, typed.Media.Inputs[1].Weight)
	}
}

func TestCanonicalSubmitOrderingRejectsGenerationModeBeforePlan(t *testing.T) {
	params := map[string]interface{}{
		"feature": workflow.FeatureSlugGeneration,
		"mode":    "text-to-image",
	}
	err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugGeneration)
	if err == nil || !strings.Contains(err.Error(), "不接受显式 mode") {
		t.Fatalf("stage 1 error=%v, want explicit-mode contract rejection before any plan", err)
	}
}

func TestCanonicalSubmitOrderingPersistsPlanMatchingJSON(t *testing.T) {
	params := map[string]interface{}{
		"feature":      workflow.FeatureSlugRender,
		"image_urls":   []string{"https://example.com/sketch.png"},
		"output_count": 2,
	}
	if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugRender); err != nil {
		t.Fatal(err)
	}
	plan, err := compileSubmitExecutionPlan(params, workflow.FeatureSlugRender)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := finalizeSubmitTypedParams(params, workflow.FeatureSlugRender, plan)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := marshalWorkflowTaskParams(typed, params)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := tasks.UnmarshalParams(persisted)
	if err != nil {
		t.Fatal(err)
	}
	decodedPlan, err := workflow.DecodeFeatureExecutionPlan(decoded.ExecutionPlan)
	if err != nil {
		t.Fatal(err)
	}
	want, err := workflow.EncodeFeatureExecutionPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	got, err := workflow.EncodeFeatureExecutionPlan(decodedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("persisted plan drifted:\n got=%s\nwant=%s", got, want)
	}
	if decoded.Controls == nil || decoded.Controls.OutputCount != 2 {
		t.Fatalf("persisted controls=%+v, want output_count 2", decoded.Controls)
	}
}

func TestCanonicalReadinessOrderingDoesNotStampPlanFacts(t *testing.T) {
	params := map[string]interface{}{"feature": workflow.FeatureSlugInpaint}
	plan, err := compileReadinessExecutionPlan(params, workflow.FeatureSlugInpaint)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := params["output_count"]; exists {
		t.Fatalf("readiness stamped output_count into caller map: %#v", params)
	}
	if _, exists := params["output_resolution"]; exists {
		t.Fatalf("readiness stamped output_resolution into caller map: %#v", params)
	}
	if plan.RuntimeInputs().ImageInputCount() != 1 || !plan.RuntimeInputs().HasMaskInput() {
		t.Fatalf("readiness plan lacks synthesized media facts: inputs=%d mask=%v",
			plan.RuntimeInputs().ImageInputCount(), plan.RuntimeInputs().HasMaskInput())
	}
	typed, err := workflowTaskParamsFromMap(params, workflow.FeatureSlugInpaint)
	if err != nil {
		t.Fatal(err)
	}
	if len(typed.ExecutionPlan) != 0 {
		t.Fatalf("readiness typed params must not attach a persisted plan: %s", typed.ExecutionPlan)
	}
}

func TestCanonicalSubmitOrderingEndToEndFactsAgree(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Models: &fakeIntakeModelStore{},
		Media:  testSubmitMediaValidator{},
	})
	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "draw a chair",
		Feature:        workflow.FeatureSlugRender,
		ImageURLs:      []string{"https://example.com/sketch.png"},
		ClientActionID: "ordering-e2e",
		Params: map[string]interface{}{
			"model":        "gpt-image-1",
			"output_count": 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	typed, err := tasks.UnmarshalParams(canonical.Params())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(typed.ExecutionPlan)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := canonical.GenerationSnapshot()
	if snapshot == nil {
		t.Fatal("missing generation snapshot")
	}
	billing := canonical.BillingRequest()
	if billing == nil {
		t.Fatal("missing billing request")
	}
	counts := map[string]int{
		"plan":       plan.RequestedOutputCount().Int(),
		"typed":      typed.Controls.OutputCount,
		"generation": snapshot.RequestedOutputCount,
		"billing":    int(billing.Quantity),
	}
	for name, count := range counts {
		if count != 2 {
			t.Fatalf("output count disagreement at %s: %#v", name, counts)
		}
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal([]byte(canonical.Params()), &persisted); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"model", "image_urls", "mask_image_url", "mask_image_urls", "original_image_url"} {
		if _, exists := persisted[legacy]; exists {
			t.Fatalf("persisted params leaked %q: %s", legacy, canonical.Params())
		}
	}
	if _, exists := persisted["execution_plan"]; !exists {
		t.Fatalf("persisted params lack execution_plan: %s", canonical.Params())
	}
}
