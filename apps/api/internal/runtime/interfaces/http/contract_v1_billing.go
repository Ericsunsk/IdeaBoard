package httpapi

// Workspace Billing Account + Credits/Billing/Payments clean contracts
// (issue #804, backend slice).
//
// Additive only: legacy /api/user and /api/admin billing routes are
// untouched; Control financial ops are #808, not here. Do NOT edit
// apps/api/openapi*.yaml (the #802-804 merger applied the documented
// deltas at the bottom of this file to openapi.public.yaml).
//
// Agreed surface (ContractV1GroupPath + PublicContractChain, NO public
// billing-account ID in URLs; the account is implicit per workspace):
//
//	GET  /api/v1/workspaces/{workspace_id}/billing-account
//	GET  /api/v1/workspaces/{workspace_id}/credits/balance
//	GET  /api/v1/workspaces/{workspace_id}/credits/ledger (cursor pagination)
//	GET  /api/v1/workspaces/{workspace_id}/billing-records
//	GET  /api/v1/workspaces/{workspace_id}/usage
//	POST /api/v1/workspaces/{workspace_id}/checkout-sessions (header Idempotency-Key)
//	GET  /api/v1/workspaces/{workspace_id}/checkout-sessions/{session_id}
//
// Ownership (inviolable):
//   - ONLY internal/credits writes the ledger, via credits.Commands
//     (Grant/Hold/Capture/Release/Refund/Purchase...). This file performs
//     NO ledger writes; checkout settlement flows through the existing
//     Payments -> Credits transactional seam. Task submission uses the
//     workspace task settlement service instead of this billing read surface.
//   - Billing ONLY mints immutable PriceSnapshots/Quotes (server-side, no
//     client price input). Recharge checkout uses the server-side Payments
//     PurchaseQuote (IdeasPerUSD); no client amount/price is accepted.
//   - Payments ONLY owns checkout sessions + provider customer/methods +
//     webhook inbox (existing internal/payments seam, reused via adapter).
//     No new webhook route here; POST /api/webhooks/stripe is reused.
// Money: integer minor units (credits) throughout; ledger entries and
// snapshots are immutable INSERT-only (existing DB constraints).
// Nullable-vs-omitted: optional timestamps/URLs are omitted when empty;
// next_cursor is "" with has_more=false when exhausted (kernel PageMeta).

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

// Frozen workspace-billing error codes (wire-stable; mirrored in
// openapi.public.yaml and promoted to internal/platform/contract).
//
//   - billing_wallet_restricted (403): wallet frozen or under an active
//     restriction. Reuses the wallet_restricted semantics from the legacy
//     purchase-options unavailable_reason; frozen here as an explicit code.
//   - billing_checkout_conflict (409): Idempotency-Key reused with a
//     different checkout request (hash mismatch); replay otherwise.
//   - billing_session_not_found (404): unknown checkout session within a
//     visible workspace (anti-enumeration still 404s hidden workspaces
//     as workspace_not_found first).
//   - billing_quote_unavailable (503): server-side price snapshot failed
//     (transient).
//   - billing_pricing_unavailable (422): unpriced or unregistered SKU
//     (deterministic, do not retry).
//
// Kernel codes reused as-is: workspace_not_found (404 hidden),
// workspace_forbidden (403 visible-but-forbidden), pagination_cursor_invalid,
// bad_request, validation_failed (422), idempotency_key_required (400),
// service_unavailable.
// Render sites reference the canonical contract.Code* values directly;
// no v1-local aliases (single registration in platform/contract).

func renderBillingPricingUnavailable(c *gin.Context, err error) bool {
	if !errors.Is(err, billing.ErrPricingUnavailable) && !errors.Is(err, billing.ErrSKUNotFound) {
		return false
	}
	contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeBillingPricingUnavailable, msgBillingPricingUnavailable)
	return true
}

