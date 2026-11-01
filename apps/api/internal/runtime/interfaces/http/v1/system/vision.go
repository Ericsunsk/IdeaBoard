package system

// Workspace-scoped vision queries for the Public API v1 (issue #806).
//
//	POST /workspaces/{workspace_id}/search/vision-queries (workspace Billing Account quote + settlement, idempotent scope search:vision)
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
// multipart surface is introduced), and external images by public
// https URL. Provider image fetch for extraction stays #808-owned
// (Control provider config); the extractor seam receives resolved
// references only.
//
// Vision provider failures render search_vision_provider_error (502);
// unconfigured vision renders search_vision_not_configured (503); key
// reuse with a different request renders search_idempotency_conflict
// (409). Vision billing failures reuse the #804 codes
// (billing_quote_unavailable 503 transient, billing_wallet_restricted
// 403) plus the member quota 403 (member_quota_exhausted #946);
// unpriced vision SKUs (catalog lacks a price) degrade to free with a
// billing_deferred metric + backfill note instead of 422, proceeding
// at amount 0 with ledger Hold/Capture/Release skipped.

import (
	"context"
	"errors"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"log/slog"
	"net/http"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	v1billing "ideaboard/api/internal/runtime/interfaces/http/v1/billing"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// V1VisionBillingDeferredObserve emits the billing_deferred metric +
// backfill note for unpriced vision SKUs via the existing slog
// observability pattern (closed labels only: sku, reason). It is a
// package-level hook so tests can stub metric emission; production uses
// slog. The two catalog sentinels stay distinct via the reason label.
var V1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
	slog.Warn("vision search billing deferred: catalog lacks price; proceeding free with backfill note",
		"sku", sku, "reason", reason, "operation_id", operationID,
		"backfill", "vision query needs backfill for unpriced SKU")
}

var v1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
	V1VisionBillingDeferredObserve(sku, reason, operationID)
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

// V1CreditSettler settles metered workspace operations through the
// credits.Commands path: Hold reserves, Capture consumes on success,
// Release refunds on failure. Every command carries a caller-owned
// idempotency key so retries converge. Only Credits owns ledger writes;
// this seam performs none. Production adapts *credits.Service;
// tests inject fakes.
type V1CreditSettler interface {
	Hold(ctx context.Context, workspacePublicID string, actorUserID int64, amountCredits int64, idempotencyKey, reason string) (string, error)
	Capture(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error
	Release(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error
}

// V1BillingQuoter mints server-side price snapshots for metered
// workspace operations. No client price input is accepted on any route.
// Production adapts the billing catalog QuoteAtResolution seam;
// tests inject fakes.
type V1BillingQuoter interface {
	Quote(ctx context.Context, workspacePublicID, sku string) (int64, error)
}

var (
	// ErrV1BillingQuoteUnavailable marks transient quoter failures for
	// the 503 billing_quote_unavailable branch. Consumed by the search surface.
	ErrV1BillingQuoteUnavailable = errors.New("v1 billing quote unavailable")
	errV1BillingQuoteUnavailable = ErrV1BillingQuoteUnavailable
	// ErrV1BillingRestricted maps hold rejections (frozen/restricted/
	// insufficient wallets) to 403 billing_wallet_restricted. Consumed by the search surface.
	ErrV1BillingRestricted = errors.New("v1 billing wallet restricted")
	errV1BillingRestricted = ErrV1BillingRestricted
)

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

func (h *Handler) createVisionQuery(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Resolve: h.resolveSearchWorkspace}, func(wctx shared.WorkspaceCtx) {
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
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
			shared.WriteInputError(c)
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
	if v1billing.RenderPricingUnavailable(c, err) {
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

func (h *Handler) runVisionQuery(c *gin.Context, workspace *tenancy.Workspace, identityPrincipal principal.Principal, input searchcatalog.V1VisionQueryInput, references []searchcatalog.V1VisionReference) (any, error) {
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
			CreatedAt: shared.FormatTime(time.Now().UTC()),
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
		CreatedAt: shared.FormatTime(time.Now().UTC()),
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
//	CreateWorkspaceVisionQueryRequest: {data: {user_text?: string (maxLength 200), library_asset_ids?: {type: array, items: {type: string, pattern: '^lasset_'}}, upload_ids?: {type: array, items: {type: string, pattern: '^upload_'}}, image_urls?: {type: array, items: {type: string, format: uri}}}} (maxItems 4 across all three ref kinds combined; at least one of user_text/refs required; library refs resolve against owned workspace library assets, upload refs against live-or-completed sessions with staged bytes verified at resolve time (unknown 404 upload_not_found, expired 410, failed 409, unstaged 422), external refs must be public https image URLs (no exotic ports, no userinfo, no literal private IPs) or 422; no client bytes anywhere)
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
