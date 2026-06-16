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
)

type FeaturePromptRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type FeaturePromptVariable struct {
	Key  string `json:"key"`
	Tone string `json:"tone,omitempty"`
}

type FeaturePromptConfig struct {
	Slug          string                  `json:"slug"`
	Prompt        string                  `json:"prompt"`
	Enabled       bool                    `json:"enabled"`
	Variables     []FeaturePromptVariable `json:"variables"`
	SchemaVersion int32                   `json:"schema_version"`
	PublishedAt   *time.Time              `json:"published_at,omitempty"`
	UpdatedAt     *time.Time              `json:"updated_at,omitempty"`
}

type FeaturePromptRevision struct {
	ID            int64                   `json:"id"`
	Slug          string                  `json:"slug"`
	Prompt        string                  `json:"prompt"`
	Enabled       bool                    `json:"enabled"`
	Variables     []FeaturePromptVariable `json:"variables"`
	SchemaVersion int32                   `json:"schema_version"`
	PublishedAt   *time.Time              `json:"published_at,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
}

type FeaturePromptSaveInput struct {
	Prompt    string
	Enabled   bool
	Variables []FeaturePromptVariable
}

func NewFeaturePromptRepository(db *sql.DB) *FeaturePromptRepository {
	return &FeaturePromptRepository{db: db, q: dbsqlc.New(db)}
}

func (r *FeaturePromptRepository) GetPrompt(ctx context.Context, slug string) (string, error) {
	row, err := r.q.GetFeaturePrompt(ctx, slug)
	if err != nil {
		return "", err
	}
	if !row.Enabled {
		return "", database.ErrNotFound
	}
	return row.Prompt, nil
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
	bySlug := make(map[string]dbsqlc.FeaturePrompt, len(rows))
	for _, row := range rows {
		bySlug[row.FeatureSlug] = row
	}
	slugs := workflow.SupportedFeatureSlugs()
	configs := make([]FeaturePromptConfig, 0, len(slugs))
	for _, slug := range slugs {
		if row, ok := bySlug[slug]; ok {
			cfg, err := promptConfigFromRow(row)
			if err != nil {
				return nil, err
			}
			configs = append(configs, cfg)
			continue
		}
		configs = append(configs, defaultPromptConfig(slug))
	}
	return configs, nil
}

func (r *FeaturePromptRepository) SetPromptConfig(ctx context.Context, slug string, input FeaturePromptSaveInput, updatedBy *int64) (*FeaturePromptConfig, error) {
	var updatedByValue sql.NullInt64
	if updatedBy != nil {
		updatedByValue = sql.NullInt64{Int64: *updatedBy, Valid: true}
	}

	variables := normalizePromptVariables(input.Variables)
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, err
	}
	publishedAt := sql.NullTime{}
	if input.Enabled {
		publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		version, err := q.UpsertFeaturePrompt(ctx, dbsqlc.UpsertFeaturePromptParams{
			FeatureSlug: slug,
			Prompt:      input.Prompt,
			Enabled:     input.Enabled,
			Variables:   variablesJSON,
			UpdatedBy:   updatedByValue,
			PublishedAt: publishedAt,
		})
		if err != nil {
			return err
		}
		_, err = q.CreateFeaturePromptRevision(ctx, dbsqlc.CreateFeaturePromptRevisionParams{
			FeatureSlug:   slug,
			Prompt:        input.Prompt,
			Enabled:       input.Enabled,
			Variables:     variablesJSON,
			SchemaVersion: version,
			UpdatedBy:     updatedByValue,
			PublishedAt:   publishedAt,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.GetPromptConfig(ctx, slug)
}

func (r *FeaturePromptRepository) ListPromptRevisions(ctx context.Context, slug string) ([]FeaturePromptRevision, error) {
	rows, err := r.q.ListFeaturePromptRevisions(ctx, slug)
	if err != nil {
		return nil, err
	}
	revisions := make([]FeaturePromptRevision, 0, len(rows))
	for _, row := range rows {
		revision, err := promptRevisionFromRow(row)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

func defaultPromptConfig(slug string) FeaturePromptConfig {
	return FeaturePromptConfig{
		Slug:          slug,
		Prompt:        "",
		Enabled:       true,
		Variables:     defaultPromptVariables(slug),
		SchemaVersion: 1,
	}
}

func promptConfigFromRow(row dbsqlc.FeaturePrompt) (FeaturePromptConfig, error) {
	variables, err := variablesFromJSON(row.Variables)
	if err != nil {
		return FeaturePromptConfig{}, fmt.Errorf("decode feature prompt %q variables: %w", row.FeatureSlug, err)
	}
	updatedAt := row.UpdatedAt
	return FeaturePromptConfig{
		Slug:          row.FeatureSlug,
		Prompt:        row.Prompt,
		Enabled:       row.Enabled,
		Variables:     variables,
		SchemaVersion: row.SchemaVersion,
		PublishedAt:   dbutil.TimePtr(row.PublishedAt),
		UpdatedAt:     &updatedAt,
	}, nil
}

func promptRevisionFromRow(row dbsqlc.FeaturePromptRevision) (FeaturePromptRevision, error) {
	variables, err := variablesFromJSON(row.Variables)
	if err != nil {
		return FeaturePromptRevision{}, fmt.Errorf("decode feature prompt revision %q v%d variables: %w", row.FeatureSlug, row.SchemaVersion, err)
	}
	return FeaturePromptRevision{
		ID:            row.ID,
		Slug:          row.FeatureSlug,
		Prompt:        row.Prompt,
		Enabled:       row.Enabled,
		Variables:     variables,
		SchemaVersion: row.SchemaVersion,
		PublishedAt:   dbutil.TimePtr(row.PublishedAt),
		CreatedAt:     row.CreatedAt,
	}, nil
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
		return []FeaturePromptVariable{{Key: "{{strength}}", Tone: "control"}}
	case workflow.FeatureSlugStyleFusion:
		return []FeaturePromptVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: "{{image_weights}}", Tone: "control"},
		}
	case workflow.FeatureSlugHDUpscale:
		return []FeaturePromptVariable{
			{Key: "{{upscale_mode}}", Tone: "control"},
			{Key: "{{upscale_factor}}", Tone: "control"},
			{Key: "{{output_format}}", Tone: "control"},
			{Key: "{{output_quality}}", Tone: "control"},
		}
	case workflow.FeatureSlugImageExtend:
		return []FeaturePromptVariable{
			{Key: "{{user_prompt}}", Tone: "prompt"},
			{Key: "{{extend_top}}", Tone: "control"},
			{Key: "{{extend_right}}", Tone: "control"},
			{Key: "{{extend_bottom}}", Tone: "control"},
			{Key: "{{extend_left}}", Tone: "control"},
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
