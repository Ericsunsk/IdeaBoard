package searchcatalog

// Workspace-scoped search for the Public API v1 (issue #806).
//
// Additive only: the legacy /api/search + /api/user vision routes and
// the searchcatalog provider seams are untouched (removal is #809).
// This file owns the v1 validation vocabulary (explicit query rules,
// provider filter parsing, vision-query input bounds) plus the opaque
// visq_ vision operation ids. Provider fetching stays behind the
// existing application.SearchService / VisionSearchService seams,
// injected by composition; the v1 HTTP handlers only translate.
//
// Invariants owned here:
//   - Search results are provider URL + source metadata only (never
//     bytes), mirroring the library asset boundary.
//   - Query validation is explicit: blank queries and over-limit queries
//     are 422s at the handler (ErrV1SearchQueryEmpty/TooLong), never
//     silent empty pages.
//   - Vision-query inputs reference owned workspace library assets by
//     lasset_ id (shape-checked here, existence-checked against the
//     workspace library store at the handler); client bytes never cross
//     the v1 vision boundary, so no multipart surface is introduced.
//   - Vision billing reuses the ai-search:vision SKU semantics from the
//     application vision_billing service (quote + hold/capture/release
//     through credits.Commands); this file only bounds the input.

import (
	"errors"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/searchcatalog/domain"
)

const (
	// V1MaxQueryRunes mirrors the application search bound (200 runes).
	V1MaxQueryRunes = 200
	// V1MaxVisionAssetRefs bounds one vision query to 4 owned library
	// references (parity with VisionMaxImages, without client bytes).
	V1MaxVisionAssetRefs = domain.VisionMaxImages
	// V1VisionQueryPrefix mints visq_ vision operation ids. Local const
	// like boards' sht_ pattern since the kernel lacks it.
	V1VisionQueryPrefix = "visq_"
)

var (
	// ErrV1SearchQueryEmpty reports a blank search query.
	ErrV1SearchQueryEmpty = errors.New("search v1 query is required")
	// ErrV1SearchQueryTooLong reports an over-limit search query.
	ErrV1SearchQueryTooLong = errors.New("search v1 query exceeds 200 characters")
	// ErrV1SearchProviderInvalid reports an unknown provider filter.
	ErrV1SearchProviderInvalid = errors.New("search v1 provider must be unsplash or pexels")
	// ErrV1VisionInput reports an invalid vision-query input.
	ErrV1VisionInput = errors.New("search v1 vision input is invalid")
)

// NewV1VisionQueryPublicID mints an opaque visq_ operation identifier.
func NewV1VisionQueryPublicID() (string, error) {
	return contract.MintPublicID(V1VisionQueryPrefix, 12)
}

// IsV1VisionQueryID reports visq_ shape.
func IsV1VisionQueryID(id string) bool {
	return contract.IsValidPublicID(V1VisionQueryPrefix, id)
}

// V1SearchResult is one normalized v1 search hit: provider URL +
// source metadata only, never bytes.
type V1SearchResult struct {
	Provider    string
	SourceID    string
	Title       string
	Author      string
	SourceURL   string
	PreviewURL  string
	OriginalURL string
	Width       int
	Height      int
}

// NormalizeV1SearchQuery trims and collapses whitespace; blank and
// over-limit queries fail explicitly (no silent empty pages).
func NormalizeV1SearchQuery(raw string) (string, error) {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if query == "" {
		return "", ErrV1SearchQueryEmpty
	}
	if utf8.RuneCountInString(query) > V1MaxQueryRunes {
		return "", ErrV1SearchQueryTooLong
	}
	return query, nil
}

// ParseV1SearchProviders validates the provider filter. Empty means
// both catalog providers; anything else must name unsplash|pexels.
func ParseV1SearchProviders(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{string(domain.ProviderUnsplash), string(domain.ProviderPexels)}, nil
	}
	seen := make(map[string]struct{}, len(raw))
	providers := make([]string, 0, len(raw))
	for _, candidate := range raw {
		trimmed := strings.ToLower(strings.TrimSpace(candidate))
		if trimmed != string(domain.ProviderUnsplash) && trimmed != string(domain.ProviderPexels) {
			return nil, ErrV1SearchProviderInvalid
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		providers = append(providers, trimmed)
	}
	if len(providers) == 0 {
		return []string{string(domain.ProviderUnsplash), string(domain.ProviderPexels)}, nil
	}
	return providers, nil
}

// V1VisionQueryInput is the validated v1 vision request: free text plus
// owned workspace library references (no client bytes).
type V1VisionQueryInput struct {
	UserText        string
	LibraryAssetIDs []string
}

// ValidateV1VisionQueryInput enforces text/refs presence, the 200-rune
// text bound, at most 4 refs, and lasset_ shape per ref (existence and
// workspace binding are checked against the library store at the
// handler; hidden refs converge to 404 there).
func ValidateV1VisionQueryInput(input V1VisionQueryInput) (V1VisionQueryInput, error) {
	normalized := V1VisionQueryInput{UserText: strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")}
	if utf8.RuneCountInString(normalized.UserText) > V1MaxQueryRunes {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	seen := make(map[string]struct{}, len(input.LibraryAssetIDs))
	for _, raw := range input.LibraryAssetIDs {
		trimmed := strings.TrimSpace(raw)
		if !contract.IsValidPublicID(library.V1AssetPrefix, trimmed) {
			return V1VisionQueryInput{}, ErrV1VisionInput
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized.LibraryAssetIDs = append(normalized.LibraryAssetIDs, trimmed)
	}
	if len(normalized.LibraryAssetIDs) > V1MaxVisionAssetRefs {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	if normalized.UserText == "" && len(normalized.LibraryAssetIDs) == 0 {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	return normalized, nil
}

// V1VisionReference is one server-resolved vision input: provider URL +
// source metadata handed to the extractor seam. Bytes never cross this
// boundary (provider fetch stays #808-owned).
type V1VisionReference struct {
	Provider   string
	SourceID   string
	SourceURL  string
	PreviewURL string
	Title      string
}
