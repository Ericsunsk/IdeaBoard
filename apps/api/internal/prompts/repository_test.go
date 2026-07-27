package prompts

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

func TestWorkflowFeatureSaveUpdatesSingleCurrentRow(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repository := NewWorkflowFeatureRepository(db)
	slug := workflow.FeatureSlugStyleVariation

	first, err := repository.SaveConfig(ctx, slug, WorkflowFeatureSaveInput{
		CanonicalModel: " model-a ", SystemPrompt: "first", Enabled: true,
		Variables: []WorkflowFeatureVariable{{Key: "{{variation_strength}}", Tone: "control"}},
	}, nil)
	if err != nil {
		t.Fatalf("SaveConfig first: %v", err)
	}
	if first.CanonicalModel != "model-a" || !first.Enabled {
		t.Fatalf("first config = %#v", first)
	}
	second, err := repository.SaveConfig(ctx, slug, WorkflowFeatureSaveInput{
		CanonicalModel: "model-b", SystemPrompt: "second", Enabled: false,
	}, nil)
	if err != nil {
		t.Fatalf("SaveConfig second: %v", err)
	}
	if second.CanonicalModel != "model-b" || second.SystemPrompt != "second" || second.Enabled {
		t.Fatalf("second config = %#v", second)
	}
	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_features WHERE feature_slug = $1`, slug).Scan(&rowCount); err != nil {
		t.Fatalf("count workflow features: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("row count = %d, want 1", rowCount)
	}
}

func TestWorkflowFeatureRuntimeRequiresConfiguredAndEnabled(t *testing.T) {
	ctx := context.Background()
	repository := NewWorkflowFeatureRepository(testdb.Open(t))
	slug := workflow.FeatureSlugPatternGen
	if _, err := repository.GetRuntimeConfig(ctx, slug); !errors.Is(err, ErrWorkflowFeatureNotConfigured) {
		t.Fatalf("unconfigured error = %v", err)
	}
	if _, err := repository.SaveConfig(ctx, slug, WorkflowFeatureSaveInput{CanonicalModel: "image-a", Enabled: false}, nil); err != nil {
		t.Fatalf("save disabled: %v", err)
	}
	if _, err := repository.GetRuntimeConfig(ctx, slug); !errors.Is(err, ErrWorkflowFeatureDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
}

func TestWorkflowFeatureListIncludesUnconfiguredFeatures(t *testing.T) {
	repository := NewWorkflowFeatureRepository(testdb.Open(t))
	configs, err := repository.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if len(configs) != len(workflow.SupportedFeatureSlugs()) {
		t.Fatalf("config count = %d, want %d", len(configs), len(workflow.SupportedFeatureSlugs()))
	}
}
