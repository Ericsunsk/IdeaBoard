package prompts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

var (
	ErrWorkflowFeatureNotConfigured = errors.New("workflow feature is not configured")
	ErrWorkflowFeatureDisabled      = errors.New("workflow feature is disabled")
)

type WorkflowFeatureRepository struct {
	q *dbsqlc.Queries
}

type WorkflowFeatureVariable struct {
	Key  string `json:"key"`
	Tone string `json:"tone,omitempty"`
}

type WorkflowFeatureConfig struct {
	FeatureSlug           string                           `json:"feature_slug"`
	CanonicalModel        string                           `json:"canonical_model"`
	SystemPrompt          string                           `json:"system_prompt"`
	Variables             []WorkflowFeatureVariable        `json:"variables"`
	UserPromptMergePolicy WorkflowFeaturePromptMergePolicy `json:"user_prompt_merge_policy"`
	Enabled               bool                             `json:"enabled"`
	UpdatedAt             *time.Time                       `json:"updated_at,omitempty"`
}

type WorkflowFeatureSaveInput struct {
	CanonicalModel        string
	SystemPrompt          string
	Variables             []WorkflowFeatureVariable
	UserPromptMergePolicy *WorkflowFeaturePromptMergePolicy
	Enabled               bool
}

type WorkflowFeatureRuntimeConfig struct {
	FeatureSlug           string
	CanonicalModel        string
	SystemPrompt          string
	UserPromptMergePolicy WorkflowFeaturePromptMergePolicy
}

func NewWorkflowFeatureRepository(db *sql.DB) *WorkflowFeatureRepository {
	return &WorkflowFeatureRepository{q: dbsqlc.New(db)}
}

func (r *WorkflowFeatureRepository) GetRuntimeConfig(ctx context.Context, slug string) (WorkflowFeatureRuntimeConfig, error) {
	row, err := r.q.GetWorkflowFeature(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			return WorkflowFeatureRuntimeConfig{}, ErrWorkflowFeatureNotConfigured
		}
		return WorkflowFeatureRuntimeConfig{}, err
	}
	if !row.Enabled {
		return WorkflowFeatureRuntimeConfig{}, ErrWorkflowFeatureDisabled
	}
	mergePolicy, err := mergePolicyFromJSON(row.UserPromptMergePolicy, row.FeatureSlug)
	if err != nil {
		return WorkflowFeatureRuntimeConfig{}, fmt.Errorf("decode workflow feature %q merge policy: %w", row.FeatureSlug, err)
	}
	return WorkflowFeatureRuntimeConfig{
		FeatureSlug:           row.FeatureSlug,
		CanonicalModel:        row.CanonicalModel,
		SystemPrompt:          row.SystemPrompt,
		UserPromptMergePolicy: mergePolicy,
	}, nil
}

