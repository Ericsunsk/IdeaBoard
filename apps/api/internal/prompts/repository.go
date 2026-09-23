package prompts

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFeaturePromptUnsupported = errors.New("该功能不支持 System Prompt")

type FeaturePromptRepository struct {
	q *dbsqlc.Queries
}

type FeaturePromptConfig struct {
	FeatureSlug string     `json:"feature_slug"`
	Prompt      string     `json:"prompt"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type FeaturePromptRuntimeConfig struct {
	Prompt                string
	UserPromptMergePolicy FeaturePromptMergePolicy
}

func NewFeaturePromptRepository(db *pgxpool.Pool) *FeaturePromptRepository {
	return &FeaturePromptRepository{q: dbsqlc.New(db)}
}

// GetRuntimeConfig returns the code-owned merge policy even when no prompt has
// been configured. A missing prompt is valid runtime state.
func (r *FeaturePromptRepository) GetRuntimeConfig(ctx context.Context, slug string) (FeaturePromptRuntimeConfig, error) {
	config := FeaturePromptRuntimeConfig{UserPromptMergePolicy: DefaultFeaturePromptMergePolicy(slug)}
	if !isPromptConfigurableFeature(slug) {
		return config, nil
	}
	row, err := r.q.GetFeaturePrompt(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			return config, nil
		}
		return FeaturePromptRuntimeConfig{}, err
	}
	config.Prompt = row.Prompt
	return config, nil
}

func (r *FeaturePromptRepository) GetConfig(ctx context.Context, slug string) (*FeaturePromptConfig, error) {
	if !isPromptConfigurableFeature(slug) {
		return nil, ErrFeaturePromptUnsupported
	}
	row, err := r.q.GetFeaturePrompt(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			config := defaultFeaturePromptConfig(slug)
			return &config, nil
		}
		return nil, err
	}
	config := featurePromptConfigFromRow(row)
	return &config, nil
}

func (r *FeaturePromptRepository) ListConfigs(ctx context.Context) ([]FeaturePromptConfig, error) {
	rows, err := r.q.ListFeaturePrompts(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]dbsqlc.FeaturePrompt, len(rows))
	for _, row := range rows {
		bySlug[row.FeatureSlug] = row
	}
	configs := make([]FeaturePromptConfig, 0, len(workflow.SupportedFeatureSlugs())-1)
	for _, slug := range workflow.SupportedFeatureSlugs() {
		if !isPromptConfigurableFeature(slug) {
			continue
		}
		if row, ok := bySlug[slug]; ok {
			configs = append(configs, featurePromptConfigFromRow(row))
			continue
		}
		configs = append(configs, defaultFeaturePromptConfig(slug))
	}
	return configs, nil
}

func (r *FeaturePromptRepository) SaveConfig(ctx context.Context, slug, prompt string, updatedBy *int64) (*FeaturePromptConfig, error) {
	if !isPromptConfigurableFeature(slug) {
		return nil, ErrFeaturePromptUnsupported
	}
	updatedByValue := dbutil.NullInt64(updatedBy)
	row, err := r.q.UpsertFeaturePrompt(ctx, dbsqlc.UpsertFeaturePromptParams{
		FeatureSlug: slug,
		Prompt:      prompt,
		UpdatedBy:   updatedByValue,
	})
	if err != nil {
		return nil, err
	}
	config := featurePromptConfigFromRow(row)
	return &config, nil
}

func isPromptConfigurableFeature(slug string) bool {
	return slug != workflow.FeatureSlugCutout
}

func defaultFeaturePromptConfig(slug string) FeaturePromptConfig {
	return FeaturePromptConfig{
		FeatureSlug: slug,
	}
}

func featurePromptConfigFromRow(row dbsqlc.FeaturePrompt) FeaturePromptConfig {
	return FeaturePromptConfig{
		FeatureSlug: row.FeatureSlug,
		Prompt:      row.Prompt,
		UpdatedAt:   dbutil.TimePtr(row.UpdatedAt),
	}
}
