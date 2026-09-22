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
