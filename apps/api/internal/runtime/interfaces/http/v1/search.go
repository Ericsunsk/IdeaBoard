package v1

// Workspace-scoped search + vision queries (issue #806).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/search via the existing
// runtimehttp.ContractV1GroupPath + runtimehttp.PublicContractChain pattern. Legacy
// /api/search + /api/user vision routes are untouched (removal is
// #809).
//
// Agreed surface:
//
//	GET  /workspaces/{workspace_id}/search (stable cursor pagination, provider filter, explicit query validation)
//	POST /workspaces/{workspace_id}/search/vision-queries (workspace Billing Account quote + settlement, idempotent scope search:vision)
//
// Auth model: runtimehttp.ResolvePrincipal + runtimehttp.RequireWorkspaceCapability for the
// explicit workspace context. Any workspace member searches and files
// vision queries; outsiders converge to 404 workspace_not_found.
// Provider results are provider URL + source metadata only (never
// bytes), mirroring the library asset boundary.
//
// Search failures map to the shared error contract: explicit query
// validation reuses validation_failed (422, details.fields names q,
// provider, library_asset_ids, user_text); malformed cursors reuse
// pagination_cursor_invalid (400); all-providers-failed renders the
// frozen search_provider_unavailable (503); vision provider failures
// render search_vision_provider_error (502); unconfigured vision
// renders search_vision_not_configured (503); key reuse with a
// different request renders search_idempotency_conflict (409). Vision
// billing failures reuse the #804 codes (billing_quote_unavailable
// 503 transient, billing_wallet_restricted 403) plus the member quota
// 403 (member_quota_exhausted #946); unpriced vision SKUs
// (catalog lacks a price) degrade to free with a billing_deferred
// metric + backfill note instead of 422, proceeding at amount 0 with
// ledger Hold/Capture/Release skipped.
//
// Vision queries settle billing through the credits.Commands path
// (V1CreditSettler Hold/Capture/Release with caller idempotency keys;
// no direct ledger writes here) against a server-side quote for the
// ai-search:vision SKU. Vision inputs reference owned workspace
// library assets by lasset_ id (resolved server-side; cross-workspace
// refs converge to 404 library_asset_not_found), staged workspace
// uploads by upload_ id (stage-then-reference through the
// upload-session byte PUT; unknown ids 404, expired 410, failed 409,
// unstaged 422 — client bytes never cross this boundary, so no
// multipart surface is introduced), and external images by
// allowlisted https URL (the tree CDN allowlist; anything else 422).
// Provider image fetch for extraction stays #808-owned (Control
// provider config); the extractor seam receives resolved references
// only.
//
// Production wiring leaves ContractV1SearchDeps zero-valued until the
// search/vision/billing adapters land (#807 wires provider fetch,
// #809 the durable stores); nil backend/quoter/settler/extractor fail
// closed with 503, nil library store with 501. Tests inject fakes.

import (
	"context"
	"errors"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/identity/principal"
	libraryports "ideaboard/api/internal/library/ports"
	mediadomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// v1VisionBillingDeferredObserve emits the billing_deferred metric +
// backfill note for unpriced vision SKUs via the existing slog
// observability pattern (closed labels only: sku, reason). It is a
// package-level hook so tests can stub metric emission; production uses
// slog. The two catalog sentinels stay distinct via the reason label.
var v1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
	slog.Warn("vision search billing deferred: catalog lacks price; proceeding free with backfill note",
		"sku", sku, "reason", reason, "operation_id", operationID,
		"backfill", "vision query needs backfill for unpriced SKU")
}

// v1VisionBillingDeferredReason preserves the catalog miss distinction
// (do NOT merge the sentinels).
func v1VisionBillingDeferredReason(err error) (string, bool) {
	if errors.Is(err, billing.ErrSKUNotFound) {
		return "sku_not_found", true
	}
	if errors.Is(err, billing.ErrPricingUnavailable) {
		return "pricing_unavailable", true
	}
	return "", false
}

