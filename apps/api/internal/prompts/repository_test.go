package prompts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

func TestFeaturePromptAdminConfigAndRuntimeShareSavedRevisionContent(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)
	slug := workflow.FeatureSlugStyleFusion

	firstConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt: "first prompt {{user_prompt}}",
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
		t.Fatalf("SetPromptConfig first revision returned error: %v", err)
	}
	if firstConfig.SchemaVersion != 1 {
		t.Fatalf("first schema version = %d, want 1", firstConfig.SchemaVersion)
	}
	if firstConfig.CurrentRevisionID <= 0 {
		t.Fatalf("first current revision id = %d, want positive id", firstConfig.CurrentRevisionID)
	}
	assertFeaturePromptCurrentPointer(t, ctx, db, slug, firstConfig.CurrentRevisionID)
	assertFeaturePromptCurrentRowHasNoCopiedConfigFacts(t, ctx, db)

	runtimePrompt, err := repo.GetPrompt(ctx, slug)
	if err != nil || runtimePrompt != firstConfig.Prompt {
		t.Fatalf("runtime GetPrompt = %q, %v, want saved revision", runtimePrompt, err)
	}

	adminConfig, err := repo.GetPromptConfig(ctx, slug)
	if err != nil {
		t.Fatalf("GetPromptConfig returned error: %v", err)
	}
	if adminConfig.Prompt != firstConfig.Prompt {
		t.Fatalf("admin prompt = %q, want %q", adminConfig.Prompt, firstConfig.Prompt)
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

	secondConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt: "second prompt {{fusion_input_weights}}",
		Variables: []FeaturePromptVariable{
			{Key: " {{fusion_input_weights}} ", Tone: "control"},
			{Key: "{{fusion_input_weights}}", Tone: "prompt"},
			{Key: "{{custom_context}}", Tone: "unknown-tone"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig second revision returned error: %v", err)
	}
	if secondConfig.SchemaVersion != 2 {
		t.Fatalf("second schema version = %d, want 2", secondConfig.SchemaVersion)
	}
	if !hasPromptVariable(secondConfig.Variables, "{{fusion_input_weights}}", "control") {
		t.Fatalf("second variables = %#v, want normalized fusion_input_weights control variable", secondConfig.Variables)
	}
	if !hasPromptVariable(secondConfig.Variables, "{{custom_context}}", "context") {
		t.Fatalf("second variables = %#v, want unknown tone normalized to context", secondConfig.Variables)
	}

	runtimePrompt, err = repo.GetPrompt(ctx, slug)
	if err != nil {
		t.Fatalf("runtime GetPrompt returned error: %v", err)
	}
	if runtimePrompt != secondConfig.Prompt {
		t.Fatalf("runtime prompt = %q, want %q", runtimePrompt, secondConfig.Prompt)
	}
	runtimeConfig, err := repo.GetPromptRuntimeConfig(ctx, slug)
	if err != nil {
		t.Fatalf("runtime GetPromptRuntimeConfig returned error: %v", err)
	}
	if !runtimeConfig.UserPromptMergePolicy.AppendBasePromptWithoutTemplate {
		t.Fatalf("runtime merge policy = %#v, want default style-fusion append policy", runtimeConfig.UserPromptMergePolicy)
	}

	clearedConfig, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt:    "prompt without visible variables",
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
	if revisions[0].SchemaVersion != 3 || len(revisions[0].Variables) != 0 {
		t.Fatalf("latest revision = %#v, want schema version 3 with empty variables", revisions[0])
	}
	if revisions[1].SchemaVersion != 2 {
		t.Fatalf("second revision = %#v, want schema version 2", revisions[1])
	}
	if revisions[2].SchemaVersion != 1 {
		t.Fatalf("old revision = %#v, want schema version 1", revisions[2])
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

func TestFeaturePromptPatternGenSaveDoesNotRequireAvailabilityGate(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)
	slug := workflow.FeatureSlugPatternGen

	config, err := repo.SetPromptConfig(ctx, slug, FeaturePromptSaveInput{
		Prompt:    "Create a seamless commercial pattern for {{missing_token}}",
		Variables: promptVariables("{{user_prompt}}"),
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig returned error: %v", err)
	}
	if config.CurrentRevisionID <= 0 || config.SchemaVersion != 1 {
		t.Fatalf("saved config = %#v, want current saved revision", config)
	}
	if revisionCount := countFeaturePromptRevisionsForTest(t, ctx, db, slug); revisionCount != 1 {
		t.Fatalf("revision count = %d, want saved prompt revision", revisionCount)
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
	Variables     []FeaturePromptVariable
	ModelDefaults FeaturePromptModelDefaults
	SchemaVersion int32
}

func listFeaturePromptRevisionsForTest(t *testing.T, ctx context.Context, db queryDB, slug string) []featurePromptRevisionForTest {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT id, variables, model_defaults, schema_version
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
			&variablesJSON,
			&modelDefaultsJSON,
			&revision.SchemaVersion,
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

func countFeaturePromptRevisionsForTest(t *testing.T, ctx context.Context, db queryDB, slug string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feature_prompt_revisions WHERE feature_slug = $1`, slug).Scan(&count); err != nil {
		t.Fatalf("count feature prompt revisions failed: %v", err)
	}
	return count
}
