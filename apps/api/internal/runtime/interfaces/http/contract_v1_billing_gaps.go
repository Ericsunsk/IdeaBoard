package httpapi

// Workspace billing/payments-query/rewards-query gaps (issue #809-close,
// backend batch B).
//
// Additive only: this file extends the #804 workspace billing surface with
// the v1 read/quote/history operations that had no v1 equivalent (frontend
// account flows fail closed with guidance without them). Legacy
// /api/user and /api/admin routes stay deleted; openapi.*.yaml is NOT
// edited here (exact spec deltas for the merger live at the bottom of
// this file).
//
// Added surface (ContractV1GroupPath + PublicContractChain, same implicit
// per-workspace authZ as contract_v1_billing.go; the account is implicit
// per workspace, no billing-account ID in URLs):
//
//	GET  /api/v1/workspaces/{workspace_id}/purchase-options
//	GET  /api/v1/workspaces/{workspace_id}/pending-orders
//	POST /api/v1/workspaces/{workspace_id}/pending-orders/{order_id}/continue   (header Idempotency-Key, empty body)
//	POST /api/v1/workspaces/{workspace_id}/pending-orders/{order_id}/abandon    (header Idempotency-Key, empty body)
//	GET  /api/v1/workspaces/{workspace_id}/recharges (cursor pagination)
//	POST /api/v1/workspaces/{workspace_id}/billing-quotes (server-side immutable snapshot; covers ai-search:vision)
//	POST /api/v1/workspaces/{workspace_id}/rewards/redemptions (header Idempotency-Key)
//	GET  /api/v1/workspaces/{workspace_id}/rewards/redemptions (cursor pagination)
//
// Ownership (inviolable):
//   - ONLY internal/credits writes the ledger, via credits.Commands. This
//     file performs NO ledger writes: quotes mint immutable snapshots,
//     checkout/pending/recharge compose the existing internal/payments
//     seam, and redeem composes the existing internal/rewards seam (which
//     Grants through credits.Commands inside its own transaction).
//   - Billing ONLY mints immutable PriceSnapshots/Quotes (server-side, no
//     client price input). Unpriced/unregistered SKUs converge to
//     billing_pricing_unavailable (422, deterministic, no retry); only
//     transient quote failures converge to billing_quote_unavailable
//     (503, no SKU oracle).
//   - Payments ONLY owns order intents + provider convergence + webhook
//     inbox (existing seam, reused via narrow interfaces below). No new
//     webhook route here; POST /api/webhooks/stripe is reused.
//   - Rewards ONLY redeems through ports.UserService (durable per-user
//     replay inside) wrapped in the kernel IdempotencyStore for
//     header-key replay; history is a read model.
// Money: integer minor units (credits) throughout; snapshots immutable.
// Nullable-vs-omitted: optional URL/amount fields omitted when empty;
// next_cursor is "" with has_more=false when exhausted (kernel PageMeta).
//
// AuthZ: every route resolves the explicit workspace context with the
// kernel visibility rule (malformed/unknown/non-member workspaces are
// invisible: 404 workspace_not_found, never 403; anon is 401). Any
// workspace member may read/act; payments + rewards facts stay
// caller-scoped (principal UserID) with the workspace as the authZ scope
// only, matching the checkout-session precedent. Wallet-balance reads
// keep the owner projection in contract_v1_billing.go; order/recharge/
// redemption facts do NOT (they are per-user top-up/redemption facts).
//
// Cursor dialects: recharges reuse the legacy opaque NextCursor directly
// in meta (service-owned); redemption history maps BeforeID through the
// kernel opaque cursor (v1BillingRedemptionCursor). List endpoints expose
// only cursor + page_size (Baseline 5.5). Pending orders are bounded
// (max 3 unpaid, service-enforced) and intentionally unpaginated.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Frozen v1-local reward error codes (wire-stable; document for the merger
// to promote into internal/platform/contract + openapi.public.yaml).
// Explicit statuses are rendered at each site (never via StatusOf), so the
// kernel registry needs no edit for this batch.
//
//   - reward_code_unavailable (422): unknown, expired, exhausted, or
//     disabled code (no oracle between shapes).
//   - reward_wallet_locked (409): frozen wallet cannot redeem.
//   - reward_redeem_conflict (409): Idempotency-Key reused with a different
//     redeem request (hash mismatch); replay otherwise.
const (
	CodeRewardCodeUnavailable contract.Code = "reward_code_unavailable"
	CodeRewardWalletLocked    contract.Code = "reward_wallet_locked"
	CodeRewardRedeemConflict  contract.Code = "reward_redeem_conflict"
)