// V1SearchBackend serves one normalized workspace search page. The
// providers filter is honored by the backend (post-filter fill stays
// bounded inside the adapter). Results carry provider URL + source
// metadata only. Implementations must return stable opaque cursors
// bound to (query, providers); the zero backend fails closed with 503
// search_provider_unavailable.
type V1SearchBackend interface {
	Search(ctx context.Context, query string, providers []string, limit int, cursor string) (items []searchcatalog.V1SearchResult, nextCursor string, hasMore bool, err error)
}

// V1VisionExtractor extracts a normalized query from resolved vision
// references. It receives provider URL + source metadata only — never
// bytes. Seams return errV1VisionNotConfigured when vision has no
// model, or errV1VisionProviderError on provider failures (the handler
// releases the hold and maps codes).
type V1VisionExtractor interface {
	ExtractVisionQuery(ctx context.Context, input V1VisionExtractInput) (string, error)
}

// V1VisionExtractInput is one extractor call: free text plus
// server-resolved owned references.
type V1VisionExtractInput struct {
	WorkspacePublicID string
	UserText          string
	References        []searchcatalog.V1VisionReference
}

var (
	// errV1SearchUnavailable reports all-providers-failed from the
	// backend (503 search_provider_unavailable).
	errV1SearchUnavailable = errors.New("v1 search providers unavailable")
	// errV1VisionNotConfigured reports unconfigured vision (503
	// search_vision_not_configured).
	errV1VisionNotConfigured = errors.New("v1 vision not configured")
	// errV1VisionProviderError reports vision provider failures (502
	// search_vision_provider_error).
	errV1VisionProviderError = errors.New("v1 vision provider error")
)

// V1VisionSKU reuses the vision billing SKU from the application
// vision_billing service (quote + hold/capture/release semantics are
// identical; only the workspace authZ + idempotency scope differ).
const V1VisionSKU = "ai-search:vision"

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

func (h *contractV1Handler) registerSearchRoutes(group *gin.RouterGroup) {
	group.GET("/workspaces/:workspace_id/search", h.searchWorkspace)
	group.POST("/workspaces/:workspace_id/search/vision-queries", h.createVisionQuery)
}

// searchWorkspaceContext resolves the explicit workspace context and
// enforces membership: hidden workspaces converge to 404
// workspace_not_found, non-members to the same 404.
func (h *contractV1Handler) searchWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, bool) {
	workspace, _, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return nil, false
	}
	if !runtimehttp.RequireWorkspaceCapability(c, visible, visible) {
		return nil, false
	}
	return workspace, true
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

func (h *contractV1Handler) toSearchResultDTO(result searchcatalog.V1SearchResult) v1SearchResultDTO {
	return v1SearchResultDTO{
		Provider: result.Provider, SourceID: result.SourceID,
		Title: result.Title, Author: result.Author,
		SourceURL: result.SourceURL, PreviewURL: result.PreviewURL, OriginalURL: result.OriginalURL,
		Width: result.Width, Height: result.Height,
		Display: h.deps.MediaPreviews.SearchDisplay(result.Provider, result.SourceID, result.PreviewURL),
	}
}

