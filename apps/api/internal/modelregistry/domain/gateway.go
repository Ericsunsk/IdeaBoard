package domain

import (
	"errors"
	"time"
)

// GatewaySummary is the masked admin read model. It never carries secrets.
type GatewaySummary struct {
	BaseURL         string     `json:"base_url,omitempty"`
	AccountID       string     `json:"account_id,omitempty"`
	GatewayID       string     `json:"gateway_id,omitempty"`
	TokenConfigured bool       `json:"token_configured"`
	IsFirstParty    bool       `json:"is_first_party"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// ResolvedGateway is the request-time credential handed to the gateway client.
type ResolvedGateway struct {
	BaseURL   string
	AccountID string
	GatewayID string
	Token     string
}

// UpdateInput carries an admin token rotation. A nil Token keeps the stored
// secret; there is no clear-token operation (rotation means replacement).
type UpdateInput struct {
	BaseURL   string
	Token     *string
	AccountID string
	GatewayID string
}

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

// ErrAIGatewayNotConfigured reports missing gateway configuration. Callers
// fail closed on it: no fallback to direct provider egress exists.
var ErrAIGatewayNotConfigured = errors.New("AI gateway is not configured")

// ErrVisionModelMissing reports a vision search model without configuration.
// Workers fail closed on it: no silent default model exists.
var ErrVisionModelMissing = errors.New("AI gateway vision model mapping is not configured")
