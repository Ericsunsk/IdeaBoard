package task

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"ideaboard/api/internal/prompts"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

// emptyPromptConfig is the template-less runtime config: intake degrades to
// straight passthrough (compiled = user prompt), preserving pre-template
// snapshot semantics.
func emptyPromptConfig() prompts.FeaturePromptRuntimeConfig {
	return prompts.FeaturePromptRuntimeConfig{}
}

func TestCompileWorkspaceParamsRejectsInvalidAutoInputDimensions(t *testing.T) {
	for _, width := range []any{nil, 0, -1, 466.5, float64(1 << 40), math.Inf(1), math.NaN()} {
		params := map[string]interface{}{
			"ratio": "auto", "original_image_height": 516,
			CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		}
		if width != nil {
			params["original_image_width"] = width
		}
		if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "follow the input", params, emptyPromptConfig()); err == nil {
			t.Errorf("original_image_width = %v, want validation error before submission", width)
		}
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "follow the input", map[string]interface{}{
		"ratio": "auto", "original_image_width": 1, "original_image_height": 16384,
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
	}, emptyPromptConfig()); err == nil {
		t.Fatal("aspect ratio that rounds to zero pixels must fail before submission")
	}
}

// promptConfigForFeature builds the runtime config an Admin-saved template
// would produce: the saved prompt plus the code-owned merge policy.
func promptConfigForFeature(slug, template string) prompts.FeaturePromptRuntimeConfig {
	return prompts.FeaturePromptRuntimeConfig{
		Prompt:                template,
		UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug),
	}
}

func TestCompileWorkspaceParamsPersistsDurableExecutionFacts(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "durability probe", map[string]interface{}{
		"output_count": 2,
	}, emptyPromptConfig())
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
	}, emptyPromptConfig())
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
	}, emptyPromptConfig())
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
	}, emptyPromptConfig())
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
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "   ", map[string]interface{}{}, emptyPromptConfig()); err == nil {
		t.Fatal("empty prompt on generation = nil, want error")
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"mask_asset_id":                     "asset_mask1",
	}, emptyPromptConfig()); err == nil {
		t.Fatal("empty prompt on inpaint = nil, want error")
	}
	over := strings.Repeat("x", tasks.MaxWorkspaceTaskPromptBytes+1)
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, over, map[string]interface{}{}, emptyPromptConfig()); err == nil {
		t.Fatal("over-long prompt = nil, want error")
	}
}

// Prompt-ignored features (cutout, upscale) drop user text: binary media
// operations never consume it (documented).
func TestCompileWorkspaceParamsDropsPromptWhenIgnored(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugUpscale, "should be dropped", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"upscale":                           map[string]interface{}{"scale": 2.0},
	}, emptyPromptConfig())
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
	}, emptyPromptConfig())
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
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "fill it", withInput(map[string]interface{}{}), emptyPromptConfig()); err == nil {
		t.Fatal("inpaint without mask = nil, want error")
	}
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugRender, "edit it", withInput(map[string]interface{}{
		"mask_asset_id": "asset_mask1",
	}), emptyPromptConfig()); err == nil {
		t.Fatal("mask on render = nil, want error")
	}
	for _, bad := range []any{"data:image/png;base64,AAAA", "https://cdn.example.com/m.png", "nope", 42, ""} {
		if _, err := CompileWorkspaceParams(workflow.FeatureSlugInpaint, "fill it", withInput(map[string]interface{}{
			"mask_asset_id": bad,
		}), emptyPromptConfig()); err == nil {
			t.Fatalf("mask %v = nil, want error", bad)
		}
	}
}

