package prompts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

type FeaturePromptRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type FeaturePromptVariable struct {
	Key  string `json:"key"`
	Tone string `json:"tone,omitempty"`
}

type FeaturePromptModelDefaults struct {
	Model string `json:"model,omitempty"`
}

func (d *FeaturePromptModelDefaults) UnmarshalJSON(raw []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if key != paramkeys.Model {
			return fmt.Errorf("unsupported model_defaults field %q", key)
		}
		var model string
		if err := json.Unmarshal(value, &model); err != nil {
			return fmt.Errorf("model must be a string")
		}
		model = strings.TrimSpace(model)
		if model == "" {
			return fmt.Errorf("model must not be empty")
		}
		d.Model = model
	}
	return nil
}

type FeaturePromptConfig struct {
	Slug                  string                     `json:"slug"`
	CurrentRevisionID     int64                      `json:"current_revision_id"`
	Prompt                string                     `json:"prompt"`
	Enabled               bool                       `json:"enabled"`
	Variables             []FeaturePromptVariable    `json:"variables"`
	ModelDefaults         FeaturePromptModelDefaults `json:"model_defaults"`
	UserPromptMergePolicy FeaturePromptMergePolicy   `json:"user_prompt_merge_policy"`
	SchemaVersion         int32                      `json:"schema_version"`
	PublishedAt           *time.Time                 `json:"published_at,omitempty"`
	UpdatedAt             *time.Time                 `json:"updated_at,omitempty"`
}

type FeaturePromptSaveInput struct {
	Prompt                string
	Enabled               bool
	Variables             []FeaturePromptVariable
	ModelDefaults         FeaturePromptModelDefaults
	UserPromptMergePolicy *FeaturePromptMergePolicy
}

type normalizedFeaturePromptSaveInput struct {
	Prompt                string
	Enabled               bool
	Variables             []FeaturePromptVariable
	ModelDefaults         FeaturePromptModelDefaults
	UserPromptMergePolicy FeaturePromptMergePolicy
}

type FeaturePromptRuntimeConfig struct {
	Prompt                string
	ModelDefaults         FeaturePromptModelDefaults
	UserPromptMergePolicy FeaturePromptMergePolicy
}

func NewFeaturePromptRepository(db *sql.DB) *FeaturePromptRepository {
	return &FeaturePromptRepository{db: db, q: dbsqlc.New(db)}
}

func (r *FeaturePromptRepository) GetPrompt(ctx context.Context, slug string) (string, error) {
	config, err := r.GetPromptRuntimeConfig(ctx, slug)
	if err != nil {
		return "", err
	}
	return config.Prompt, nil
}

func (r *FeaturePromptRepository) GetPromptRuntimeConfig(ctx context.Context, slug string) (FeaturePromptRuntimeConfig, error) {
	row, err := r.q.GetFeaturePrompt(ctx, slug)
	if err != nil {
		return FeaturePromptRuntimeConfig{}, err
	}
	if !row.Enabled {
		return FeaturePromptRuntimeConfig{}, database.ErrNotFound
	}
	mergePolicy, err := mergePolicyFromJSON(row.UserPromptMergePolicy, row.FeatureSlug)
	if err != nil {
		return FeaturePromptRuntimeConfig{}, fmt.Errorf("decode feature prompt %q merge policy: %w", row.FeatureSlug, err)
	}
	modelDefaults, err := modelDefaultsFromJSON(row.ModelDefaults)
	if err != nil {
		return FeaturePromptRuntimeConfig{}, fmt.Errorf("decode feature prompt %q model defaults: %w", row.FeatureSlug, err)
	}
	return FeaturePromptRuntimeConfig{
		Prompt:                row.Prompt,
		ModelDefaults:         modelDefaults,
		UserPromptMergePolicy: mergePolicy,
	}, nil
}

