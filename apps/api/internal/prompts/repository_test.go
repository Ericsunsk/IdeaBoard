package prompts

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

func TestFeaturePromptAdminConfigAndRuntimePromptHaveSeparateContracts(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)
	slug := workflow.FeatureSlugStyleFusion

	disabledConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt:  "disabled prompt {{user_prompt}}",
		Enabled: false,
		Variables: []FeaturePromptVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: "{{fusion_input_weights}}", Tone: "control"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig disabled returned error: %v", err)
	}
	if disabledConfig.Enabled {
		t.Fatalf("disabled config Enabled = true, want false")
	}
	if disabledConfig.PublishedAt != nil {
		t.Fatalf("disabled config PublishedAt = %v, want nil", disabledConfig.PublishedAt)
	}
	if disabledConfig.SchemaVersion != 1 {
		t.Fatalf("disabled schema version = %d, want 1", disabledConfig.SchemaVersion)
	}

	if _, err := repo.GetPrompt(ctx, slug); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("runtime GetPrompt disabled error = %v, want ErrNotFound", err)
	}

	adminConfig, err := repo.GetPromptConfig(ctx, slug)
	if err != nil {
		t.Fatalf("GetPromptConfig returned error: %v", err)
	}
	if adminConfig.Prompt != disabledConfig.Prompt {
		t.Fatalf("admin prompt = %q, want %q", adminConfig.Prompt, disabledConfig.Prompt)
	}
	if !hasPromptVariable(adminConfig.Variables, "{{fusion_input_weights}}", "control") {
		t.Fatalf("admin variables = %#v, want fusion_input_weights control variable", adminConfig.Variables)
	}

	enabledConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt:  "enabled prompt {{fusion_input_weights}}",
		Enabled: true,
		Variables: []FeaturePromptVariable{
			{Key: " {{fusion_input_weights}} ", Tone: "control"},
			{Key: "{{fusion_input_weights}}", Tone: "prompt"},
			{Key: "{{custom_context}}", Tone: "unknown-tone"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig enabled returned error: %v", err)
	}
	if !enabledConfig.Enabled {
		t.Fatalf("enabled config Enabled = false, want true")
	}
	if enabledConfig.PublishedAt == nil {
		t.Fatalf("enabled config PublishedAt = nil, want value")
	}
	if enabledConfig.SchemaVersion != 2 {
		t.Fatalf("enabled schema version = %d, want 2", enabledConfig.SchemaVersion)
	}
	if !hasPromptVariable(enabledConfig.Variables, "{{fusion_input_weights}}", "control") {
		t.Fatalf("enabled variables = %#v, want normalized fusion_input_weights control variable", enabledConfig.Variables)
	}
	if !hasPromptVariable(enabledConfig.Variables, "{{custom_context}}", "context") {
		t.Fatalf("enabled variables = %#v, want unknown tone normalized to context", enabledConfig.Variables)
	}

	runtimePrompt, err := repo.GetPrompt(ctx, slug)
	if err != nil {
		t.Fatalf("runtime GetPrompt enabled returned error: %v", err)
	}
	if runtimePrompt != enabledConfig.Prompt {
		t.Fatalf("runtime prompt = %q, want %q", runtimePrompt, enabledConfig.Prompt)
	}

	clearedConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt:    "enabled prompt without visible variables",
		Enabled:   true,
		Variables: []FeaturePromptVariable{},
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig cleared variables returned error: %v", err)
	}
	if clearedConfig.SchemaVersion != 3 {
		t.Fatalf("cleared schema version = %d, want 3", clearedConfig.SchemaVersion)
	}
	if len(clearedConfig.Variables) != 0 {
		t.Fatalf("cleared variables = %#v, want empty slice", clearedConfig.Variables)
	}

	revisions, err := repo.ListPromptRevisions(ctx, slug)
	if err != nil {
		t.Fatalf("ListPromptRevisions returned error: %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("revision count = %d, want 3", len(revisions))
	}
	if revisions[0].SchemaVersion != 3 || !revisions[0].Enabled || revisions[0].PublishedAt == nil || len(revisions[0].Variables) != 0 {
		t.Fatalf("latest revision = %#v, want enabled schema version 3 with empty variables and PublishedAt", revisions[0])
	}
	if revisions[1].SchemaVersion != 2 || !revisions[1].Enabled || revisions[1].PublishedAt == nil {
		t.Fatalf("second revision = %#v, want enabled schema version 2 with PublishedAt", revisions[1])
	}
	if revisions[2].SchemaVersion != 1 || revisions[2].Enabled || revisions[2].PublishedAt != nil {
		t.Fatalf("old revision = %#v, want disabled schema version 1 without PublishedAt", revisions[2])
	}
}

func TestFeaturePromptDefaultConfigUsesStableSchemaVersion(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)

	config, err := repo.GetPromptConfig(ctx, workflow.FeatureSlugPartialEdit)
	if err != nil {
		t.Fatalf("GetPromptConfig default returned error: %v", err)
	}
	if config.SchemaVersion != 1 {
		t.Fatalf("default schema version = %d, want 1", config.SchemaVersion)
	}
	if !hasPromptVariable(config.Variables, "{{user_prompt}}", "prompt") {
		t.Fatalf("default variables = %#v, want user_prompt variable", config.Variables)
	}
}

func TestFeaturePromptDefaultConfigHDUpscaleUsesSingleControlVariable(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)

	config, err := repo.GetPromptConfig(ctx, workflow.FeatureSlugHDUpscale)
	if err != nil {
		t.Fatalf("GetPromptConfig default returned error: %v", err)
	}
	if len(config.Variables) != 1 {
		t.Fatalf("default hd variables = %#v, want one variable", config.Variables)
	}
	if !hasPromptVariable(config.Variables, "{{clarity_strength}}", "control") {
		t.Fatalf("default hd variables = %#v, want clarity_strength control variable", config.Variables)
	}
}

func hasPromptVariable(variables []FeaturePromptVariable, key string, tone string) bool {
	for _, variable := range variables {
		if variable.Key == key && variable.Tone == tone {
			return true
		}
	}
	return false
}