func (r *WorkflowFeatureRepository) GetConfig(ctx context.Context, slug string) (*WorkflowFeatureConfig, error) {
	row, err := r.q.GetWorkflowFeature(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			config := defaultWorkflowFeatureConfig(slug)
			return &config, nil
		}
		return nil, err
	}
	config, err := workflowFeatureConfigFromRow(row)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *WorkflowFeatureRepository) ListConfigs(ctx context.Context) ([]WorkflowFeatureConfig, error) {
	rows, err := r.q.ListWorkflowFeatures(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]dbsqlc.WorkflowFeature, len(rows))
	for _, row := range rows {
		bySlug[row.FeatureSlug] = row
	}
	configs := make([]WorkflowFeatureConfig, 0, len(workflow.SupportedFeatureSlugs()))
	for _, slug := range workflow.SupportedFeatureSlugs() {
		row, ok := bySlug[slug]
		if !ok {
			configs = append(configs, defaultWorkflowFeatureConfig(slug))
			continue
		}
		config, err := workflowFeatureConfigFromRow(row)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (r *WorkflowFeatureRepository) SaveConfig(ctx context.Context, slug string, input WorkflowFeatureSaveInput, updatedBy *int64) (*WorkflowFeatureConfig, error) {
	canonicalModel := strings.TrimSpace(input.CanonicalModel)
	if canonicalModel == "" {
		return nil, validation.New("canonical_model 不能为空")
	}
	variablesJSON, err := json.Marshal(normalizeWorkflowFeatureVariables(input.Variables))
	if err != nil {
		return nil, err
	}
	mergePolicy := DefaultWorkflowFeaturePromptMergePolicy(slug)
	if input.UserPromptMergePolicy != nil {
		mergePolicy = *input.UserPromptMergePolicy
	}
	mergePolicyJSON, err := json.Marshal(mergePolicy)
	if err != nil {
		return nil, err
	}
	updatedByValue := sql.NullInt64{}
	if updatedBy != nil {
		updatedByValue = sql.NullInt64{Int64: *updatedBy, Valid: true}
	}
	row, err := r.q.UpsertWorkflowFeature(ctx, dbsqlc.UpsertWorkflowFeatureParams{
		FeatureSlug:           slug,
		CanonicalModel:        canonicalModel,
		SystemPrompt:          input.SystemPrompt,
		Variables:             variablesJSON,
		UserPromptMergePolicy: mergePolicyJSON,
		Enabled:               input.Enabled,
		UpdatedBy:             updatedByValue,
	})
	if err != nil {
		return nil, err
	}
	config, err := workflowFeatureConfigFromRow(row)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func defaultWorkflowFeatureConfig(slug string) WorkflowFeatureConfig {
	return WorkflowFeatureConfig{
		FeatureSlug:           slug,
		Variables:             defaultWorkflowFeatureVariables(slug),
		UserPromptMergePolicy: DefaultWorkflowFeaturePromptMergePolicy(slug),
	}
}

func workflowFeatureConfigFromRow(row dbsqlc.WorkflowFeature) (WorkflowFeatureConfig, error) {
	variables, err := workflowFeatureVariablesFromJSON(row.Variables)
	if err != nil {
		return WorkflowFeatureConfig{}, fmt.Errorf("decode workflow feature %q variables: %w", row.FeatureSlug, err)
	}
	mergePolicy, err := mergePolicyFromJSON(row.UserPromptMergePolicy, row.FeatureSlug)
	if err != nil {
		return WorkflowFeatureConfig{}, fmt.Errorf("decode workflow feature %q merge policy: %w", row.FeatureSlug, err)
	}
	updatedAt := row.UpdatedAt
	return WorkflowFeatureConfig{
		FeatureSlug:           row.FeatureSlug,
		CanonicalModel:        row.CanonicalModel,
		SystemPrompt:          row.SystemPrompt,
		Variables:             variables,
		UserPromptMergePolicy: mergePolicy,
		Enabled:               row.Enabled,
		UpdatedAt:             &updatedAt,
	}, nil
}

func workflowFeatureVariablesFromJSON(raw json.RawMessage) ([]WorkflowFeatureVariable, error) {
	variables := []WorkflowFeatureVariable{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &variables); err != nil {
			return nil, err
		}
	}
	return normalizeWorkflowFeatureVariables(variables), nil
}

func mergePolicyFromJSON(raw json.RawMessage, slug string) (WorkflowFeaturePromptMergePolicy, error) {
	policy := DefaultWorkflowFeaturePromptMergePolicy(slug)
	if len(raw) == 0 {
		return policy, nil
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return WorkflowFeaturePromptMergePolicy{}, err
	}
	return policy, nil
}

func normalizeWorkflowFeatureVariables(variables []WorkflowFeatureVariable) []WorkflowFeatureVariable {
	result := make([]WorkflowFeatureVariable, 0, len(variables))
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
		result = append(result, WorkflowFeatureVariable{Key: key, Tone: normalizePromptVariableTone(variable.Tone)})
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

func defaultWorkflowFeatureVariables(slug string) []WorkflowFeatureVariable {
	switch slug {
	case workflow.FeatureSlugPartialEdit, workflow.FeatureSlugPatternGen:
		return workflowFeatureVariables("{{user_prompt}}")
	case workflow.FeatureSlugStyleVariation:
		return []WorkflowFeatureVariable{{Key: paramkeys.PromptPlaceholderVariationStrength, Tone: "control"}}
	case workflow.FeatureSlugStyleFusion:
		return []WorkflowFeatureVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: paramkeys.PromptPlaceholderFusionInputWeights, Tone: "control"},
		}
	case workflow.FeatureSlugHDUpscale:
		return []WorkflowFeatureVariable{{Key: paramkeys.PromptPlaceholderClarityStrength, Tone: "control"}}
	case workflow.FeatureSlugImageExtend:
		return []WorkflowFeatureVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: paramkeys.PromptPlaceholderExtendTopPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendRightPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendBottomPercent, Tone: "control"},
			{Key: paramkeys.PromptPlaceholderExtendLeftPercent, Tone: "control"},
		}
	default:
		return []WorkflowFeatureVariable{}
	}
}

func workflowFeatureVariables(keys ...string) []WorkflowFeatureVariable {
	variables := make([]WorkflowFeatureVariable, 0, len(keys))
	for _, key := range keys {
		variables = append(variables, WorkflowFeatureVariable{Key: key, Tone: "prompt"})
	}
	return variables
}