func (h *contractV1Handler) searchWorkspace(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Resolve: h.resolveSearchWorkspace}, func(wctx v1WorkspaceCtx) {
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
		pageSize, ok := v1PageSize(c)
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

// errV1CursorInvalid classifies v1 search cursor binding failures
// (wrong query/filter/page-size binding, malformed bytes) into
// pagination_cursor_invalid at the handler.
var errV1CursorInvalid = errors.New("v1 search cursor invalid")

func v1ProviderSelected(providers []string, candidate string) bool {
	for _, provider := range providers {
		if provider == candidate {
			return true
		}
	}
	return false
}

type v1CreateVisionQueryBody struct {
	Data struct {
		UserText        string   `json:"user_text"`
		LibraryAssetIDs []string `json:"library_asset_ids"`
		UploadIDs       []string `json:"upload_ids"`
		ImageURLs       []string `json:"image_urls"`
	} `json:"data"`
}

type v1VisionQueryDTO struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Query       string `json:"query"`
	Replayed    bool   `json:"replayed,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func (h *contractV1Handler) createVisionQuery(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Resolve: h.resolveSearchWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Search.Library == nil {
			contract.RenderError(c, http.StatusNotImplemented,
				contract.CodeContractNotImplemented, "The v1 search surface is not implemented yet")
			c.Abort()
			return
		}
		if h.deps.Search.Quoter == nil || h.deps.Search.Settler == nil {
			renderSearchBillingError(c, errV1BillingQuoteUnavailable)
			return
		}
		if h.deps.Search.Extractor == nil {
			renderSearchError(c, errV1VisionNotConfigured)
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateVisionQueryBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		input, validateErr := searchcatalog.ValidateV1VisionQueryInput(searchcatalog.V1VisionQueryInput{
			UserText: body.Data.UserText, LibraryAssetIDs: body.Data.LibraryAssetIDs,
			UploadIDs: body.Data.UploadIDs, ImageURLs: body.Data.ImageURLs,
		})
		if validateErr != nil {
			renderSearchError(c, validateErr)
			return
		}
		references, ok := h.resolveVisionReferences(c, workspace, input.LibraryAssetIDs)
		if !ok {
			return
		}
		uploadReferences, ok := h.resolveVisionUploads(c, workspace, input.UploadIDs)
		if !ok {
			return
		}
		references = append(references, uploadReferences...)
		for _, raw := range input.ImageURLs {
			references = append(references, searchcatalog.V1VisionReference{
				Kind: searchcatalog.V1VisionReferenceExternal, ImageURL: raw,
			})
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hashParts := append([]string{"search:vision", workspace.PublicID, input.UserText}, input.LibraryAssetIDs...)
		hashParts = append(hashParts, input.UploadIDs...)
		hashParts = append(hashParts, input.ImageURLs...)
		hash := contract.HashIdempotencyRequest(hashParts...)
		h.deps.Search.normalize()
		scope := contract.IdempotencyScope(externalUser, "search:vision")
		result, replayed, doErr := h.deps.Search.Idempotency.Do(c.Request.Context(), scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			return h.runVisionQuery(c, workspace, identityPrincipal, input, references)
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeSearchIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			// Quota failures ride the same billing branch as the
			// wallet/quote gates so they converge to the member quota
			// 403 (member_quota_exhausted) instead of the 503 default below.
			// The quote path wraps failures with multi-%w alongside
			// errV1BillingQuoteUnavailable; errors.Is still matches the
			// quota cause through that chain, and
			// renderSearchBillingError checks that case first.
			if errors.Is(doErr, errV1BillingRestricted) || errors.Is(doErr, errV1BillingQuoteUnavailable) ||
				errors.Is(doErr, memberquotas.ErrQuotaExhausted) {
				renderSearchBillingError(c, doErr)
				return
			}
			renderSearchError(c, doErr)
			return
		}
		dto, ok := result.(v1VisionQueryDTO)
		if !ok {
			renderSearchError(c, errors.New("search: vision-query type mismatch"))
			return
		}
		dto.Replayed = replayed
		contract.RenderCreated(c, dto)
	})
}

func renderSearchBillingError(c *gin.Context, err error) {
	if renderBillingPricingUnavailable(c, err) {
		c.Abort()
		return
	}
	switch {
	// Member quota (#946) gate mirrors the Task-side quota feedback:
	// 403 member_quota_exhausted, distinct from 403 billing_wallet_restricted
	// and 503 billing_quote_unavailable. This case stays first: the
	// quote path wraps failures with multi-%w alongside
	// errV1BillingQuoteUnavailable, so a wrapped quota cause must
	// win over the transient branch below.
	case errors.Is(err, memberquotas.ErrQuotaExhausted):
		contract.RenderError(c, contract.StatusOf(contract.CodeMemberQuotaExhausted),
			contract.CodeMemberQuotaExhausted, runtimehttp.MsgMemberQuotaExhausted)
	case errors.Is(err, errV1BillingRestricted):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, runtimehttp.MsgBillingWalletRestricted2)
	case errors.Is(err, errV1BillingQuoteUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeBillingQuoteUnavailable, runtimehttp.MsgBillingQuoteUnavailable)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgSearchProviderUnavailable)
	}
	c.Abort()
}

// resolveVisionReferences loads owned library assets for the vision
// input. Unknown, malformed, or cross-workspace ids converge to 404
// library_asset_not_found (the extractor never sees unowned refs).
func (h *contractV1Handler) resolveVisionReferences(c *gin.Context, workspace *tenancy.Workspace, assetIDs []string) ([]searchcatalog.V1VisionReference, bool) {
	references := make([]searchcatalog.V1VisionReference, 0, len(assetIDs))
	for _, id := range assetIDs {
		asset, err := h.deps.Search.Library.GetAsset(c.Request.Context(), workspace.PublicID, id)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, runtimehttp.MsgLibraryAssetNotFound)
			c.Abort()
			return nil, false
		}
		references = append(references, searchcatalog.V1VisionReference{
			Provider: asset.Provider, SourceID: asset.SourceID,
			SourceURL: asset.SourceURL, PreviewURL: asset.PreviewURL, Title: asset.Title,
			Kind: searchcatalog.V1VisionReferenceLibrary,
		})
	}
	return references, true
}

// errV1VisionUploadUnstaged reports a staged-upload vision ref whose
// bytes were never PUT (422: the caller must stage first, then retry
// the same vision call).
var errV1VisionUploadUnstaged = errors.New("v1 vision upload has no staged bytes")

// resolveVisionUploads resolves staged-upload vision refs (the
// stage-then-reference half of the vision file-input gap). Unknown,
// malformed, or cross-workspace ids converge to 404 upload_not_found;
// expired sessions to 410 upload_expired; failed sessions to 409
// upload_state_conflict. Completed sessions stay usable (their bytes
// were verified at complete time). Live sessions must have staged
// bytes readable through the byte seam, verified against the session
// declaration (unstaged reads 422); the session itself is never
// failed here — vision is a reader, not the transfer owner.
func (h *contractV1Handler) resolveVisionUploads(c *gin.Context, workspace *tenancy.Workspace, uploadIDs []string) ([]searchcatalog.V1VisionReference, bool) {
	if len(uploadIDs) == 0 {
		return nil, true
	}
	if h.deps.Media == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 vision-upload surface is not implemented yet")
		c.Abort()
		return nil, false
	}
	if h.deps.MediaObjects == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable54)
		c.Abort()
		return nil, false
	}
	ctx := c.Request.Context()
	references := make([]searchcatalog.V1VisionReference, 0, len(uploadIDs))
	for _, id := range uploadIDs {
		session, err := h.deps.Media.GetSession(ctx, workspace.PublicID, id)
		if err != nil {
			switch {
			case errors.Is(err, workspacemedia.ErrExpired):
				contract.RenderError(c, contract.StatusOf(contract.CodeUploadExpired),
					contract.CodeUploadExpired, runtimehttp.MsgUploadExpired)
			case errors.Is(err, workspacemedia.ErrStateConflict):
				contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
					contract.CodeUploadStateConflict, runtimehttp.MsgUploadStateConflict)
			default:
				contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
			}
			c.Abort()
			return nil, false
		}
		if session.Status == workspacemedia.StatusFailed {
			contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
				contract.CodeUploadStateConflict, runtimehttp.MsgUploadStateConflict)
			c.Abort()
			return nil, false
		}
		object, err := h.deps.MediaObjects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, workspacemedia.MaxUploadSizeBytes)
		if err != nil || object == nil || len(object.Data) == 0 || int64(len(object.Data)) != session.SizeBytes ||
			v1SniffedImageMIME(object.Data) != session.MIMEType || v1StagedChecksum(object.Data) != session.ChecksumSHA256 {
			renderSearchError(c, errV1VisionUploadUnstaged)
			return nil, false
		}
		references = append(references, searchcatalog.V1VisionReference{
			Kind: searchcatalog.V1VisionReferenceUpload, UploadID: session.ID,
			ObjectKey: session.StorageKey, MIMEType: session.MIMEType,
		})
	}
	return references, true
}

// runVisionQuery executes one vision query inside the search:vision
// idempotency scope: server-side quote for the workspace Billing
// Account, credit hold through credits.Commands, provider extraction,
// then capture (201 with the normalized query) or release (provider
// failure maps to 502/503 with the hold refunded).
func (h *contractV1Handler) runVisionQuery(c *gin.Context, workspace *tenancy.Workspace, identityPrincipal principal.Principal, input searchcatalog.V1VisionQueryInput, references []searchcatalog.V1VisionReference) (any, error) {
	ctx := c.Request.Context()
	amount, quoteErr := h.deps.Search.Quoter.Quote(ctx, workspace.PublicID, V1VisionSKU)
	deferredFree := false
	deferredReason := ""
	if quoteErr != nil {
		if reason, ok := v1VisionBillingDeferredReason(quoteErr); ok {
			// Catalog lacks a price: allow the user-facing read with a
			// billing_deferred metric + backfill note, proceeding at
			// amount 0 (free-marker: ledger Hold/Capture/Release skipped).
			// 403 wallet-restricted and transient 503 paths below stay
			// fail-closed. The sentinels stay distinct via reason.
			amount, deferredFree, deferredReason = 0, true, reason
		} else {
			// Preserve the underlying billing cause alongside the
			// quote-unavailable identity (multi-%w): transient failures
			// keep the 503 billing_quote_unavailable branch. The
			// createVisionQuery gate still matches
			// errV1BillingQuoteUnavailable, so routing is unchanged.
			return nil, fmt.Errorf("search vision quote: %w: %w", quoteErr, errV1BillingQuoteUnavailable)
		}
	}
	if amount < 0 {
		return nil, errV1BillingQuoteUnavailable
	}
	if amount == 0 {
		deferredFree = true
	}
	operationID, mintErr := searchcatalog.NewV1VisionQueryPublicID()
	if mintErr != nil {
		return nil, mintErr
	}
	if deferredReason != "" {
		v1VisionBillingDeferredObserve(V1VisionSKU, deferredReason, operationID)
	} else if deferredFree {
		// Quoted zero without a catalog error: same free-marker degrade
		// with a backfill note so unpriced reads stay attributable.
		v1VisionBillingDeferredObserve(V1VisionSKU, "unpriced_zero", operationID)
	}
	if deferredFree {
		// Free-marker: skip ledger Hold/Capture/Release entirely.
		extracted, extractErr := h.deps.Search.Extractor.ExtractVisionQuery(ctx, V1VisionExtractInput{
			WorkspacePublicID: workspace.PublicID, UserText: input.UserText, References: references,
		})
		if extractErr != nil {
			if errors.Is(extractErr, errV1VisionNotConfigured) {
				return nil, errV1VisionNotConfigured
			}
			return nil, errV1VisionProviderError
		}
		normalized, normalizeErr := searchcatalog.NormalizeV1SearchQuery(extracted)
		if normalizeErr != nil {
			return nil, errV1VisionProviderError
		}
		return v1VisionQueryDTO{
			OperationID: operationID, Status: "succeeded", Query: normalized,
			CreatedAt: v1FormatTime(time.Now().UTC()),
		}, nil
	}
	holdKey := "search-vision:" + operationID + ":hold"
	holdID, holdErr := h.deps.Search.Settler.Hold(ctx, workspace.PublicID, identityPrincipal.UserID(), amount, holdKey, "search vision query")
	if holdErr != nil {
		if errors.Is(holdErr, errV1BillingRestricted) {
			return nil, errV1BillingRestricted
		}
		return nil, holdErr
	}
	extracted, extractErr := h.deps.Search.Extractor.ExtractVisionQuery(ctx, V1VisionExtractInput{
		WorkspacePublicID: workspace.PublicID, UserText: input.UserText, References: references,
	})
	if extractErr != nil {
		_ = h.deps.Search.Settler.Release(ctx, workspace.PublicID, identityPrincipal.UserID(), holdID, amount, "search-vision:"+operationID+":release")
		if errors.Is(extractErr, errV1VisionNotConfigured) {
			return nil, errV1VisionNotConfigured
		}
		return nil, errV1VisionProviderError
	}
	normalized, normalizeErr := searchcatalog.NormalizeV1SearchQuery(extracted)
	if normalizeErr != nil {
		_ = h.deps.Search.Settler.Release(ctx, workspace.PublicID, identityPrincipal.UserID(), holdID, amount, "search-vision:"+operationID+":release")
		return nil, errV1VisionProviderError
	}
	if captureErr := h.deps.Search.Settler.Capture(ctx, workspace.PublicID, identityPrincipal.UserID(), holdID, amount, "search-vision:"+operationID+":capture"); captureErr != nil {
		_ = h.deps.Search.Settler.Release(ctx, workspace.PublicID, identityPrincipal.UserID(), holdID, amount, "search-vision:"+operationID+":release")
		return nil, captureErr
	}
	return v1VisionQueryDTO{
		OperationID: operationID, Status: "succeeded", Query: normalized,
		CreatedAt: v1FormatTime(time.Now().UTC()),
	}, nil
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tags: WorkspaceSearch), reusing components parameters
// Cursor/PageSize/WorkspaceId/IdempotencyKey/RequestId and responses
// BadRequest/Unauthorized/Forbidden/NotFound/Conflict/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/search:
//	  get: {operationId: searchWorkspace, parameters: [WorkspaceId, Cursor, PageSize, {name: q, in: query, required: true, schema: {type: string, maxLength: 200}}, {name: provider, in: query, schema: {type: array, items: {type: string, enum: [unsplash, pexels]}}}]} (q blank or over-limit renders validation_failed 422, never a silent empty page)
//	/workspaces/{workspace_id}/search/vision-queries:
//	  post: {operationId: createWorkspaceVisionQuery, parameters: [WorkspaceId, IdempotencyKey]}
//
// Schemas (results are provider URL + source metadata only, never bytes):
//
//	WorkspaceSearchResult: {provider: {enum: [unsplash, pexels]}, source_id, title?, author?, source_url: uri, preview_url: uri, original_url: uri, width: integer>=0, height: integer>=0}
//	CreateWorkspaceVisionQueryRequest: {data: {user_text?: string (maxLength 200), library_asset_ids?: {type: array, items: {type: string, pattern: '^lasset_'}}, upload_ids?: {type: array, items: {type: string, pattern: '^upload_'}}, image_urls?: {type: array, items: {type: string, format: uri}}}} (maxItems 4 across all three ref kinds combined; at least one of user_text/refs required; library refs resolve against owned workspace library assets, upload refs against live-or-completed sessions with staged bytes verified at resolve time (unknown 404 upload_not_found, expired 410, failed 409, unstaged 422), external refs must be allowlisted https CDN URLs (images.unsplash.com, plus.unsplash.com, images.pexels.com) or 422; no client bytes anywhere)
//	WorkspaceVisionQuery: {operation_id: {pattern: '^visq_'}, status: {const: succeeded}, query: string, replayed?: boolean, created_at: date-time}
//
// Error codes frozen: search_provider_unavailable (503), search_vision_provider_error (502),
// search_vision_not_configured (503), search_idempotency_conflict (409); reused kernel codes
// workspace_not_found/library_asset_not_found (404), validation_failed (422),
// pagination_cursor_invalid (400), bad_request (400), idempotency_key_required (400),
// service_unavailable (503) plus #804 billing_quote_unavailable (503
// transient)/billing_wallet_restricted (403) plus the member quota
// 403 member_quota_exhausted (#946).
// Unpriced vision SKUs never
// surface billing_pricing_unavailable (422): they degrade to free with a
// billing_deferred metric + backfill note (amount 0, ledger skipped).
//
// Events note for #807: search/vision lifecycle events (vision query
// settled, materialization completed) are NOT emitted here; #807 owns
// the library/search event catalog on NATS JetStream.