// billingAccountStore provisions the implicit per-workspace billing account.
// Production uses tenancypg.BillingAccountStore; tests stub this interface.
type billingAccountStore interface {
	EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (tenancypg.BillingAccount, error)
}

// billingWalletReader composes the Credits-Ledger read side. Only Credits
// owns ledger writes; this interface exposes reads.
type billingWalletReader interface {
	GetWallet(ctx context.Context, userID int64) (domain.WalletDetails, error)
	ListLedger(ctx context.Context, input domain.ListLedgerInput) (domain.LedgerPage, error)
}

// billingRecordsLister is the caller-owned read model over settled ledger
// facts + immutable task billing snapshots. No ledger writes.
type billingRecordsLister interface {
	List(ctx context.Context, input billingrecords.ListInput) (billingrecords.Page, error)
}

// billingUsageReader composes settled Captures + task snapshots. Read-only.
type billingUsageReader interface {
	GetCurrent(ctx context.Context, userID int64, usageRange usage.Range) (usage.Usage, error)
}

// billingQuoter mints immutable server-side price snapshots. No client
// price input is accepted on any route in this file.
type billingQuoter interface {
	QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error)
}

// workspaceCheckoutService owns checkout sessions bound to the workspace
// billing account. Production adapts the existing internal/payments
// checkout seam (namespaced idempotency key, server-side PurchaseQuote);
// webhook settlement stays inside Payments and settles ONLY through
// credits.Commands transactionally.
type workspaceCheckoutService interface {
	CreateWorkspaceCheckout(ctx context.Context, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error)
	GetWorkspaceCheckout(ctx context.Context, userID int64, workspacePublicID, sessionID string) (workspaceCheckoutStatus, error)
}

type workspaceCheckoutInput struct {
	IdeaQuantity        int64
	TermsAccepted       bool
	TermsVersion        string
	RefundPolicyVersion string
}

type workspaceCheckoutResult struct {
	SessionID   string
	CheckoutURL string
	ExpiresAt   time.Time
	Reused      bool
}

type workspaceCheckoutStatus struct {
	SessionID    string
	Status       string
	IdeaQuantity int64
	CheckoutURL  string
	ExpiresAt    time.Time
}

// ContractV1BillingDeps wires the workspace billing handlers. Zero values
// fail closed (501/503); production fills every field in http_wiring.go.
type ContractV1BillingDeps struct {
	Tenancy         tenancy.Store
	Users           V1UserDirectory
	Idempotency     *contract.IdempotencyStore
	BillingAccounts billingAccountStore
	Wallets         billingWalletReader
	Records         billingRecordsLister
	Usage           billingUsageReader
	Quoter          billingQuoter
	Checkout        workspaceCheckoutService
	// Gaps batch (#809-close, backend B): purchase options, pending
	// orders (continue/abandon), recharge history, billing quotes, and
	// rewards redeem/history. Zero values keep each route fail-closed;
	// production fills them in http_wiring.go. The Quoter above is
	// reused for billing-quotes (no second quoter).
	PurchaseOptions v1PurchaseOptionsService
	PendingOrders   v1PendingOrderService
	Recharges       v1RechargeHistoryService
	Rewards         v1RewardsUserService
}

func (d *ContractV1BillingDeps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
}

// RegisterContractV1BillingRoutesOnEngine mounts the workspace billing
// surface on its own /api/v1 group (same prefix, same PublicContractChain
// + session auth + tenant scope as the #801 group). Legacy routes untouched.
func RegisterContractV1BillingRoutesOnEngine(r *gin.Engine, authDeps AuthDeps, deps ContractV1BillingDeps) {
	deps.normalize()
	handler := &contractV1BillingHandler{deps: deps}
	group := r.Group(ContractV1GroupPath)
	group.Use(PublicContractChain()...)
	group.Use(v1AuthMiddleware(authDeps), ResolvePrincipal())
	group.GET("/workspaces/:workspace_id/billing-account", handler.getBillingAccount)
	group.GET("/workspaces/:workspace_id/credits/balance", handler.getCreditsBalance)
	group.GET("/workspaces/:workspace_id/credits/ledger", handler.listCreditsLedger)
	group.GET("/workspaces/:workspace_id/billing-records", handler.listBillingRecords)
	group.GET("/workspaces/:workspace_id/usage", handler.getUsage)
	group.POST("/workspaces/:workspace_id/checkout-sessions", handler.createCheckoutSession)
	group.GET("/workspaces/:workspace_id/checkout-sessions/:session_id", handler.getCheckoutSession)
	// Gaps batch (#809-close, backend B): purchase options, pending
	// orders, recharge history, billing quotes, rewards redeem/history.
	registerContractV1BillingGapsRoutes(group, handler)
}

