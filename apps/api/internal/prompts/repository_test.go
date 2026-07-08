package prompts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
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
		ModelDefaults: FeaturePromptModelDefaults{Model: "image-alpha"},
		UserPromptMergePolicy: &FeaturePromptMergePolicy{
			AppendUserPromptWhenTemplateMissesPlaceholder: false,
			AppendBasePromptWithoutTemplate:               false,
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
	if disabledConfig.CurrentRevisionID <= 0 {
		t.Fatalf("disabled current revision id = %d, want positive id", disabledConfig.CurrentRevisionID)
	}
	assertFeaturePromptCurrentPointer(t, ctx, db, slug, disabledConfig.CurrentRevisionID)
	assertFeaturePromptCurrentRowHasNoCopiedConfigFacts(t, ctx, db)

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
	if adminConfig.ModelDefaults.Model != "image-alpha" {
		t.Fatalf("admin model defaults = %#v, want persisted revision model default", adminConfig.ModelDefaults)
	}
	if adminConfig.UserPromptMergePolicy.AppendBasePromptWithoutTemplate {
		t.Fatalf("admin merge policy = %#v, want revision policy", adminConfig.UserPromptMergePolicy)
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
	runtimeConfig, err := repo.GetPromptRuntimeConfig(ctx, slug)
	if err != nil {
		t.Fatalf("runtime GetPromptRuntimeConfig enabled returned error: %v", err)
	}
	if !runtimeConfig.UserPromptMergePolicy.AppendBasePromptWithoutTemplate {
		t.Fatalf("runtime merge policy = %#v, want default style-fusion append policy", runtimeConfig.UserPromptMergePolicy)
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

	revisions := listFeaturePromptRevisionsForTest(t, ctx, db, slug)
	if len(revisions) != 3 {
		t.Fatalf("revision count = %d, want 3", len(revisions))
	}
	if revisions[0].SchemaVersion != 3 || !revisions[0].Enabled || !revisions[0].PublishedAt.Valid || len(revisions[0].Variables) != 0 {
		t.Fatalf("latest revision = %#v, want enabled schema version 3 with empty variables and PublishedAt", revisions[0])
	}
	if revisions[1].SchemaVersion != 2 || !revisions[1].Enabled || !revisions[1].PublishedAt.Valid {
		t.Fatalf("second revision = %#v, want enabled schema version 2 with PublishedAt", revisions[1])
	}
	if revisions[2].SchemaVersion != 1 || revisions[2].Enabled || revisions[2].PublishedAt.Valid {
		t.Fatalf("old revision = %#v, want disabled schema version 1 without PublishedAt", revisions[2])
	}
	if revisions[2].ModelDefaults.Model != "image-alpha" {
		t.Fatalf("old revision model defaults = %#v, want append-only revision payload", revisions[2].ModelDefaults)
	}
}

func TestFeaturePromptConcurrentSavesCreateSequentialAppendOnlyRevisions(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)
	slug := workflow.FeatureSlugImageExtend

	const saveCount = 6
	var wg sync.WaitGroup
	errs := make(chan error, saveCount)
	for i := 0; i < saveCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
				Prompt:    fmt.Sprintf("concurrent prompt %02d {{user_prompt}}", i),
				Enabled:   true,
				Variables: promptVariables("{{user_prompt}}"),
			}, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SetPromptConfig returned error: %v", err)
		}
	}

	revisions := listFeaturePromptRevisionsForTest(t, ctx, db, slug)
	if len(revisions) != saveCount {
		t.Fatalf("revision count = %d, want %d", len(revisions), saveCount)
	}
	versions := make([]int, 0, len(revisions))
	ids := make(map[int64]struct{}, len(revisions))
	for _, revision := range revisions {
		versions = append(versions, int(revision.SchemaVersion))
		ids[revision.ID] = struct{}{}
	}
	sort.Ints(versions)
	for i, version := range versions {
		if version != i+1 {
			t.Fatalf("schema versions = %v, want contiguous versions 1..%d", versions, saveCount)
		}
	}
	current, err := repo.GetPromptConfig(ctx, slug)
	if err != nil {
		t.Fatalf("GetPromptConfig returned error: %v", err)
	}
	if _, ok := ids[current.CurrentRevisionID]; !ok {
		t.Fatalf("current revision id %d not found in append-only revisions %#v", current.CurrentRevisionID, revisions)
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
	if config.CurrentRevisionID != 0 {
		t.Fatalf("default current revision id = %d, want 0 before first save", config.CurrentRevisionID)
	}
	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feature_prompts`).Scan(&rowCount); err != nil {
		t.Fatalf("count feature_prompts returned error: %v", err)
	}
	if rowCount != 0 {
		t.Fatalf("default prompt initialization inserted %d current rows, want 0", rowCount)
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

func assertFeaturePromptCurrentPointer(t *testing.T, ctx context.Context, db queryDB, slug string, revisionID int64) {
	t.Helper()
	var currentRevisionID int64
	if err := db.QueryRowContext(ctx, `SELECT current_revision_id FROM feature_prompts WHERE feature_slug = $1`, slug).Scan(&currentRevisionID); err != nil {
		t.Fatalf("read current revision pointer failed: %v", err)
	}
	if currentRevisionID != revisionID {
		t.Fatalf("current revision pointer = %d, want %d", currentRevisionID, revisionID)
	}
}

func assertFeaturePromptCurrentRowHasNoCopiedConfigFacts(t *testing.T, ctx context.Context, db queryDB) {
	t.Helper()
	var copiedFactColumns int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_name = 'feature_prompts'
  AND column_name IN ('prompt', 'enabled', 'variables', 'model_defaults', 'user_prompt_merge_policy', 'schema_version', 'published_at', 'updated_by')
`).Scan(&copiedFactColumns); err != nil {
		t.Fatalf("inspect feature_prompts columns failed: %v", err)
	}
	if copiedFactColumns != 0 {
		t.Fatalf("feature_prompts has %d copied config fact columns, want only identity/current pointer metadata", copiedFactColumns)
	}
}