func (r *FeaturePromptRepository) GetPromptConfig(ctx context.Context, slug string) (*FeaturePromptConfig, error) {
	row, err := r.q.GetFeaturePrompt(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			cfg := defaultPromptConfig(slug)
			return &cfg, nil
		}
		return nil, err
	}
	cfg, err := promptConfigFromRow(row)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *FeaturePromptRepository) ListPromptConfigs(ctx context.Context) ([]FeaturePromptConfig, error) {
	rows, err := r.q.ListFeaturePrompts(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]dbsqlc.ListFeaturePromptsRow, len(rows))
	for _, row := range rows {
		bySlug[row.FeatureSlug] = row
	}
	slugs := workflow.SupportedFeatureSlugs()
	configs := make([]FeaturePromptConfig, 0, len(slugs))
	for _, slug := range slugs {
		if row, ok := bySlug[slug]; ok {
			cfg, err := promptConfigFromListRow(row)
			if err != nil {
				return nil, err
			}
			configs = append(configs, cfg)
			continue
		}
		cfg := defaultPromptConfig(slug)
		configs = append(configs, cfg)
	}
	return configs, nil
}

func (r *FeaturePromptRepository) SetPromptConfig(ctx context.Context, slug string, input FeaturePromptSaveInput, updatedBy *int64) (*FeaturePromptConfig, error) {
	normalized, err := normalizeFeaturePromptSaveInput(slug, input)
	if err != nil {
		return nil, err
	}

	var updatedByValue sql.NullInt64
	if updatedBy != nil {
		updatedByValue = sql.NullInt64{Int64: *updatedBy, Valid: true}
	}

	variablesJSON, err := json.Marshal(normalized.Variables)
	if err != nil {
		return nil, err
	}
	modelDefaultsJSON, err := json.Marshal(normalized.ModelDefaults)
	if err != nil {
		return nil, err
	}
	mergePolicyJSON, err := json.Marshal(normalized.UserPromptMergePolicy)
	if err != nil {
		return nil, err
	}
	publishedAt := sql.NullTime{}
	if normalized.Enabled {
		publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if err := q.EnsureFeaturePrompt(ctx, slug); err != nil {
			return err
		}
		if _, err := q.GetFeaturePromptForUpdate(ctx, slug); err != nil {
			return err
		}
		revision, err := q.CreateFeaturePromptRevision(ctx, dbsqlc.CreateFeaturePromptRevisionParams{
			FeatureSlug:           slug,
			Prompt:                normalized.Prompt,
			Enabled:               normalized.Enabled,
			Variables:             variablesJSON,
			ModelDefaults:         modelDefaultsJSON,
			UserPromptMergePolicy: mergePolicyJSON,
			UpdatedBy:             updatedByValue,
			PublishedAt:           publishedAt,
		})
		if err != nil {
			return err
		}
		_, err = q.SetFeaturePromptCurrentRevision(ctx, dbsqlc.SetFeaturePromptCurrentRevisionParams{
			FeatureSlug: slug,
			RevisionID:  sql.NullInt64{Int64: revision.ID, Valid: true},
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.GetPromptConfig(ctx, slug)
}

func normalizeFeaturePromptSaveInput(slug string, input FeaturePromptSaveInput) (normalizedFeaturePromptSaveInput, error) {
	mergePolicy := DefaultFeaturePromptMergePolicy(slug)
	if input.UserPromptMergePolicy != nil {
		mergePolicy = *input.UserPromptMergePolicy
	}
	return normalizedFeaturePromptSaveInput{
		Prompt:                input.Prompt,
		Enabled:               input.Enabled,
		Variables:             normalizePromptVariables(input.Variables),
		ModelDefaults:         input.ModelDefaults,
		UserPromptMergePolicy: mergePolicy,
	}, nil
}

func defaultPromptConfig(slug string) FeaturePromptConfig {
	return FeaturePromptConfig{
		Slug:                  slug,
		Prompt:                "",
		Enabled:               true,
		Variables:             defaultPromptVariables(slug),
		ModelDefaults:         FeaturePromptModelDefaults{},
		UserPromptMergePolicy: DefaultFeaturePromptMergePolicy(slug),
		SchemaVersion:         1,
	}
}

func promptConfigFromRow(row dbsqlc.GetFeaturePromptRow) (FeaturePromptConfig, error) {
	variables, err := variablesFromJSON(row.Variables)
	if err != nil {
		return FeaturePromptConfig{}, fmt.Errorf("decode feature prompt %q variables: %w", row.FeatureSlug, err)
	}
	modelDefaults, err := modelDefaultsFromJSON(row.ModelDefaults)
	if err != nil {
		return FeaturePromptConfig{}, fmt.Errorf("decode feature prompt %q model defaults: %w", row.FeatureSlug, err)
	}
	mergePolicy, err := mergePolicyFromJSON(row.UserPromptMergePolicy, row.FeatureSlug)
	if err != nil {
		return FeaturePromptConfig{}, fmt.Errorf("decode feature prompt %q merge policy: %w", row.FeatureSlug, err)
	}
	updatedAt := row.UpdatedAt
	return FeaturePromptConfig{
		Slug:                  row.FeatureSlug,
		CurrentRevisionID:     row.RevisionID,
		Prompt:                row.Prompt,
		Enabled:               row.Enabled,
		Variables:             variables,
		ModelDefaults:         modelDefaults,
		UserPromptMergePolicy: mergePolicy,
		SchemaVersion:         row.SchemaVersion,
		PublishedAt:           dbutil.TimePtr(row.PublishedAt),
		UpdatedAt:             &updatedAt,
	}, nil
}

func promptConfigFromListRow(row dbsqlc.ListFeaturePromptsRow) (FeaturePromptConfig, error) {
	return promptConfigFromRow(dbsqlc.GetFeaturePromptRow(row))
}

func variablesFromJSON(raw json.RawMessage) ([]FeaturePromptVariable, error) {
	var variables []FeaturePromptVariable
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &variables); err != nil {
			return nil, err
		}
	}
	return normalizePromptVariables(variables), nil
}

func modelDefaultsFromJSON(raw json.RawMessage) (FeaturePromptModelDefaults, error) {
	result := FeaturePromptModelDefaults{}
	if len(raw) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return FeaturePromptModelDefaults{}, err
	}
	return result, nil
}

