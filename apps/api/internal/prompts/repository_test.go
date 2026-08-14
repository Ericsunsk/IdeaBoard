package prompts

import (
	"context"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

func TestFeaturePromptSaveUpdatesSingleCurrentRow(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repository := NewFeaturePromptRepository(db)
	slug := workflow.FeatureSlugStyleVariation

	if _, err := repository.SaveConfig(ctx, slug, "first", nil); err != nil {
		t.Fatalf("SaveConfig first: %v", err)
	}
	second, err := repository.SaveConfig(ctx, slug, "", nil)
	if err != nil {
		t.Fatalf("SaveConfig empty: %v", err)
	}
	if second.Prompt != "" {
		t.Fatalf("prompt = %q, want empty", second.Prompt)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feature_prompts WHERE feature_slug = $1`, slug).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}

func TestFeaturePromptRuntimeAllowsMissingPrompt(t *testing.T) {
	repository := NewFeaturePromptRepository(testdb.Open(t))
	config, err := repository.GetRuntimeConfig(context.Background(), workflow.FeatureSlugPromptGeneration)
	if err != nil {
		t.Fatalf("GetRuntimeConfig: %v", err)
	}
	if config.Prompt != "" {
		t.Fatalf("prompt = %q, want empty", config.Prompt)
	}
}

func TestFeaturePromptListIncludesUnconfiguredFeatures(t *testing.T) {
	repository := NewFeaturePromptRepository(testdb.Open(t))
	configs, err := repository.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if len(configs) != len(workflow.SupportedFeatureSlugs()) {
		t.Fatalf("config count = %d, want %d", len(configs), len(workflow.SupportedFeatureSlugs()))
	}
}