// v1PurchaseOptionsService is the caller-owned read over the Payments
// purchase-eligibility projection (static quantities + terms versions +
// wallet gate). No ledger writes.
type v1PurchaseOptionsService interface {
	GetPurchaseOptions(ctx context.Context, userID int64) (paymentcontracts.PurchaseOptions, error)
}

// v1PendingOrderService composes the Payments order-intent seam:
// list/continue/abandon converge provider state inside Payments; the
// handler adds workspace authZ + header-key idempotency only.
type v1PendingOrderService interface {
	ListPendingOrders(ctx context.Context, userID int64) (paymentcontracts.PendingPaymentOrderList, error)
	ContinuePendingOrder(ctx context.Context, userID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error)
	AbandonPendingOrder(ctx context.Context, userID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error)
}

// v1RechargeHistoryService composes the Payments settled-facts read model
// (credited + refunded + user-abandoned, latest 12 months). Read-only.
type v1RechargeHistoryService interface {
	ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (paymentcontracts.RechargeHistoryPage, error)
}

// v1RewardsUserService composes the Rewards user seam: Redeem settles
// through credits.Commands inside the Rewards transaction (durable
// per-user replay); ListRedemptions is a read model.
type v1RewardsUserService interface {
	Redeem(ctx context.Context, cmd ports.RedeemCommand) (ports.RedeemResult, error)
	ListRedemptions(ctx context.Context, input ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error)
}

// registerContractV1BillingGapsRoutes mounts the gaps surface on the shared
// workspace billing group. Called from
// RegisterContractV1BillingRoutesOnEngine; nil seams fail closed per route.
func registerContractV1BillingGapsRoutes(group *gin.RouterGroup, handler *contractV1BillingHandler) {
	group.GET("/workspaces/:workspace_id/purchase-options", handler.getPurchaseOptions)
	group.GET("/workspaces/:workspace_id/pending-orders", handler.listPendingOrders)
	group.POST("/workspaces/:workspace_id/pending-orders/:order_id/continue", handler.continuePendingOrder)
	group.POST("/workspaces/:workspace_id/pending-orders/:order_id/abandon", handler.abandonPendingOrder)
	group.GET("/workspaces/:workspace_id/recharges", handler.listRecharges)
	group.POST("/workspaces/:workspace_id/billing-quotes", handler.createBillingQuote)
	group.POST("/workspaces/:workspace_id/rewards/redemptions", handler.redeemReward)
	group.GET("/workspaces/:workspace_id/rewards/redemptions", handler.listRewardRedemptions)
}