func mergePolicyFromJSON(raw json.RawMessage, slug string) (FeaturePromptMergePolicy, error) {
	policy := DefaultFeaturePromptMergePolicy(slug)
	if len(raw) == 0 {
		return policy, nil
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return FeaturePromptMergePolicy{}, err
	}
	return policy, nil
}

func normalizePromptVariables(variables []FeaturePromptVariable) []FeaturePromptVariable {
	result := make([]FeaturePromptVariable, 0, len(variables))
	seen := map[string]struct{}{}
	for _, variable := range variables {
		key := strings.TrimSpace(variable.Key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, FeaturePromptVariable{
			Key:  key,
			Tone: normalizePromptVariableTone(variable.Tone),
		})
	}
	return result
}

func normalizePromptVariableTone(tone string) string {
	switch strings.TrimSpace(tone) {
	case "prompt", "control", "context":
		return strings.TrimSpace(tone)
	default:
		return "context"
	}
}

func defaultPromptVariables(slug string) []FeaturePromptVariable {
	switch slug {
	case workflow.FeatureSlugPartialEdit, workflow.FeatureSlugPatternGen:
		return promptVariables("{{user_prompt}}")
	case workflow.FeatureSlugStyleVariation:
		return []FeaturePromptVariable{{Key: paramkeys.PromptPlaceholderVariationStrength, Tone: "control"}}
	case workflow.FeatureSlugStyleFusion:
		return []FeaturePromptVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: paramkeys.PromptPlaceholderFusionInputWeights, Tone: "control"},
		}
	case workflow.FeatureSlugHDUpscale:
		return []FeaturePromptVariable{
			{Key: paramkeys.PromptPlaceholderClarityStrength, Tone: "control"},
		}
	case workflow.FeatureSlugImageExtend:
		return []FeaturePromptVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: paramkeys.PromptPlaceholderExtendTopPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendRightPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendBottomPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendLeftPercent, Tone: "control"},
		}
	default:
		return []FeaturePromptVariable{}
	}
}

func promptVariables(keys ...string) []FeaturePromptVariable {
	variables := make([]FeaturePromptVariable, 0, len(keys))
	for _, key := range keys {
		variables = append(variables, FeaturePromptVariable{Key: key, Tone: "prompt"})
	}
	return variables
}
