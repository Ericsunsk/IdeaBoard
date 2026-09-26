package task

import (
	"strings"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow"
)

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
// the bound is workspacetasks.MaxWorkspaceTaskPromptBytes (8KB).
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
	over := strings.Repeat("x", workspacetasks.MaxWorkspaceTaskPromptBytes+1)
	if _, err := CompileWorkspaceParams(workflow.FeatureSlugGeneration, over, map[string]interface{}{}); err == nil {
		t.Fatal("over-long prompt = nil, want error")
	}
}

// Prompt-ignored features (cutout, upscale) drop user text: binary media
// operations never consume it (legacy parity, documented).
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
