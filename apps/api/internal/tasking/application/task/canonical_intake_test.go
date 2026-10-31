package task

import (
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
		params := map[string]interface{}{"service": workflow.FeatureSlugGeneration}
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
			"service":           workflow.FeatureSlugGeneration,
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

func TestCanonicalSubmitOrderingStripsModelAndStampsService(t *testing.T) {
	params := map[string]interface{}{
		"model":             "gpt-image-1",
		"provider_specific": "keep-me",
	}
	if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugRender); err != nil {
		t.Fatal(err)
	}
	if params["service"] != workflow.FeatureSlugRender {
		t.Fatalf("service=%v, want stamped render", params["service"])
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
	if _, exists := typed.Extra["service"]; exists {
		t.Fatalf("service leaked into typed extra: %#v", typed.Extra)
	}
	if got := typed.Extra["provider_specific"]; got != "keep-me" {
		t.Fatalf("provider_specific=%v, want keep-me", got)
	}
}

func TestCanonicalSubmitOrderingNormalizesFusionBeforeMediaWeights(t *testing.T) {
	params := map[string]interface{}{
		"service":    workflow.FeatureSlugFusion,
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

func TestCanonicalSubmitOrderingEndToEndFactsAgree(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugRender, "draw a chair", map[string]interface{}{
		"output_count": 2,
		"image_urls":   []string{"https://example.com/sketch.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	typed, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(typed.ExecutionPlan)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := typed.Generation
	if snapshot == nil {
		t.Fatal("missing generation snapshot")
	}
	counts := map[string]int{
		"plan":       plan.RequestedOutputCount().Int(),
		"typed":      typed.Controls.OutputCount,
		"generation": snapshot.RequestedOutputCount,
		"billing":    int(snapshot.RequestedOutputCount),
	}
	for name, count := range counts {
		if count != 2 {
			t.Fatalf("output count disagreement at %s: %#v", name, counts)
		}
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"model", "image_urls", "mask_image_url", "mask_image_urls", "original_image_url"} {
		if _, exists := persisted[legacy]; exists {
			t.Fatalf("persisted params leaked %q: %s", legacy, raw)
		}
	}
	if _, exists := persisted["execution_plan"]; !exists {
		t.Fatalf("persisted params lack execution_plan: %s", raw)
	}
}

// merged from canonical_prompt_mask_test.go (canonical_intake.go aggregation: CompileWorkspaceParams prompt/mask threading).
// v1 data.prompt threads into the Generation snapshot (UserPrompt +
// CompiledPrompt) that the worker bridges to the provider.
func TestCompileWorkspaceParamsThreadsPrompt(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "a red bicycle", map[string]interface{}{
		"output_count": 1,
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	params, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if params.Generation == nil {
		t.Fatal("missing generation snapshot")
	}
	if params.Generation.UserPrompt != "a red bicycle" || params.Generation.CompiledPrompt != "a red bicycle" {
		t.Fatalf("generation=%+v, want the v1 prompt in both fields", params.Generation)
	}
}

// Prompt-required features (generation, inpaint) reject an empty prompt;
// the bound is tasks.MaxWorkspaceTaskPromptBytes (8KB).
func TestCompileWorkspaceParamsRejectsMissingPrompt(t *testing.T) {
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "   ", map[string]interface{}{}); err == nil {
		t.Fatal("empty prompt on generation = nil, want error")
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"mask_asset_id":                     "asset_mask1",
	}); err == nil {
		t.Fatal("empty prompt on inpaint = nil, want error")
	}
	over := strings.Repeat("x", tasks.MaxWorkspaceTaskPromptBytes+1)
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, over, map[string]interface{}{}); err == nil {
		t.Fatal("over-long prompt = nil, want error")
	}
}

// Prompt-ignored features (cutout, upscale) drop user text: binary media
// operations never consume it (documented).
func TestCompileWorkspaceParamsDropsPromptWhenIgnored(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugUpscale, "should be dropped", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"upscale":                           map[string]interface{}{"scale": 2.0},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	params, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if params.Generation == nil {
		t.Fatal("missing generation snapshot")
	}
	if params.Generation.UserPrompt != "" || params.Generation.CompiledPrompt != "" {
		t.Fatalf("generation=%+v, want dropped prompt", params.Generation)
	}
}

// Inpaint carries the v1 mask reference (upload-session asset id) into the
// canonical media facts: typed Media.Mask + generation intent + plan mask
// input, with no compiler-only leakage into Extra.
func TestCompileWorkspaceParamsCarriesMaskAsset(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "fill the hole", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"mask_asset_id":                     "asset_mask1",
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	params, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if params.Media == nil || params.Media.Mask == nil || params.Media.Mask.PublicAssetID != "asset_mask1" {
		t.Fatalf("media=%+v, want mask public asset id", params.Media)
	}
	if params.Generation == nil || len(params.Generation.Media) != 2 {
		t.Fatalf("generation=%+v, want input + mask intents", params.Generation)
	}
	maskIntent := params.Generation.Media[1]
	if maskIntent.Role != tasks.MediaRoleMask || maskIntent.PublicAssetID != "asset_mask1" {
		t.Fatalf("mask intent=%+v, want mask role with public asset id", maskIntent)
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(params.ExecutionPlan)
	if err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if !plan.RuntimeInputs().HasMaskInput() || plan.RuntimeInputs().ImageInputCount() != 1 {
		t.Fatalf("runtime inputs=%+v, want one image plus mask", plan.RuntimeInputs())
	}
	if params.Generation.UserPrompt != "fill the hole" || params.Generation.CompiledPrompt != "fill the hole" {
		t.Fatalf("generation=%+v, want the v1 prompt", params.Generation)
	}
	if _, exists := params.Extra[CompilerWorkspaceMediaAssetIDsParam]; exists {
		t.Fatalf("compiler-only key leaked into Extra: %+v", params.Extra)
	}
	if _, exists := params.Extra["mask_asset_id"]; exists {
		t.Fatalf("mask key leaked into Extra: %+v", params.Extra)
	}
}

// Mask policy is plan-enforced: inpaint requires a mask, every other
// feature rejects one; malformed mask references fail before any plan work.
func TestCompileWorkspaceParamsEnforcesMaskPolicy(t *testing.T) {
	withInput := func(params map[string]interface{}) map[string]interface{} {
		params[CompilerWorkspaceMediaAssetIDsParam] = []string{"asset_input1"}
		return params
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "fill it", withInput(map[string]interface{}{})); err == nil {
		t.Fatal("inpaint without mask = nil, want error")
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugRender, "edit it", withInput(map[string]interface{}{
		"mask_asset_id": "asset_mask1",
	})); err == nil {
		t.Fatal("mask on render = nil, want error")
	}
	for _, bad := range []any{"data:image/png;base64,AAAA", "https://cdn.example.com/m.png", "nope", 42, ""} {
		if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "fill it", withInput(map[string]interface{}{
			"mask_asset_id": bad,
		})); err == nil {
			t.Fatalf("mask %v = nil, want error", bad)
		}
	}
}