// Plan-consistency is submit-owned (C4): tampered durable facts must fail
// closed in validateSubmitParamsAgainstPlan. Each case starts from a valid
// CompileWorkspaceParams baseline, tampers one plan-bound fact, and requires
// the submit-side validator to reject it.
func TestSubmitParamsAgainstPlanRejectsIllegalPlans(t *testing.T) {
	upscaleBaseline := func(t *testing.T) (*tasks.WorkflowTaskParams, workflow.FeatureExecutionPlan) {
		t.Helper()
		raw, err := CompileWorkspaceParams(workflow.FeatureSlugUpscale, "ignored prompt", map[string]interface{}{
			CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
			"upscale":                           map[string]interface{}{"scale": 2.0},
		}, emptyPromptConfig())
		if err != nil {
			t.Fatalf("compile upscale baseline: %v", err)
		}
		typed, err := tasks.UnmarshalParams(raw)
		if err != nil {
			t.Fatalf("decode upscale baseline: %v", err)
		}
		plan, err := workflow.DecodeFeatureExecutionPlan(typed.ExecutionPlan)
		if err != nil {
			t.Fatalf("decode upscale plan: %v", err)
		}
		if err := validateSubmitParamsAgainstPlan(typed, plan); err != nil {
			t.Fatalf("valid baseline rejected: %v", err)
		}
		return typed, plan
	}
	t.Run("output count mismatch", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Controls.OutputCount++
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("tampered output count accepted, want fail-closed")
		}
	})
	t.Run("generation snapshot service mismatch", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Generation.Service = workflow.FeatureSlugRender
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("tampered snapshot service accepted, want fail-closed")
		}
	})
	t.Run("generation snapshot primitive mismatch", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Generation.Primitive = string(workflow.GenerationPrimitiveTextToImage)
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("tampered snapshot primitive accepted, want fail-closed")
		}
	})
	t.Run("media dropped below plan", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Media.Inputs = nil
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("dropped media accepted, want fail-closed")
		}
	})
	t.Run("foreign control presence", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Variation = &tasks.VariationsParams{Strength: 50}
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("foreign variation on upscale accepted, want fail-closed")
		}
	})
	t.Run("missing required control", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Upscale = nil
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("missing upscale control accepted, want fail-closed")
		}
	})
	t.Run("upscale scale out of range", func(t *testing.T) {
		typed, plan := upscaleBaseline(t)
		typed.Upscale.Scale = 8
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("out-of-range scale accepted, want fail-closed")
		}
	})
	t.Run("fusion weights diverge from inputs", func(t *testing.T) {
		// URL-source media lose Source on JSON round-trip (json:"-"), so
		// build the baseline in-memory through the canonical stages and
		// validate pre-marshal, exactly where CompileWorkspaceParams calls
		// the validator.
		params := map[string]interface{}{
			"service":    workflow.FeatureSlugFusion,
			"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
			"fusion":     map[string]interface{}{"input_weights": []interface{}{3.0, 1.0}},
		}
		if err := normalizeSubmitExecutionMap(params, workflow.FeatureSlugFusion); err != nil {
			t.Fatalf("normalize fusion baseline: %v", err)
		}
		plan, err := compileSubmitExecutionPlan(params, workflow.FeatureSlugFusion)
		if err != nil {
			t.Fatalf("compile fusion plan: %v", err)
		}
		typed, err := finalizeSubmitTypedParams(params, workflow.FeatureSlugFusion, plan)
		if err != nil {
			t.Fatalf("finalize fusion baseline: %v", err)
		}
		snapshot, err := tasksubmission.CompileGenerationSnapshot(tasksubmission.GenerationCompilerInput{
			Plan: plan, UserPrompt: "fuse", CompiledPrompt: "fuse",
			TemplateFingerprint: tasksubmission.TemplateFingerprint("", "fuse"), Params: typed,
		})
		if err != nil {
			t.Fatalf("compile fusion snapshot: %v", err)
		}
		typed.Generation = &snapshot
		if err := validateSubmitParamsAgainstPlan(typed, plan); err != nil {
			t.Fatalf("valid fusion baseline rejected: %v", err)
		}
		typed.Fusion.InputWeights = append(typed.Fusion.InputWeights, 0.1)
		if err := validateSubmitParamsAgainstPlan(typed, plan); err == nil {
			t.Fatal("diverged fusion weights accepted, want fail-closed")
		}
	})
}

