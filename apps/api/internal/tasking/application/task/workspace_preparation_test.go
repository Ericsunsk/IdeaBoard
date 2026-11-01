package task

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/prompts"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

type currentPromptReader struct {
	template string
	err      error
	calls    int
}

func (r *currentPromptReader) GetRuntimeConfig(_ context.Context, feature string) (prompts.FeaturePromptRuntimeConfig, error) {
	r.calls++
	return prompts.FeaturePromptRuntimeConfig{Prompt: r.template, UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(feature)}, r.err
}

func preparedSnapshot(t *testing.T, preparer *WorkspacePreparer, feature, prompt string, params map[string]interface{}) *tasks.GenerationRequestSnapshot {
	t.Helper()
	raw, err := preparer.Prepare(context.Background(), feature, prompt, params)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := tasks.UnmarshalParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if typed.Generation == nil {
		t.Fatal("missing frozen snapshot")
	}
	return typed.Generation
}

func TestWorkspacePreparerCurrentTemplateFreezesPromptAndSeed(t *testing.T) {
	reader := &currentPromptReader{template: "first {{user_prompt}}"}
	preparer := NewWorkspacePreparer(reader)
	first := preparedSnapshot(t, preparer, workflow.FeatureSlugGeneration, "red bicycle", nil)
	if first.UserPrompt != "red bicycle" || first.SystemPrompt != "first {{user_prompt}}" || first.CompiledPrompt != "first red bicycle" {
		t.Fatalf("snapshot = %+v", first)
	}
	reader.template = "second {{user_prompt}}"
	second := preparedSnapshot(t, preparer, workflow.FeatureSlugGeneration, "red bicycle", nil)
	if reader.calls != 2 || first.TemplateFingerprint == second.TemplateFingerprint || first.Seed == second.Seed {
		t.Fatalf("template changes must change frozen fingerprint and seed: first=%+v second=%+v calls=%d", first, second, reader.calls)
	}
	if first.CompiledPrompt != "first red bicycle" {
		t.Fatal("current edit changed accepted snapshot")
	}
}

func TestWorkspacePreparerMissingTemplateAndReadError(t *testing.T) {
	for _, reader := range []FeaturePromptRuntimeReader{nil, &currentPromptReader{}} {
		snapshot := preparedSnapshot(t, NewWorkspacePreparer(reader), workflow.FeatureSlugGeneration, "passthrough", nil)
		if snapshot.CompiledPrompt != "passthrough" || snapshot.SystemPrompt != "" {
			t.Fatalf("snapshot=%+v", snapshot)
		}
	}
	failure := errors.New("template load failed")
	raw, err := NewWorkspacePreparer(&currentPromptReader{err: failure}).Prepare(context.Background(), workflow.FeatureSlugGeneration, "prompt", nil)
	if raw != "" || !errors.Is(err, failure) {
		t.Fatalf("raw=%q err=%v; want fail before compilation", raw, err)
	}
}

func TestWorkspacePreparerIgnoredPromptKeepsMedia(t *testing.T) {
	snapshot := preparedSnapshot(t, NewWorkspacePreparer(&currentPromptReader{template: "do not inject {{user_prompt}}"}), workflow.FeatureSlugUpscale, "drop user text", map[string]interface{}{
		CompilerWorkspaceMediaAssetIDsParam: []string{"asset_input1"}, "upscale": map[string]interface{}{"scale": 2.0},
	})
	if snapshot.UserPrompt != "" || snapshot.SystemPrompt != "" || snapshot.CompiledPrompt != "" {
		t.Fatalf("ignored prompt injected: %+v", snapshot)
	}
	if len(snapshot.Media) != 1 || snapshot.Media[0].PublicAssetID != "asset_input1" {
		t.Fatalf("media changed: %+v", snapshot.Media)
	}
}

func TestWorkspacePreparerUsesPersistentCurrentTemplate(t *testing.T) {
	ctx := context.Background()
	repository := prompts.NewFeaturePromptRepository(testdb.Open(t))
	preparer := NewWorkspacePreparer(repository)
	missing := preparedSnapshot(t, preparer, workflow.FeatureSlugGeneration, "bicycle", nil)
	if missing.CompiledPrompt != "bicycle" {
		t.Fatalf("missing=%+v", missing)
	}
	if _, err := repository.SaveConfig(ctx, workflow.FeatureSlugGeneration, "stored {{user_prompt}}", nil); err != nil {
		t.Fatal(err)
	}
	current := preparedSnapshot(t, preparer, workflow.FeatureSlugGeneration, "bicycle", nil)
	if current.CompiledPrompt != "stored bicycle" || current.TemplateFingerprint == missing.TemplateFingerprint {
		t.Fatalf("current=%+v missing=%+v", current, missing)
	}
}