type queryDB interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

type featurePromptRevisionForTest struct {
	ID            int64
	Enabled       bool
	Variables     []FeaturePromptVariable
	ModelDefaults FeaturePromptModelDefaults
	SchemaVersion int32
	PublishedAt   sql.NullTime
}

func listFeaturePromptRevisionsForTest(t *testing.T, ctx context.Context, db queryDB, slug string) []featurePromptRevisionForTest {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT id, enabled, variables, model_defaults, schema_version, published_at
FROM feature_prompt_revisions
WHERE feature_slug = $1
ORDER BY schema_version DESC, id DESC`, slug)
	if err != nil {
		t.Fatalf("list feature prompt revisions failed: %v", err)
	}
	defer rows.Close()

	revisions := []featurePromptRevisionForTest{}
	for rows.Next() {
		var revision featurePromptRevisionForTest
		var variablesJSON json.RawMessage
		var modelDefaultsJSON json.RawMessage
		if err := rows.Scan(
			&revision.ID,
			&revision.Enabled,
			&variablesJSON,
			&modelDefaultsJSON,
			&revision.SchemaVersion,
			&revision.PublishedAt,
		); err != nil {
			t.Fatalf("scan feature prompt revision failed: %v", err)
		}
		variables, err := variablesFromJSON(variablesJSON)
		if err != nil {
			t.Fatalf("decode revision variables failed: %v", err)
		}
		modelDefaults, err := modelDefaultsFromJSON(modelDefaultsJSON)
		if err != nil {
			t.Fatalf("decode revision model defaults failed: %v", err)
		}
		revision.Variables = variables
		revision.ModelDefaults = modelDefaults
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate feature prompt revisions failed: %v", err)
	}
	return revisions
}
