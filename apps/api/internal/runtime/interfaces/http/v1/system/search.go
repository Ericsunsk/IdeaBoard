package system

// Workspace-scoped search for the Public API v1 (issue #806).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/search via the existing
// runtimehttp.ContractV1GroupPath + runtimehttp.PublicContractChain pattern.
// Legacy /api/search routes are untouched (removal is #809).
//
// Agreed surface:
//
//	GET /workspaces/{workspace_id}/search (stable cursor pagination, provider filter, explicit query validation)
//
// Auth model: runtimehttp.ResolvePrincipal + runtimehttp.RequireWorkspaceCapability
// for the explicit workspace context. Any workspace member searches;
// outsiders converge to 404 workspace_not_found. Provider results are
// provider URL + source metadata only (never bytes), mirroring the
// library asset boundary.
//
// Search failures map to the shared error contract: explicit query
// validation reuses validation_failed (422, details.fields names q,
// provider); malformed cursors reuse pagination_cursor_invalid (400);
// all-providers-failed renders the frozen search_provider_unavailable
// (503).
//
// Production wiring leaves ContractV1SearchDeps zero-valued until the
// search/vision/billing adapters land (#807 wires provider fetch,
// #809 the durable stores); nil backend fails closed with 503, nil
// library store with 501. Tests inject fakes.
//
// File split (same package): search.go owns the keyword-search edge,
// vision.go the vision-query edge plus billing settlement, vision_refs.go
// the reference-resolution helpers.

import (
	"context"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	libraryports "ideaboard/api/internal/library/ports"
	mediadomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)
// V1SearchBackend serves one normalized workspace search page. The
// providers filter is honored by the backend (post-filter fill stays
// bounded inside the adapter). Results carry provider URL + source
// metadata only. Implementations must return stable opaque cursors
// bound to (query, providers); the zero backend fails closed with 503
// search_provider_unavailable.
type V1SearchBackend interface {
	Search(ctx context.Context, query string, providers []string, limit int, cursor string) (items []searchcatalog.V1SearchResult, nextCursor string, hasMore bool, err error)
}

// errV1SearchUnavailable reports all-providers-failed from the
// backend (503 search_provider_unavailable).
var errV1SearchUnavailable = errors.New("v1 search providers unavailable")

// ContractV1SearchDeps wires the workspace search handlers. Zero values
// fail closed; production fills every field when the adapters land
// (#807/#809).
type ContractV1SearchDeps struct {
	Backend     V1SearchBackend
	Library     libraryports.V1Store
	Quoter      V1BillingQuoter
	Settler     V1CreditSettler
	Extractor   V1VisionExtractor
	Idempotency *contract.IdempotencyStore
}

func (d *ContractV1SearchDeps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
}

// searchWorkspaceContext resolves the explicit workspace context and
// enforces membership: hidden workspaces converge to 404
// workspace_not_found, non-members to the same 404.
func (h *Handler) searchWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, bool) {
	workspace, _, visible := shared.ResolveWorkspaceContext(c, h.deps.Store, identityPrincipal)
	if workspace == nil {
		return nil, false
	}
	if !runtimehttp.RequireWorkspaceCapability(c, visible, visible) {
		return nil, false
	}
	return workspace, true
}

func (h *Handler) resolveSearchWorkspace(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
	workspace, ok := h.searchWorkspaceContext(c, p)
	if !ok {
		return nil, "", false
	}
	return workspace, "", true
}

func renderSearchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, searchcatalog.ErrV1SearchQueryEmpty) || errors.Is(err, searchcatalog.ErrV1SearchQueryTooLong):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderSearchError, contract.FieldErrors{Fields: map[string]string{"q": "invalid"}})
	case errors.Is(err, searchcatalog.ErrV1SearchProviderInvalid):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderSearchError2, contract.FieldErrors{Fields: map[string]string{"provider": "invalid"}})
	case errors.Is(err, searchcatalog.ErrV1VisionInput):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderSearchError3, contract.FieldErrors{Fields: map[string]string{"vision": "invalid"}})
	case errors.Is(err, errV1VisionUploadUnstaged):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderSearchError3, contract.FieldErrors{Fields: map[string]string{"upload_ids": "unstaged"}})
	case errors.Is(err, errV1SearchUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeSearchProviderUnavailable, runtimehttp.MsgSearchProviderUnavailable)
	case errors.Is(err, errV1VisionProviderError):
		contract.RenderError(c, http.StatusBadGateway, contract.CodeSearchVisionProviderError, runtimehttp.MsgSearchVisionProviderError)
	case errors.Is(err, errV1VisionNotConfigured):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeSearchVisionNotConfigured, runtimehttp.MsgSearchVisionNotConfigured)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgSearchProviderUnavailable)
	}
	c.Abort()
}

type v1SearchResultDTO struct {
	Provider    string               `json:"provider"`
	SourceID    string               `json:"source_id"`
	Title       string               `json:"title,omitempty"`
	Author      string               `json:"author,omitempty"`
	SourceURL   string               `json:"source_url"`
	PreviewURL  string               `json:"preview_url"`
	OriginalURL string               `json:"original_url"`
	Width       int                  `json:"width"`
	Height      int                  `json:"height"`
	Display     *mediadomain.Display `json:"display,omitempty"`
}

func (h *Handler) toSearchResultDTO(result searchcatalog.V1SearchResult) v1SearchResultDTO {
	return v1SearchResultDTO{
		Provider: result.Provider, SourceID: result.SourceID,
		Title: result.Title, Author: result.Author,
		SourceURL: result.SourceURL, PreviewURL: result.PreviewURL, OriginalURL: result.OriginalURL,
		Width: result.Width, Height: result.Height,
		Display: h.deps.Media.MediaPreviews.SearchDisplay(result.Provider, result.SourceID, result.PreviewURL),
	}
}

func (h *Handler) searchWorkspace(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Resolve: h.resolveSearchWorkspace}, func(wctx shared.WorkspaceCtx) {
		_ = wctx
		query, err := searchcatalog.NormalizeV1SearchQuery(c.Query("q"))
		if err != nil {
			renderSearchError(c, err)
			return
		}
		providers, err := searchcatalog.ParseV1SearchProviders(c.QueryArray("provider"))
		if err != nil {
			renderSearchError(c, err)
			return
		}
		pageSize, ok := shared.PageSize(c)
		if !ok {
			return
		}
		cursor := strings.TrimSpace(c.Query(contract.CursorParam))
		if h.deps.Search.Backend == nil {
			renderSearchError(c, errV1SearchUnavailable)
			return
		}
		items, nextCursor, hasMore, searchErr := h.deps.Search.Backend.Search(c.Request.Context(), query, providers, pageSize, cursor)
		if searchErr != nil {
			// Opaque cursor binding failures (wrong query/filter/page
			// size, malformed bytes) converge to pagination_cursor_invalid
			// (Baseline 5.5); backend outages stay 503.
			if errors.Is(searchErr, errV1CursorInvalid) || contract.IsCursorInvalid(searchErr) {
				contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
				c.Abort()
				return
			}
			renderSearchError(c, searchErr)
			return
		}
		page := make([]v1SearchResultDTO, 0, len(items))
		for _, item := range items {
			page = append(page, h.toSearchResultDTO(item))
		}
		contract.RenderPaginated(c, page, contract.NewPageMeta(nextCursor, hasMore))
	})
}

// ErrV1CursorInvalid classifies v1 search cursor binding failures
// (wrong query/filter/page-size binding, malformed bytes) into
// pagination_cursor_invalid at the handler.
var ErrV1CursorInvalid = errors.New("v1 search cursor invalid")
var errV1CursorInvalid = ErrV1CursorInvalid