// System template ⊕ user prompt: the configured template renders at intake
// into CompiledPrompt while UserPrompt keeps the raw user text.
func TestCompileWorkspaceParamsRendersSystemTemplate(t *testing.T) {
	decode := func(t *testing.T, raw string) *tasks.WorkflowTaskParams {
		t.Helper()
		params, err := tasks.UnmarshalParams(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if params.Generation == nil {
			t.Fatal("missing generation snapshot")
		}
		return params
	}
	t.Run("required renders placeholder template", func(t *testing.T) {
		raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "a red bicycle", map[string]interface{}{
			"output_count": 1,
		}, promptConfigForFeature(workflow.FeatureSlugGeneration, "SYS style {{user_prompt}}"))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		params := decode(t, raw)
		if params.Generation.UserPrompt != "a red bicycle" {
			t.Fatalf("user=%q, want raw user text", params.Generation.UserPrompt)
		}
		if params.Generation.CompiledPrompt != "SYS style a red bicycle" {
			t.Fatalf("compiled=%q, want rendered template", params.Generation.CompiledPrompt)
		}
		if params.Generation.SystemPrompt != "SYS style {{user_prompt}}" {
			t.Fatalf("system=%q, want configured template", params.Generation.SystemPrompt)
		}
	})
	t.Run("non-image feature appends base prompt without placeholder", func(t *testing.T) {
		raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "a red bicycle", map[string]interface{}{
			"output_count": 1,
		}, promptConfigForFeature(workflow.FeatureSlugGeneration, "SYS"))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if got := decode(t, raw).Generation.CompiledPrompt; got != "SYS a red bicycle" {
			t.Fatalf("compiled=%q, want appended base prompt", got)
		}
	})
	t.Run("image node never implicitly appends without placeholder", func(t *testing.T) {
		raw, err := CompileWorkspaceParams(workflow.FeatureSlugRender, "", map[string]interface{}{
			CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input123"},
		}, promptConfigForFeature(workflow.FeatureSlugRender, "SYS"))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		params := decode(t, raw)
		if params.Generation.CompiledPrompt != "SYS" {
			t.Fatalf("compiled=%q, want template only", params.Generation.CompiledPrompt)
		}
		if params.Generation.UserPrompt != "" {
			t.Fatalf("user=%q, want empty", params.Generation.UserPrompt)
		}
	})
	t.Run("empty template degrades to passthrough", func(t *testing.T) {
		without, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "a red bicycle", map[string]interface{}{
			"output_count": 1,
		}, emptyPromptConfig())
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		params := decode(t, without)
		if params.Generation.CompiledPrompt != "a red bicycle" || params.Generation.SystemPrompt != "" {
			t.Fatalf("generation=%+v, want passthrough with empty system", params.Generation)
		}
	})
}

// Prompt-ignored features never inject the system template: the snapshot
// stays double-empty even when a template is configured.
func TestCompileWorkspaceParamsIgnoresTemplateWhenPromptIgnored(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugUpscale, "should be dropped", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"},
		"upscale":                           map[string]interface{}{"scale": 2.0},
	}, promptConfigForFeature(workflow.FeatureSlugUpscale, "SYS {{user_prompt}}"))
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
	if params.Generation.UserPrompt != "" || params.Generation.CompiledPrompt != "" || params.Generation.SystemPrompt != "" {
		t.Fatalf("generation=%+v, want empty snapshot", params.Generation)
	}
}

// Fingerprint covers template content: the same user prompt with a
// different template must produce a different fingerprint and seed.
func TestCompileWorkspaceParamsFingerprintChangesWithTemplate(t *testing.T) {
	compile := func(t *testing.T, template string) *tasks.WorkflowTaskParams {
		t.Helper()
		raw, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, "same prompt", map[string]interface{}{
			"output_count": 1,
		}, promptConfigForFeature(workflow.FeatureSlugGeneration, template))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		params, err := tasks.UnmarshalParams(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return params
	}
	a := compile(t, "SYS-A {{user_prompt}}")
	b := compile(t, "SYS-B {{user_prompt}}")
	if a.Generation.TemplateFingerprint == "" || a.Generation.TemplateFingerprint == b.Generation.TemplateFingerprint {
		t.Fatalf("fingerprints must differ when the template changes: %q vs %q",
			a.Generation.TemplateFingerprint, b.Generation.TemplateFingerprint)
	}
	if a.Generation.Seed == b.Generation.Seed {
		t.Fatal("seeds must differ when the template changes")
	}
}

// End-to-end snapshot assertion: variations (PromptPolicyOptional) with a
// configured system template and an empty user prompt still compiles to a
// non-empty provider prompt (the Vertex 422 case).
func TestCompileWorkspaceParamsVariationsEmptyPromptWithTemplateCompiles(t *testing.T) {
	raw, err := CompileWorkspaceParams(workflow.FeatureSlugVariations, "", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input123"},
		"variation":                         map[string]interface{}{"strength": 40},
	}, promptConfigForFeature(workflow.FeatureSlugVariations, "VARIATION_RENDER strength={{variation_strength}} {{user_prompt}}"))
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
	if params.Generation.UserPrompt != "" {
		t.Fatalf("user=%q, want empty", params.Generation.UserPrompt)
	}
	if params.Generation.CompiledPrompt != "VARIATION_RENDER strength=40" {
		t.Fatalf("compiled=%q, want rendered template with bound strength", params.Generation.CompiledPrompt)
	}
}
