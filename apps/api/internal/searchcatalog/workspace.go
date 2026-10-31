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
//     workspace library store at the handler), staged workspace uploads
//     by upload_ id (stage-then-reference through the upload-session
//     byte PUT; existence, liveness, and staged bytes are checked at
//     the handler), and external images by public https URL
//     (IsV1VisionExternalImageURL). Client bytes never cross the v1
//     vision boundary, so no multipart surface is introduced.
//   - Vision billing reuses the ai-search:vision SKU semantics from the
//     application vision_billing service (quote + hold/capture/release
//     through credits.Commands); this file only bounds the input.

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/searchcatalog/domain"
)

const (
	// V1MaxQueryRunes aliases the canonical domain bound (single source;
	// never redeclare the literal).
	V1MaxQueryRunes = domain.QueryMaxRunes
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
// owned references (workspace library assets, staged workspace uploads,
// and allowlisted external image URLs). Client bytes never cross the
// boundary: upload bytes ride the upload-session byte PUT first and are
// referenced here by upload_ id only.
type V1VisionQueryInput struct {
	UserText        string
	LibraryAssetIDs []string
	UploadIDs       []string
	ImageURLs       []string
}

// IsV1VisionExternalImageURL reports whether raw is a fetchable external
// image URL: any public https URL (no exotic ports, no userinfo, no
// literal private/loopback IP). The curated CDN host allowlist is gone:
// the single gate is the safehttp transport at fetch time
// (private/loopback DNS + dial re-check, redirect re-validation, 3-hop
// cap). Literal private IPs fail fast here; DNS-resolved private targets
// still fail at fetch time as unavailable.
func IsV1VisionExternalImageURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return false
	}
	if ip := net.ParseIP(strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")); ip != nil && isPrivateVisionImageIP(ip) {
		return false
	}
	return true
}

// isPrivateVisionImageIP mirrors the safehttp transport block list for
// literal-IP fast rejection (DNS-resolved names are still enforced at
// fetch time, where TOCTOU-safe re-checks run).
func isPrivateVisionImageIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// ValidateV1VisionQueryInput enforces text/refs presence, the 200-rune
// text bound, at most 4 refs in total across all three ref kinds, and
// per-ref shapes (lasset_ for library refs, upload_ for staged-upload
// refs, public https for external URLs). Existence and workspace
// binding are checked against the owning stores at the handler; hidden
// refs converge to 404 there.
func ValidateV1VisionQueryInput(input V1VisionQueryInput) (V1VisionQueryInput, error) {
	normalized := V1VisionQueryInput{UserText: strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")}
	if utf8.RuneCountInString(normalized.UserText) > V1MaxQueryRunes {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	seen := make(map[string]struct{}, len(input.LibraryAssetIDs))
	for _, raw := range input.LibraryAssetIDs {
		trimmed := strings.TrimSpace(raw)
		if !contract.IsValidPublicID(librarydomain.V1AssetPrefix, trimmed) {
			return V1VisionQueryInput{}, ErrV1VisionInput
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized.LibraryAssetIDs = append(normalized.LibraryAssetIDs, trimmed)
	}
	seenUploads := make(map[string]struct{}, len(input.UploadIDs))
	for _, raw := range input.UploadIDs {
		trimmed := strings.TrimSpace(raw)
		if !contract.IsUploadID(trimmed) {
			return V1VisionQueryInput{}, ErrV1VisionInput
		}
		if _, ok := seenUploads[trimmed]; ok {
			continue
		}
		seenUploads[trimmed] = struct{}{}
		normalized.UploadIDs = append(normalized.UploadIDs, trimmed)
	}
	seenURLs := make(map[string]struct{}, len(input.ImageURLs))
	for _, raw := range input.ImageURLs {
		trimmed := strings.TrimSpace(raw)
		if !IsV1VisionExternalImageURL(trimmed) {
			return V1VisionQueryInput{}, ErrV1VisionInput
		}
		if _, ok := seenURLs[trimmed]; ok {
			continue
		}
		seenURLs[trimmed] = struct{}{}
		normalized.ImageURLs = append(normalized.ImageURLs, trimmed)
	}
	if len(normalized.LibraryAssetIDs)+len(normalized.UploadIDs)+len(normalized.ImageURLs) > V1MaxVisionAssetRefs {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	if normalized.UserText == "" && len(normalized.LibraryAssetIDs) == 0 && len(normalized.UploadIDs) == 0 && len(normalized.ImageURLs) == 0 {
		return V1VisionQueryInput{}, ErrV1VisionInput
	}
	return normalized, nil
}

// Vision reference kinds carried on V1VisionReference.Kind.
const (
	// V1VisionReferenceLibrary is an owned workspace library asset
	// (the original v1 vision input).
	V1VisionReferenceLibrary = "library"
	// V1VisionReferenceUpload is a staged workspace upload referenced
	// by upload_ id (ObjectKey locates the staged bytes server-side).
	V1VisionReferenceUpload = "upload"
	// V1VisionReferenceExternal is a public external image URL.
	V1VisionReferenceExternal = "external"
)

// V1VisionReference is one server-resolved vision input handed to the
// extractor seam. Library refs carry provider URL + source metadata;
// upload refs carry the staged byte locator (resolved server-side,
// never client bytes); external refs carry the validated public
// image URL. Bytes never cross this boundary (provider fetch stays
// #808-owned).
type V1VisionReference struct {
	Provider   string
	SourceID   string
	SourceURL  string
	PreviewURL string
	Title      string
	Kind       string
	UploadID   string
	ObjectKey  string
	MIMEType   string
	ImageURL   string
}
