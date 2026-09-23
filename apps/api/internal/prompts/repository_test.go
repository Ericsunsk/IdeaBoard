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
	slug := workflow.FeatureSlugVariations

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
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM feature_prompts WHERE feature_slug = $1`, slug).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}

func TestFeaturePromptRuntimeAllowsMissingPrompt(t *testing.T) {
	repository := NewFeaturePromptRepository(testdb.Open(t))
	config, err := repository.GetRuntimeConfig(context.Background(), workflow.FeatureSlugGeneration)
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
	want := make(map[string]bool, len(workflow.SupportedFeatureSlugs())-1)
	for _, slug := range workflow.SupportedFeatureSlugs() {
		if slug != workflow.FeatureSlugCutout {
			want[slug] = false
		}
	}
	if len(configs) != len(want) {
		t.Fatalf("config count = %d, want %d", len(configs), len(want))
	}
	for _, config := range configs {
		if _, ok := want[config.FeatureSlug]; !ok {
			t.Fatalf("unexpected prompt config for feature %q", config.FeatureSlug)
		}
		want[config.FeatureSlug] = true
	}
	for slug, found := range want {
		if !found {
			t.Fatalf("missing prompt config for feature %q", slug)
		}
	}
}
