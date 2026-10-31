package ports

import (
	"context"
	"ideaboard/api/internal/searchcatalog/domain"
)

// SearchRequest is one supplier page fetch: query text, 1-based page and
// per-page limit.
type SearchRequest struct {
	Query string
	Page  int
	Limit int
}

// QueryMaxRunes is the canonical query-text bound, re-exported from domain
// so application edges program to ports without importing domain directly.
const QueryMaxRunes = domain.QueryMaxRunes

// SearchProviderPage is one supplier's normalized page.
type SearchProviderPage struct {
	Results []domain.Result
	HasMore bool
}

// SearchProviderAdapter serves normalized result pages for one web search
// supplier. Implementations live in infrastructure; the application service
// consumes this contract.
type SearchProviderAdapter interface {
	Provider() domain.Provider
	Search(context.Context, string, SearchRequest) (SearchProviderPage, error)
}

// merged from internal/searchcatalog/ports/provider.go (structural consolidation; symbols unchanged).
// ProviderTestAdapter probes one web search provider with a raw API key and
// classifies the outcome for the admin connection-test flow.
type ProviderTestAdapter interface {
	Provider() domain.Provider
	Test(context.Context, string) (domain.FailureCategory, string, error)
}