type contractV1BillingHandler struct {
	deps ContractV1BillingDeps
}

func (h *contractV1BillingHandler) requireTenancy(c *gin.Context) bool {
	if h.deps.Tenancy == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 billing surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// billingWorkspaceContext resolves the explicit workspace context with the
// kernel visibility rule: malformed/unknown workspaces are invisible
// (404 workspace_not_found, never 403); members resolve to role+member=true.
func (h *contractV1BillingHandler) billingWorkspaceContext(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, msgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	workspace, err := h.deps.Tenancy.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, msgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Tenancy.GetWorkspaceRole(c.Request.Context(), raw, p.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, msgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	if !member {
		return workspace, "", false
	}
	return workspace, role, true
}

// billingWalletUser resolves which user wallet backs workspace reads.
//
// Transitional projection (#804): the Credits ledger stays user-scoped, so
// workspace reads project the workspace owner's wallet (personal and
// org workspaces always record an owner). When the owner is unknown the
// caller's own wallet is used. Workspace-scoped ledger wrappers (hold/
// capture/release keyed by billing account) are #805/#808 follow-ups; no
// second ledger is introduced here.
func billingWalletUser(workspace *tenancy.Workspace, callerUserID int64) int64 {
	if workspace != nil && workspace.OwnerUserID != nil && *workspace.OwnerUserID > 0 {
		return *workspace.OwnerUserID
	}
	return callerUserID
}

type v1BillingAccountDTO struct {
	WorkspaceID string `json:"workspace_id"`
	OwnerType   string `json:"owner_type"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func (h *contractV1BillingHandler) getBillingAccount(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable39)
			c.Abort()
			return
		}
		account, err := h.deps.BillingAccounts.EnsureBillingAccountForWorkspace(
			c.Request.Context(), workspace.InternalID, workspace.PublicID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable39)
			c.Abort()
			return
		}
		dto := v1BillingAccountDTO{WorkspaceID: workspace.PublicID, OwnerType: "workspace"}
		if !account.CreatedAt.IsZero() {
			dto.CreatedAt = account.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !account.UpdatedAt.IsZero() {
			dto.UpdatedAt = account.UpdatedAt.UTC().Format(time.RFC3339)
		}
		contract.RenderSuccess(c, dto)
	})
}

type v1BillingCreditsBalanceDTO struct {
	WorkspaceID        string   `json:"workspace_id"`
	AvailableCredits   int64    `json:"available_credits"`
	ReservedCredits    int64    `json:"reserved_credits"`
	Status             string   `json:"status"`
	ActiveRestrictions []string `json:"active_restrictions,omitempty"`
}

func billingToBalanceDTO(workspacePublicID string, wallet domain.WalletDetails) v1BillingCreditsBalanceDTO {
	restrictions := make([]string, 0, len(wallet.ActiveRestrictions))
	for _, restriction := range wallet.ActiveRestrictions {
		if restriction.Type != "" {
			restrictions = append(restrictions, restriction.Type)
		}
	}
	dto := v1BillingCreditsBalanceDTO{
		WorkspaceID:      workspacePublicID,
		AvailableCredits: wallet.Available,
		ReservedCredits:  wallet.Reserved,
		Status:           string(wallet.Status),
	}
	if len(restrictions) > 0 {
		dto.ActiveRestrictions = restrictions
	}
	return dto
}

// renderV1BillingRecordsError maps billing-records read outcomes onto
// the wire: invalid list input (bad user/cursor/limit facts) renders
// 400 bad_request; every other List failure stays 503
// service_unavailable (transient or store-side). Pagination-shape
// failures (malformed cursor, bad page_size) are rejected at the edge
// above with pagination_cursor_invalid/bad_request before the service
// is reached.
func renderV1BillingRecordsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billingrecords.ErrInvalidListInput),
		errors.Is(err, billingrecords.ErrInvalidPageLimit):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest32)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable39)
	}
	c.Abort()
}

// renderV1UsageError maps usage read outcomes onto the wire:
// invalid-input sentinels render 4xx (bad range reuses the 422
// validation_failed shape the edge uses for query parsing; bad user
// renders 400 bad_request); every other GetCurrent failure stays 503
// service_unavailable (transient or store-side).
func renderV1UsageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usage.ErrInvalidRange):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			msgRenderV1UsageError, contract.FieldErrors{Fields: map[string]string{"range": "invalid"}})
	case errors.Is(err, usage.ErrInvalidUserID):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest33)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable40)
	}
	c.Abort()
}

// billingRenderWalletGate maps frozen/restricted wallets to the frozen
// billing_wallet_restricted code (403). Missing wallets are invisible
// inside a visible workspace: 404 with the same code family would leak
// provisioning state, so they render service_unavailable-safe 404.
func billingRenderWalletGate(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrWalletRestricted), errors.Is(err, domain.ErrWalletFrozen):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, msgBillingWalletRestricted2)
	case errors.Is(err, domain.ErrWalletNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBillingWalletRestricted, msgBillingWalletRestricted3)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable41)
	}
	c.Abort()
}

func (h *contractV1BillingHandler) getCreditsBalance(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Wallets == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable41)
			c.Abort()
			return
		}
		wallet, err := h.deps.Wallets.GetWallet(c.Request.Context(), billingWalletUser(workspace, p.UserID()))
		if err != nil {
			billingRenderWalletGate(c, err)
			return
		}
		contract.RenderSuccess(c, billingToBalanceDTO(workspace.PublicID, wallet))
	})
}

type v1BillingLedgerCursor struct {
	BeforeSequence int64 `json:"before_sequence"`
}

type v1BillingLedgerEntryDTO struct {
	TransactionID         string `json:"transaction_id"`
	CommandType           string `json:"command_type"`
	AmountCredits         int64  `json:"amount_credits"`
	AvailableCreditsAfter int64  `json:"available_credits_after"`
	ReservedCreditsAfter  int64  `json:"reserved_credits_after"`
	CreatedAt             string `json:"created_at"`
}

func billingToLedgerEntryDTO(tx domain.LedgerTransaction) v1BillingLedgerEntryDTO {
	return v1BillingLedgerEntryDTO{
		TransactionID:         tx.TransactionID,
		CommandType:           string(tx.CommandType),
		AmountCredits:         tx.Amount,
		AvailableCreditsAfter: tx.AvailableBalanceAfter,
		ReservedCreditsAfter:  tx.ReservedBalanceAfter,
		CreatedAt:             tx.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *contractV1BillingHandler) listCreditsLedger(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Wallets == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable42)
			c.Abort()
			return
		}
		pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest31)
			c.Abort()
			return
		}
		var beforeSequence int64
		if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
			cursor, decodeErr := contract.DecodeCursor[v1BillingLedgerCursor](raw)
			if decodeErr != nil || cursor == nil || cursor.BeforeSequence < 0 {
				contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid3)
				c.Abort()
				return
			}
			beforeSequence = cursor.BeforeSequence
		}
		page, err := h.deps.Wallets.ListLedger(c.Request.Context(), domain.ListLedgerInput{
			UserID:         billingWalletUser(workspace, p.UserID()),
			BeforeSequence: beforeSequence,
			Limit:          pageSize,
		})
		if err != nil {
			if errors.Is(err, domain.ErrWalletRestricted) || errors.Is(err, domain.ErrWalletFrozen) {
				billingRenderWalletGate(c, err)
				return
			}
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable42)
			c.Abort()
			return
		}
		entries := make([]v1BillingLedgerEntryDTO, 0, len(page.Transactions))
		for _, tx := range page.Transactions {
			entries = append(entries, billingToLedgerEntryDTO(tx))
		}
		meta := contract.NewPageMeta("", false)
		if page.NextBeforeSequence > 0 {
			next, encodeErr := contract.EncodeCursor(v1BillingLedgerCursor{BeforeSequence: page.NextBeforeSequence})
			if encodeErr != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable42)
				c.Abort()
				return
			}
			meta = contract.NewPageMeta(next, true)
		}
		contract.RenderPaginated(c, entries, meta)
	})
}

type v1BillingRecordDTO struct {
	RecordID              string `json:"record_id"`
	OccurredAt            string `json:"occurred_at"`
	Kind                  string `json:"kind"`
	Title                 string `json:"title"`
	CreditDelta           int64  `json:"credit_delta"`
	AvailableCreditsAfter int64  `json:"available_credits_after"`
}

func (h *contractV1BillingHandler) listBillingRecords(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Records == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable39)
			c.Abort()
			return
		}
		pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest31)
			c.Abort()
			return
		}
		var beforeSequence int64
		if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
			cursor, decodeErr := contract.DecodeCursor[v1BillingLedgerCursor](raw)
			if decodeErr != nil || cursor == nil || cursor.BeforeSequence < 0 {
				contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid3)
				c.Abort()
				return
			}
			beforeSequence = cursor.BeforeSequence
		}
		page, err := h.deps.Records.List(c.Request.Context(), billingrecords.ListInput{
			UserID:         billingWalletUser(workspace, p.UserID()),
			BeforeSequence: beforeSequence,
			Limit:          pageSize,
		})
		if err != nil {
			renderV1BillingRecordsError(c, err)
			return
		}
		records := make([]v1BillingRecordDTO, 0, len(page.Records))
		for _, record := range page.Records {
			records = append(records, v1BillingRecordDTO{
				RecordID:              record.RecordID,
				OccurredAt:            record.OccurredAt.UTC().Format(time.RFC3339),
				Kind:                  string(record.Kind),
				Title:                 record.Title,
				CreditDelta:           record.CreditDelta,
				AvailableCreditsAfter: record.AvailableCreditsAfter,
			})
		}
		meta := contract.NewPageMeta("", false)
		if page.NextBeforeSequence > 0 {
			next, encodeErr := contract.EncodeCursor(v1BillingLedgerCursor{BeforeSequence: page.NextBeforeSequence})
			if encodeErr != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable39)
				c.Abort()
				return
			}
			meta = contract.NewPageMeta(next, true)
		}
		contract.RenderPaginated(c, records, meta)
	})
}

func (h *contractV1BillingHandler) getUsage(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Usage == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable40)
			c.Abort()
			return
		}
		usageRange, err := usage.ParseRange(strings.TrimSpace(c.Query("range")))
		if err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgRenderV1UsageError, contract.FieldErrors{Fields: map[string]string{"range": "invalid"}})
			c.Abort()
			return
		}
		snapshot, err := h.deps.Usage.GetCurrent(c.Request.Context(), billingWalletUser(workspace, p.UserID()), usageRange)
		if err != nil {
			renderV1UsageError(c, err)
			return
		}
		contract.RenderSuccess(c, snapshot)
	})
}

type v1CreateCheckoutBody struct {
	Data struct {
		IdeaQuantity        int64  `json:"idea_quantity"`
		TermsAccepted       bool   `json:"terms_accepted"`
		TermsVersion        string `json:"terms_version"`
		RefundPolicyVersion string `json:"refund_policy_version"`
	} `json:"data"`
}

type v1CheckoutSessionDTO struct {
	SessionID    string `json:"session_id"`
	Status       string `json:"status,omitempty"`
	IdeaQuantity int64  `json:"idea_quantity,omitempty"`
	CheckoutURL  string `json:"checkout_url,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Reused       bool   `json:"reused,omitempty"`
}

func (h *contractV1BillingHandler) createCheckoutSession(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Checkout == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable43)
			c.Abort()
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, msgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateCheckoutBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		quantity := body.Data.IdeaQuantity
		if quantity < paymentsdomain.MinimumPurchaseIdeas || quantity > paymentsdomain.MaximumPurchaseIdeas {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgCreateCheckoutSession, contract.FieldErrors{Fields: map[string]string{"idea_quantity": "invalid"}})
			c.Abort()
			return
		}
		termsVersion := strings.TrimSpace(body.Data.TermsVersion)
		if termsVersion == "" {
			termsVersion = paymentsdomain.CurrentTermsVersion
		}
		refundVersion := strings.TrimSpace(body.Data.RefundPolicyVersion)
		if refundVersion == "" {
			refundVersion = paymentsdomain.CurrentRefundPolicyVersion
		}
		if !body.Data.TermsAccepted || termsVersion != paymentsdomain.CurrentTermsVersion || refundVersion != paymentsdomain.CurrentRefundPolicyVersion {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgCreateCheckoutSession2, contract.FieldErrors{Fields: map[string]string{"terms_accepted": "required"}})
			c.Abort()
			return
		}
		// Server-side price snapshot: no client price input is accepted. Recharge
		// uses the fixed IdeasPerUSD PurchaseQuote inside Payments; Billing SKU
		// quotes stay reserved for task settlement (#805).
		if _, quoteErr := paymentsdomain.NewPurchaseQuote(quantity); quoteErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeBillingQuoteUnavailable, msgBillingQuoteUnavailable)
			c.Abort()
			return
		}
		ctx := c.Request.Context()
		externalUser := tenancy.FormatUserPublicID(p.PublicID())
		scope := contract.IdempotencyScope(externalUser, "billing.checkout.create")
		hash := contract.HashIdempotencyRequest("billing.checkout.create", workspace.PublicID,
			strings.TrimSpace(billingFormatInt64(quantity)), termsVersion, refundVersion)
		result, _, doErr := h.deps.Idempotency.Do(ctx, scope, p.UserID(), key, hash, func() (any, error) {
			return h.deps.Checkout.CreateWorkspaceCheckout(ctx, p.UserID(), workspace.PublicID, key, workspaceCheckoutInput{
				IdeaQuantity:        quantity,
				TermsAccepted:       true,
				TermsVersion:        termsVersion,
				RefundPolicyVersion: refundVersion,
			})
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeBillingCheckoutConflict, msgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			if errors.Is(doErr, paymentsdomain.ErrWalletNotRechargeable) {
				contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, msgBillingWalletRestricted4)
				c.Abort()
				return
			}
			if errors.Is(doErr, paymentsdomain.ErrInvalidIdeaQuantity) || errors.Is(doErr, paymentsdomain.ErrTermsAcceptanceRequired) {
				contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
					msgCreateCheckoutSession3, contract.FieldErrors{Fields: map[string]string{"idea_quantity": "invalid"}})
				c.Abort()
				return
			}
			if errors.Is(doErr, paymentsdomain.ErrCheckoutIdempotencyConflict) {
				contract.RenderError(c, http.StatusConflict, contract.CodeBillingCheckoutConflict, msgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable43)
			c.Abort()
			return
		}
		created, ok := result.(workspaceCheckoutResult)
		if !ok {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable43)
			c.Abort()
			return
		}
		dto := v1CheckoutSessionDTO{SessionID: created.SessionID, IdeaQuantity: quantity, Reused: created.Reused}
		if created.CheckoutURL != "" {
			dto.CheckoutURL = created.CheckoutURL
		}
		if !created.ExpiresAt.IsZero() {
			dto.ExpiresAt = created.ExpiresAt.UTC().Format(time.RFC3339)
		}
		contract.RenderCreated(c, dto)
	})
}