func (h *contractV1BillingHandler) getPurchaseOptions(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.PurchaseOptions == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	options, err := h.deps.PurchaseOptions.GetPurchaseOptions(c.Request.Context(), p.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	if options.QuickIdeaQuantities == nil {
		options.QuickIdeaQuantities = []int64{}
	}
	contract.RenderSuccess(c, options)
}

type v1PendingOrderDTO struct {
	PublicToken  string `json:"public_token"`
	IdeaQuantity int64  `json:"idea_quantity"`
	CheckoutURL  string `json:"checkout_url,omitempty"`
	Status       string `json:"status"`
	ExpiresAt    string `json:"expires_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toV1PendingOrderDTO(order paymentcontracts.PendingPaymentOrder) v1PendingOrderDTO {
	return v1PendingOrderDTO{
		PublicToken:  order.PublicToken,
		IdeaQuantity: order.IdeaQuantity,
		CheckoutURL:  order.CheckoutURL,
		Status:       order.Status,
		ExpiresAt:    order.ExpiresAt.UTC().Format(time.RFC3339),
		UpdatedAt:    order.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *contractV1BillingHandler) listPendingOrders(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.PendingOrders == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	orders, err := h.deps.PendingOrders.ListPendingOrders(c.Request.Context(), p.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	items := make([]v1PendingOrderDTO, 0, len(orders.Items))
	for _, order := range orders.Items {
		items = append(items, toV1PendingOrderDTO(order))
	}
	contract.RenderSuccess(c, map[string]any{"items": items})
}

// v1PendingOrderToken validates the order path param with the legacy
// anti-oracle rule: malformed tokens converge to 404
// billing_session_not_found, never 400, so the token is never an
// ownership oracle.
func v1PendingOrderToken(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.Param("order_id"))
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed.String() != strings.ToLower(raw) {
		contract.RenderError(c, http.StatusNotFound, CodeBillingSessionNotFound, "充值订单不存在")
		c.Abort()
		return "", false
	}
	return raw, true
}

// requireEmptyV1BillingActionBody rejects bodies on the empty-body action
// routes (continue/abandon): the routes have no request schema, so a body
// is a caller bug, never a second command API.
func requireEmptyV1BillingActionBody(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (64<<10)+1))
	if err != nil || int64(len(body)) > 64<<10 || strings.TrimSpace(string(body)) != "" {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return false
	}
	return true
}

// renderPendingOrderError maps the Payments order-intent outcomes onto the
// frozen v1 codes. Terminal convergence returns the order (no error), so
// only fact-access, wallet-gate, and provider outages map here.
func renderPendingOrderError(c *gin.Context, err error, action string) {
	switch {
	case errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound):
		contract.RenderError(c, http.StatusNotFound, CodeBillingSessionNotFound, "充值订单不存在")
	case errors.Is(err, paymentsdomain.ErrWalletNotRechargeable):
		contract.RenderError(c, http.StatusForbidden, CodeBillingWalletRestricted, "当前工作区钱包受限，暂不可"+action)
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable),
		errors.Is(err, paymentsdomain.ErrPurchasesUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付服务暂不可用")
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
	}
	c.Abort()
}

func (h *contractV1BillingHandler) continuePendingOrder(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.PendingOrders == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	orderID, ok := v1PendingOrderToken(c)
	if !ok {
		return
	}
	if !requireEmptyV1BillingActionBody(c) {
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(p.PublicID())
	scope := contract.IdempotencyScope(externalUser, "billing.pending.continue")
	hash := contract.HashIdempotencyRequest("billing.pending.continue", workspace.PublicID, orderID)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, p.UserID(), key, hash, func() (any, error) {
		return h.deps.PendingOrders.ContinuePendingOrder(ctx, p.UserID(), orderID)
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, CodeBillingCheckoutConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderPendingOrderError(c, doErr, "继续充值")
		return
	}
	order, ok := result.(paymentcontracts.PendingPaymentOrder)
	if !ok {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderSuccess(c, toV1PendingOrderDTO(order))
}

func (h *contractV1BillingHandler) abandonPendingOrder(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.PendingOrders == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	orderID, ok := v1PendingOrderToken(c)
	if !ok {
		return
	}
	if !requireEmptyV1BillingActionBody(c) {
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(p.PublicID())
	scope := contract.IdempotencyScope(externalUser, "billing.pending.abandon")
	hash := contract.HashIdempotencyRequest("billing.pending.abandon", workspace.PublicID, orderID)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, p.UserID(), key, hash, func() (any, error) {
		return h.deps.PendingOrders.AbandonPendingOrder(ctx, p.UserID(), orderID)
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, CodeBillingCheckoutConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderPendingOrderError(c, doErr, "放弃充值")
		return
	}
	order, ok := result.(paymentcontracts.PendingPaymentOrder)
	if !ok {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderSuccess(c, toV1PendingOrderDTO(order))
}

type v1RechargeHistoryItemDTO struct {
	OccurredAt           string `json:"occurred_at"`
	IdeaQuantity         int64  `json:"idea_quantity"`
	PaidAmountMinor      int64  `json:"paid_amount_minor,omitempty"`
	PaidCurrency         string `json:"paid_currency,omitempty"`
	Status               string `json:"status"`
	RefundedIdeaQuantity int64  `json:"refunded_idea_quantity,omitempty"`
	RefundedAmountMinor  int64  `json:"refunded_amount_minor,omitempty"`
	ReceiptURL           string `json:"receipt_url,omitempty"`
}

func (h *contractV1BillingHandler) listRecharges(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.Recharges == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		c.Abort()
		return
	}
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return
	}
	cursor := strings.TrimSpace(c.Query(contract.CursorParam))
	page, err := h.deps.Recharges.ListRecharges(c.Request.Context(), p.UserID(), cursor, pageSize)
	if err != nil {
		switch {
		case errors.Is(err, paymentcontracts.ErrRechargeHistoryCursor):
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
		case errors.Is(err, paymentcontracts.ErrRechargeHistoryLimit):
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		default:
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "充值服务暂不可用")
		}
		c.Abort()
		return
	}
	items := make([]v1RechargeHistoryItemDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, v1RechargeHistoryItemDTO{
			OccurredAt:           item.OccurredAt.UTC().Format(time.RFC3339),
			IdeaQuantity:         item.IdeaQuantity,
			PaidAmountMinor:      item.PaidAmountMinor,
			PaidCurrency:         item.PaidCurrency,
			Status:               item.Status,
			RefundedIdeaQuantity: item.RefundedIdeaQuantity,
			RefundedAmountMinor:  item.RefundedAmountMinor,
			ReceiptURL:           item.ReceiptURL,
		})
	}
	meta := contract.NewPageMeta("", false)
	if strings.TrimSpace(page.NextCursor) != "" {
		meta = contract.NewPageMeta(page.NextCursor, true)
	}
	contract.RenderPaginated(c, items, meta)
}

type v1CreateBillingQuoteBody struct {
	Data struct {
		SKU              string `json:"sku"`
		OutputResolution string `json:"output_resolution"`
		Quantity         *int64 `json:"quantity"`
	} `json:"data"`
}

type v1BillingQuoteDTO struct {
	SKU              string `json:"sku"`
	DisplayName      string `json:"display_name"`
	PriceVersionID   int64  `json:"price_version_id"`
	PriceVersion     int64  `json:"price_version"`
	OutputResolution string `json:"output_resolution"`
	UnitCredits      int64  `json:"unit_credits"`
	Quantity         int64  `json:"quantity"`
	TotalCredits     int64  `json:"total_credits"`
	QuotedAt         string `json:"quoted_at"`
}

// createBillingQuote mints a server-side immutable price snapshot with no
// persistence (preflight, like task-quotes): the quote path is the only
// place a price is ever shown, and settlement re-quotes authoritatively.
// No Idempotency-Key: pure read, safe to retry.
func (h *contractV1BillingHandler) createBillingQuote(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.Quoter == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, CodeBillingQuoteUnavailable, "价格服务暂不可用")
		c.Abort()
		return
	}
	var body v1CreateBillingQuoteBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	sku := strings.TrimSpace(body.Data.SKU)
	if sku == "" {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"计费功能标识非法", contract.FieldErrors{Fields: map[string]string{"sku": "invalid"}})
		c.Abort()
		return
	}
	resolutionRaw := strings.TrimSpace(body.Data.OutputResolution)
	if resolutionRaw == "" {
		resolutionRaw = string(billing.OutputResolutionDefault)
	}
	resolution, err := billing.NormalizeOutputResolution(resolutionRaw)
	if err != nil {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"输出规格非法", contract.FieldErrors{Fields: map[string]string{"output_resolution": "invalid"}})
		c.Abort()
		return
	}
	quantity := int64(1)
	if body.Data.Quantity != nil {
		quantity = *body.Data.Quantity
	}
	if quantity <= 0 {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"报价数量非法", contract.FieldErrors{Fields: map[string]string{"quantity": "invalid"}})
		c.Abort()
		return
	}
	quote, err := h.deps.Quoter.QuoteAtResolution(c.Request.Context(), sku, resolution, quantity)
	if err != nil {
		if renderBillingPricingUnavailable(c, err) {
			c.Abort()
			return
		}
		switch {
		case errors.Is(err, billing.ErrInvalidSKU),
			errors.Is(err, billing.ErrInvalidCatalogEntry),
			errors.Is(err, billing.ErrInvalidOutputResolution):
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				"计费功能标识非法", contract.FieldErrors{Fields: map[string]string{"sku": "invalid"}})
		case errors.Is(err, billing.ErrInvalidQuantity):
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				"报价数量非法", contract.FieldErrors{Fields: map[string]string{"quantity": "invalid"}})
		case errors.Is(err, billing.ErrQuoteTotalExceeded):
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				"报价超过允许上限", contract.FieldErrors{Fields: map[string]string{"quantity": "invalid"}})
		case errors.Is(err, billing.ErrServiceUnavailable):
			contract.RenderError(c, http.StatusServiceUnavailable, CodeBillingQuoteUnavailable, "价格服务暂不可用")
		default:
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "计费服务暂不可用")
		}
		c.Abort()
		return
	}
	contract.RenderSuccess(c, v1BillingQuoteDTO{
		SKU: string(quote.SKU), DisplayName: quote.DisplayName,
		PriceVersionID: quote.PriceVersionID, PriceVersion: quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution), UnitCredits: quote.UnitCredits,
		Quantity: quote.Quantity, TotalCredits: quote.TotalCredits,
		QuotedAt: quote.QuotedAt.UTC().Format(time.RFC3339),
	})
}

type v1RedeemRewardBody struct {
	Data struct {
		Code string `json:"code"`
	} `json:"data"`
}

type v1RewardRedemptionDTO struct {
	ID                    int64  `json:"id"`
	RewardCodeID          int64  `json:"reward_code_id"`
	UserPublicID          string `json:"user_public_id"`
	DisplayName           string `json:"display_name"`
	CreditAmount          int64  `json:"credit_amount"`
	CreditTransactionID   string `json:"credit_transaction_id"`
	AvailableBalanceAfter int64  `json:"available_balance_after"`
	RedeemedAt            string `json:"redeemed_at"`
}

type v1RewardRedemptionHistoryItemDTO struct {
	ID                    int64  `json:"id"`
	RewardCodeDisplayName string `json:"reward_code_display_name"`
	CreditedAmount        int64  `json:"credited_amount"`
	MaskedCodeHint        string `json:"masked_code_hint"`
	RedeemedAt            string `json:"redeemed_at"`
}

type v1RedeemRewardResultDTO struct {
	Redemption       v1RewardRedemptionDTO            `json:"redemption"`
	HistoryItem      v1RewardRedemptionHistoryItemDTO `json:"history_item"`
	CreditedAmount   int64                            `json:"credited_amount"`
	AvailableBalance int64                            `json:"available_balance"`
	Replayed         bool                             `json:"replayed,omitempty"`
}

func toV1RewardHistoryItemDTO(item ports.RedemptionHistoryItem) v1RewardRedemptionHistoryItemDTO {
	return v1RewardRedemptionHistoryItemDTO{
		ID: item.ID, RewardCodeDisplayName: item.RewardCodeDisplayName,
		CreditedAmount: item.CreditedAmount, MaskedCodeHint: item.MaskedCodeHint,
		RedeemedAt: item.RedeemedAt.UTC().Format(time.RFC3339),
	}
}

// redeemReward redeems one reward code into the caller's wallet. The
// Rewards service owns the Grant (credits.Commands inside its own
// transaction) with durable per-user replay; the kernel IdempotencyStore
// adds header-key replay for transport retries. Code plaintext never
// leaves the request hash.
func (h *contractV1BillingHandler) redeemReward(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.Rewards == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
		c.Abort()
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	var body v1RedeemRewardBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	code := strings.TrimSpace(body.Data.Code)
	if code == "" {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"兑换码不能为空", contract.FieldErrors{Fields: map[string]string{"code": "required"}})
		c.Abort()
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(p.PublicID())
	scope := contract.IdempotencyScope(externalUser, "rewards.redeem")
	hash := contract.HashIdempotencyRequest("rewards.redeem", workspace.PublicID, code)
	result, replayed, doErr := h.deps.Idempotency.Do(ctx, scope, p.UserID(), key, hash, func() (any, error) {
		return h.deps.Rewards.Redeem(ctx, ports.RedeemCommand{UserID: p.UserID(), Code: code})
	})
	if doErr != nil {
		switch {
		case contract.IsIdempotencyConflict(doErr):
			contract.RenderError(c, http.StatusConflict, CodeRewardRedeemConflict, "幂等请求与原请求不一致")
		case errors.Is(doErr, rewardsdomain.ErrInvalidCode),
			errors.Is(doErr, rewardsdomain.ErrCodeUnavailable):
			contract.RenderError(c, http.StatusUnprocessableEntity, CodeRewardCodeUnavailable, "兑换码无效或已失效")
		case errors.Is(doErr, rewardsdomain.ErrWalletFrozen):
			contract.RenderError(c, http.StatusConflict, CodeRewardWalletLocked, "当前账户无法兑换奖励")
		default:
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
		}
		c.Abort()
		return
	}
	redeemed, ok := result.(ports.RedeemResult)
	if !ok {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
		c.Abort()
		return
	}
	redeemed.Redemption.UserPublicID = p.PublicID()
	redeemed.Redemption.DisplayName = p.DisplayName()
	redemption := redeemed.Redemption
	dto := v1RedeemRewardResultDTO{
		Redemption: v1RewardRedemptionDTO{
			ID: redemption.ID, RewardCodeID: redemption.RewardCodeID,
			UserPublicID: redemption.UserPublicID.String(), DisplayName: redemption.DisplayName,
			CreditAmount: redemption.CreditAmount, CreditTransactionID: redemption.CreditTransactionID,
			AvailableBalanceAfter: redemption.AvailableBalanceAfter,
			RedeemedAt:            redemption.RedeemedAt.UTC().Format(time.RFC3339),
		},
		HistoryItem:      toV1RewardHistoryItemDTO(redeemed.HistoryItem),
		CreditedAmount:   redeemed.CreditedAmount,
		AvailableBalance: redeemed.AvailableBalance,
		Replayed:         replayed || redeemed.Replayed,
	}
	contract.RenderCreated(c, dto)
}

type v1RewardRedemptionCursor struct {
	BeforeID int64 `json:"before_id"`
}

func (h *contractV1BillingHandler) listRewardRedemptions(c *gin.Context) {
	p, ok := v1RequestPrincipal(c)
	if !ok || !h.requireTenancy(c) {
		return
	}
	workspace, _, visible := h.billingWorkspaceContext(c, p)
	if workspace == nil {
		return
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return
	}
	if h.deps.Rewards == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
		c.Abort()
		return
	}
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return
	}
	var beforeID int64
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1RewardRedemptionCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.BeforeID < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return
		}
		beforeID = cursor.BeforeID
	}
	page, err := h.deps.Rewards.ListRedemptions(c.Request.Context(), ports.ListRedemptionsInput{
		UserID: p.UserID(), BeforeID: beforeID, Limit: pageSize,
	})
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
		c.Abort()
		return
	}
	items := make([]v1RewardRedemptionHistoryItemDTO, 0, len(page.Redemptions))
	for _, item := range page.Redemptions {
		items = append(items, toV1RewardHistoryItemDTO(item))
	}
	meta := contract.NewPageMeta("", false)
	if page.NextBeforeID > 0 {
		next, encodeErr := contract.EncodeCursor(v1RewardRedemptionCursor{BeforeID: page.NextBeforeID})
		if encodeErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "奖励服务暂不可用")
			c.Abort()
			return
		}
		meta = contract.NewPageMeta(next, true)
	}
	contract.RenderPaginated(c, items, meta)
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tags: WorkspaceBilling for payments/billing, WorkspaceRewards
// for rewards), reusing components parameters Cursor/PageSize/WorkspaceId/
// IdempotencyKey/RequestId and responses BadRequest/Unauthorized/Forbidden/
// NotFound/Conflict/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/purchase-options:
//	  get: {operationId: getWorkspacePurchaseOptions, parameters: [WorkspaceId]}
//	/workspaces/{workspace_id}/pending-orders:
//	  get: {operationId: listWorkspacePendingOrders, parameters: [WorkspaceId]}
//	/workspaces/{workspace_id}/pending-orders/{order_id}/continue:
//	  post: {operationId: continueWorkspacePendingOrder, parameters: [WorkspaceId, {name: order_id, in: path, required: true, schema: {type: string, format: uuid}}, IdempotencyKey]}
//	/workspaces/{workspace_id}/pending-orders/{order_id}/abandon:
//	  post: {operationId: abandonWorkspacePendingOrder, parameters: [WorkspaceId, {name: order_id, in: path, required: true, schema: {type: string, format: uuid}}, IdempotencyKey]}
//	/workspaces/{workspace_id}/recharges:
//	  get: {operationId: listWorkspaceRecharges, parameters: [WorkspaceId, Cursor, PageSize]}
//	/workspaces/{workspace_id}/billing-quotes:
//	  post: {operationId: createWorkspaceBillingQuote, parameters: [WorkspaceId]}
//	/workspaces/{workspace_id}/rewards/redemptions:
//	  post: {operationId: redeemWorkspaceReward, parameters: [WorkspaceId, IdempotencyKey]}
//	  get: {operationId: listWorkspaceRewardRedemptions, parameters: [WorkspaceId, Cursor, PageSize]}
//
// Schemas (all money integer minor units; snapshots immutable):
//
//	WorkspacePurchaseOptions: {purchases_available: boolean, unavailable_reason?: string, restriction_type?: string, quick_idea_quantities: int64[], min_idea_quantity: int64, max_idea_quantity: int64, ideas_per_usd: int64, terms: {version, url}, refund_policy: {version, url}}
//	WorkspacePendingOrderList: {items: WorkspacePendingOrder[]}
//	WorkspacePendingOrder: {public_token: {format: uuid}, idea_quantity: int64, checkout_url?: uri, status: {enum: [checkout_open, async_payment_pending, paid_pending_credit, credited, cancelled, expired, payment_failed]}, expires_at: date-time, updated_at: date-time}
//	WorkspaceRechargeHistoryItem: {occurred_at: date-time, idea_quantity: int64, paid_amount_minor?: int64, paid_currency?: string, status: {enum: [credited, refunded, cancelled]}, refunded_idea_quantity?: int64, refunded_amount_minor?: int64, receipt_url?: uri}
//	CreateWorkspaceBillingQuoteRequest: {data: {sku: string (pattern ^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$), output_resolution?: {enum: [default, 1k, 2k, 4k], default: default}, quantity?: {type: integer, minimum: 1, default: 1}}} (no price fields; unpriced/unregistered SKUs converge to billing_pricing_unavailable 422 deterministic, transient quote failures to billing_quote_unavailable 503, no oracle)
//	WorkspaceBillingQuote: {sku, display_name, price_version_id: int64, price_version: int64, output_resolution, unit_credits: int64>=0, quantity: int64>=1, total_credits: int64>=0, quoted_at: date-time}
//	RedeemWorkspaceRewardRequest: {data: {code: {type: string, minLength: 1}}}
//	WorkspaceRewardRedemption: {id: int64, reward_code_id: int64, user_public_id: {format: uuid}, display_name, credit_amount: int64, credit_transaction_id, available_balance_after: int64, redeemed_at: date-time}
//	WorkspaceRewardRedemptionHistoryItem: {id: int64, reward_code_display_name, credited_amount: int64, masked_code_hint, redeemed_at: date-time}
//	RedeemWorkspaceRewardResult: {redemption: WorkspaceRewardRedemption, history_item: WorkspaceRewardRedemptionHistoryItem, credited_amount: int64, available_balance: int64, replayed?: boolean}
//
// Error codes frozen: reward_code_unavailable (422), reward_wallet_locked
// (409), reward_redeem_conflict (409); reused kernel + #804 codes:
// workspace_not_found (404 hidden)/workspace_forbidden (never for member
// reads; kept for forward-compat)/pagination_cursor_invalid (400)/
// bad_request (400)/validation_failed (422)/idempotency_key_required
// (400)/service_unavailable (503) plus billing_session_not_found (404
// unknown/malformed order token), billing_checkout_conflict (409
// continue/abandon key reuse), billing_wallet_restricted (403 continue
// wallet gate), billing_quote_unavailable (503 transient quote failure),
// billing_pricing_unavailable (422 unpriced/unregistered SKU).
//
// Frontend migration notes (account + search surfaces):
//   - getPurchaseOptions/listPendingOrders/continue/abandon/listRecharges
//     map 1:1 onto the existing payments resource view types; the v1
//     session_id/order public_token is the same value as the legacy
//     public_token (same settlement source).
//   - recharges/redemptions pages arrive as the contract {data, meta}
//     envelope: meta.next_cursor/has_more replaces the legacy
//     next_cursor/next_before_id fields.
//   - fetchVisionSearchPrice posts {data:{sku:"ai-search:vision"}} to
//     billing-quotes (output_resolution defaults to default, quantity to
//     1) and reads total_credits; unpriced/unregistered SKUs stay 422
//     billing_pricing_unavailable, transient failures 503.
//   - redeemRewardCode posts {data:{code}} with Idempotency-Key and reads
//     the RedeemWorkspaceRewardResult shape 1:1.
//
// Residuals for Control (NOT built here, flagged for the merger/sibling C):
//   - Reward-code administration (create/update/enable/delete/search,
//     admin redemption reads) stays Control-plane owned; no admin route
//     is added here.
//   - Redeem rate limiting (legacy per-user throttle) is not re-added on
//     this surface; if brute-force code guessing needs an edge throttle,
//     it belongs in Control policy or the gateway, not in v1 reads.
//   - Refund/dispute administration and Stripe configuration stay Control
//     owned (existing control payments routes); recharges expose receipts
//     read-only.
