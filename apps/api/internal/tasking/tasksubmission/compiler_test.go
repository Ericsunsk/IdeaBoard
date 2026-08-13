package tasksubmission

import (
	"testing"

	"ideaboard/api/internal/tasking/tasks"
)

func TestCompileGenerationSnapshotFreezesIntentAndMediaOrder(t *testing.T) {
	upsampling := true
	params := &tasks.WorkflowTaskParams{
		Feature:  "image-edit",
		Controls: &tasks.GenerationControls{OutputCount: 3, PromptUpsampling: &upsampling},
		Media: &tasks.MediaInputs{
			Inputs:     []tasks.MediaRef{{URL: " upload-a ", Role: "input"}},
			References: []tasks.MediaRef{{URL: "reference-b", Role: "reference", Weight: 0.4}},
			Mask:       &tasks.MediaRef{URL: "mask-c", Role: "mask"},
		},
	}
	snapshot, err := CompileGenerationSnapshot(GenerationCompilerInput{
		Feature:         "image-edit",
		UserPrompt:      "edit this",
		CompiledPrompt:  "SYS edit this",
		TemplateVersion: "v7",
		Params:          params,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Primitive != "image-to-image" || snapshot.RequestedOutputCount != 3 {
		t.Fatalf("snapshot semantics = %+v", snapshot)
	}
	if len(snapshot.Media) != 3 || snapshot.Media[0].InputIndex != 0 || snapshot.Media[0].Ref != "upload-a" || snapshot.Media[2].Role != "mask" {
		t.Fatalf("snapshot media = %+v", snapshot.Media)
	}
	params.Controls.OutputCount = 1
	if snapshot.Controls == nil || snapshot.Controls.OutputCount != 3 {
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