func (h *contractV1BillingHandler) getCheckoutSession(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		p := wctx.Principal
		workspace := wctx.Workspace
		if h.deps.Checkout == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable43)
			c.Abort()
			return
		}
		sessionID := strings.TrimSpace(c.Param("session_id"))
		if sessionID == "" {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBillingSessionNotFound, msgBillingSessionNotFound)
			c.Abort()
			return
		}
		status, err := h.deps.Checkout.GetWorkspaceCheckout(c.Request.Context(), p.UserID(), workspace.PublicID, sessionID)
		if err != nil {
			if errors.Is(err, errBillingSessionNotFound) {
				contract.RenderError(c, http.StatusNotFound, contract.CodeBillingSessionNotFound, msgBillingSessionNotFound)
			} else {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable43)
			}
			c.Abort()
			return
		}
		dto := v1CheckoutSessionDTO{SessionID: status.SessionID, Status: status.Status, IdeaQuantity: status.IdeaQuantity}
		if status.CheckoutURL != "" {
			dto.CheckoutURL = status.CheckoutURL
		}
		if !status.ExpiresAt.IsZero() {
			dto.ExpiresAt = status.ExpiresAt.UTC().Format(time.RFC3339)
		}
		contract.RenderSuccess(c, dto)
	})
}

