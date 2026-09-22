package ports

import (
	"context"

	"ideaboard/api/internal/searchcatalog/domain"
)

// ProviderTestAdapter probes one web search provider with a raw API key and
// classifies the outcome for the admin connection-test flow.
type ProviderTestAdapter interface {
	Provider() domain.Provider
	Test(context.Context, string) (domain.FailureCategory, string, error)
}
