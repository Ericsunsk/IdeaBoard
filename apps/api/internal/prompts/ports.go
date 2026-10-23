package prompts

import (
	"context"
)

// FeaturePromptReader serves prompt reads: single config, full catalog,
// and the runtime merge policy (missing prompts are valid runtime state).
type FeaturePromptReader interface {
	GetConfig(ctx context.Context, slug string) (*FeaturePromptConfig, error)
	ListConfigs(ctx context.Context) ([]FeaturePromptConfig, error)
	GetRuntimeConfig(ctx context.Context, slug string) (FeaturePromptRuntimeConfig, error)
}

// FeaturePromptWriter persists operator-edited prompts.
type FeaturePromptWriter interface {
	SaveConfig(ctx context.Context, slug, prompt string, updatedBy *int64) (*FeaturePromptConfig, error)
}

// FeaturePromptStore is the combined persistence seam for the handler:
// reads through FeaturePromptReader, writes through FeaturePromptWriter.
// The postgres FeaturePromptRepository is the production implementation;
// tests inject fakes through this interface.
type FeaturePromptStore interface {
	FeaturePromptReader
	FeaturePromptWriter
}

// Compile-time convergence: the repository implements the seam.
var _ FeaturePromptStore = (*FeaturePromptRepository)(nil)