var errBillingSessionNotFound = errors.New("billing: checkout session not found")

func billingFormatInt64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	pos := len(buf)
	for v > 0 {
		pos--
		buf[pos] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// PaymentsCheckoutAdapter binds workspace checkout to the existing
// internal/payments seam. ClientIdempotencyKey is namespaced by workspace so
// the durable (user_id, client_key) order intent stays workspace-bound;
// webhook settlement (POST /api/webhooks/stripe) then credits ONLY through
// the existing Payments -> credits.Commands transaction. Provider endpoint
// config risk: the webhook path is unchanged by #804 (code only); any future
// path change needs a provider dashboard update.
//
// webhooks settle via ReceiveStripeWebhook -> ConvergeCheckoutSession ->
// CreditPaidOrder (Purchase in the payment-order transaction). No Public
// handler writes the ledger directly.
type PaymentsCheckoutAdapter struct {
	creator       paymentcontracts.UserCheckoutService
	returnBaseURL string
}

func NewPaymentsCheckoutAdapter(creator paymentcontracts.UserCheckoutService, returnBaseURL string) *PaymentsCheckoutAdapter {
	return &PaymentsCheckoutAdapter{creator: creator, returnBaseURL: returnBaseURL}
}

func (a *PaymentsCheckoutAdapter) CreateWorkspaceCheckout(ctx context.Context, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error) {
	if a == nil || a.creator == nil {
		return workspaceCheckoutResult{}, paymentsdomain.ErrPurchasesUnavailable
	}
	namespaced := workspacePublicID + ":" + strings.TrimSpace(clientKey)
	result, err := a.creator.CreateCheckout(ctx, paymentsdomain.CreateCheckoutCommand{
		UserID:               userID,
		ClientIdempotencyKey: namespaced,
		IdeaQuantity:         input.IdeaQuantity,
		TermsAccepted:        input.TermsAccepted,
		TermsVersion:         input.TermsVersion,
		RefundPolicyVersion:  input.RefundPolicyVersion,
		ReturnBaseURL:        a.returnBaseURL,
	})
	if err != nil {
		return workspaceCheckoutResult{}, err
	}
	return workspaceCheckoutResult{
		SessionID:   result.PublicToken,
		CheckoutURL: result.CheckoutURL,
		ExpiresAt:   result.ExpiresAt,
		Reused:      result.Reused,
	}, nil
}

func (a *PaymentsCheckoutAdapter) GetWorkspaceCheckout(ctx context.Context, userID int64, _ string, sessionID string) (workspaceCheckoutStatus, error) {
	if a == nil || a.creator == nil {
		return workspaceCheckoutStatus{}, paymentsdomain.ErrPurchasesUnavailable
	}
	status, err := a.creator.GetPaymentOrderReturnStatus(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
			return workspaceCheckoutStatus{}, errBillingSessionNotFound
		}
		return workspaceCheckoutStatus{}, err
	}
	return workspaceCheckoutStatus{
		SessionID:    sessionID,
		Status:       status.Status,
		IdeaQuantity: status.IdeaQuantity,
		ExpiresAt:    status.UpdatedAt,
	}, nil
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tags: WorkspaceBilling), reusing components parameters
// Cursor/PageSize/WorkspaceId/IdempotencyKey/RequestId and responses
// BadRequest/Unauthorized/Forbidden/NotFound/Conflict/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/billing-account:
//	  get: {operationId: getWorkspaceBillingAccount}
//	/workspaces/{workspace_id}/credits/balance:
//	  get: {operationId: getWorkspaceCreditsBalance}
//	/workspaces/{workspace_id}/credits/ledger:
//	  get: {operationId: listWorkspaceCreditsLedger, parameters: [WorkspaceId, Cursor, PageSize]}
//	/workspaces/{workspace_id}/billing-records:
//	  get: {operationId: listWorkspaceBillingRecords, parameters: [WorkspaceId, Cursor, PageSize]}
//	/workspaces/{workspace_id}/usage:
//	  get: {operationId: getWorkspaceUsage, parameters: [WorkspaceId, {name: range, in: query, schema: {type: string, enum: [7d, 30d, 1y], default: 30d}}]}
//	/workspaces/{workspace_id}/checkout-sessions:
//	  post: {operationId: createWorkspaceCheckoutSession, parameters: [WorkspaceId, IdempotencyKey]}
//	/workspaces/{workspace_id}/checkout-sessions/{session_id}:
//	  get: {operationId: getWorkspaceCheckoutSession, parameters: [WorkspaceId, {name: session_id, in: path, required: true, schema: {type: string}}]}
//
// Schemas (all money integer minor units; snapshots immutable):
//
//	WorkspaceBillingAccount: {workspace_id: WorkspaceId, owner_type: {const: workspace}, created_at?, updated_at?}
//	WorkspaceCreditsBalance: {workspace_id, available_credits: int64>=0, reserved_credits: int64>=0, status: {enum: [active, frozen]}, active_restrictions?: string[]}
//	CreditsLedgerEntry: {transaction_id: string, command_type: string, amount_credits: int64, available_credits_after: int64, reserved_credits_after: int64, created_at: date-time}
//	BillingRecord: {record_id, occurred_at: date-time, kind: {enum: [ai_usage, usage, reward, system_grant, system_adjustment, top_up]}, title, credit_delta: int64, available_credits_after: int64}
//	WorkspaceUsage: {timezone: {const: Asia/Shanghai}, lifetime: {total_credits, peak_daily_credits, average_daily_credits, task_count, active_days}, trend: [{date, credits}]}
//	CreateWorkspaceCheckoutRequest: {data: {idea_quantity: {type: integer, minimum: 500, maximum: 500000}, terms_accepted: {const: true}, terms_version?: string, refund_policy_version?: string}} (no price fields)
//	WorkspaceCheckoutSession: {session_id: string, status?: string, idea_quantity?: int64, checkout_url?: string (uri), expires_at?: date-time, reused?: boolean}
//
// Error codes frozen: billing_wallet_restricted (403/404), billing_checkout_conflict (409),
// billing_session_not_found (404), billing_quote_unavailable (503); reused kernel codes
// workspace_not_found/workspace_forbidden/pagination_cursor_invalid/bad_request/
// validation_failed/idempotency_key_required/service_unavailable.
