package domain

import (
	"errors"
)

// CatalogItem describes one model available in the AI gateway catalog.
// The catalog carries routing and display metadata only: it holds no
// prices. Billing charges solely on the billing SKU UnitCredits, never from
// stored catalog data.
type CatalogItem struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	Mode                string   `json:"mode,omitempty"`
	Description         string   `json:"description"`
	ContextOrResolution string   `json:"context_or_resolution"`
	Capabilities        []string `json:"capabilities"`
	Hosting             string   `json:"hosting"`
	TermsURL            string   `json:"terms_url"`
	DocsURL             string   `json:"docs_url"`
	CustomPath          string   `json:"custom_path,omitempty"`
	CustomParams        string   `json:"custom_params,omitempty"`
	// DescriptionEnriched distinguishes replaceable import text (true)
	// from operator-written text (false, never overwritten).
	DescriptionEnriched   bool `json:"description_enriched,omitempty"`
	SupportsPromptCaching bool `json:"supports_prompt_caching,omitempty"`
}

// DefaultCatalog returns no built-in models: the catalog is fully
// operator-managed through the admin model library. An empty slice keeps
// ListCatalog/SaveCatalog semantics (empty configured catalog) unchanged.
func DefaultCatalog() []CatalogItem {
	return []CatalogItem{}
}

// ErrVisionModelMissing reports a vision search model without configuration.
// Workers fail closed on it: no silent default model exists.
var ErrVisionModelMissing = errors.New("AI gateway vision model mapping is not configured")
