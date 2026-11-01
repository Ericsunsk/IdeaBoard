package submission

import (
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func TestCompileGenerationSnapshotFreezesIntentAndMediaOrder(t *testing.T) {
	upsampling := true
	params := &tasks.WorkflowTaskParams{
		Service:  "inpaint",
		Controls: &tasks.GenerationControls{OutputCount: 1, PromptUpsampling: &upsampling},
		Media: &tasks.MediaInputs{
			Original: &tasks.MediaRef{Source: " upload-a ", Role: "original"},
			Mask:     &tasks.MediaRef{Source: "mask-b", Role: "mask"},
		},
	}
	plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: workflow.FeatureSlugInpaint,
		Params: workflow.FeatureExecutionParams{
			ImageInputCount: 1,
			HasMaskInput:    true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := CompileGenerationSnapshot(GenerationCompilerInput{
		Plan:                plan,
		UserPrompt:          "edit this",
		SystemPrompt:        "SYS",
		CompiledPrompt:      "SYS edit this",
		TemplateVersion:     "v7",
		TemplateFingerprint: "fingerprint-v7",
		SeedMaterial:        "action-123",
		Params:              params,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Primitive != "image-to-image" || snapshot.RequestedOutputCount != 1 || snapshot.SystemPrompt != "SYS" || snapshot.Seed == 0 {
		t.Fatalf("snapshot semantics = %+v", snapshot)
	}
	if len(snapshot.Media) != 2 || snapshot.Media[0].InputIndex != 0 || snapshot.Media[0].Source != "upload-a" || snapshot.Media[1].Role != "mask" {
		t.Fatalf("snapshot media = %+v", snapshot.Media)
	}
	params.Controls.OutputCount = 4
	if snapshot.Controls == nil || snapshot.Controls.OutputCount != 1 {
		t.Fatalf("controls were not frozen: %+v", snapshot.Controls)
	}
}

func TestTemplateFingerprintChangesWhenTemplateChanges(t *testing.T) {
	a := TemplateFingerprint("v1", "template A")
	b := TemplateFingerprint("v1", "template B")
	c := TemplateFingerprint("v2", "template A")
	if a == "" || a == b || a == c {
		t.Fatalf("fingerprints must be stable and content-addressed: a=%q b=%q c=%q", a, b, c)
	}
}

func TestCompileGenerationSnapshotFallbackFingerprintCoversSystemTemplate(t *testing.T) {
	plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: workflow.FeatureSlugGeneration,
		Params:  workflow.FeatureExecutionParams{},
	})
	if err != nil {
		t.Fatal(err)
	}
	compile := func(system string) tasks.GenerationRequestSnapshot {
		t.Helper()
		snapshot, err := CompileGenerationSnapshot(GenerationCompilerInput{
			Plan: plan, UserPrompt: "prompt", SystemPrompt: system,
			CompiledPrompt: "SYS prompt", Params: nil,
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		return snapshot
	}
	a := compile("SYS-A")
	b := compile("SYS-B")
	if a.TemplateFingerprint == "" || a.TemplateFingerprint == b.TemplateFingerprint {
		t.Fatalf("fallback fingerprints must differ when the template changes: %q vs %q",
			a.TemplateFingerprint, b.TemplateFingerprint)
	}
	without := compile("")
	with := compile("SYS-A")
	if without.TemplateFingerprint != TemplateFingerprint("", "SYS prompt") {
		t.Fatal("template-less fingerprint must preserve the compiled-prompt fingerprint")
	}
	if a.TemplateFingerprint != with.TemplateFingerprint || a.Seed != with.Seed {
		t.Fatal("identical template content must produce a stable snapshot")
	}
	if a.Seed == b.Seed {
		t.Fatal("template changes must change the derived seed")
	}
}
