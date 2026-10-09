package httpapi

// Control payments family (issue #808): payment order reads, payment
// refunds, and Stripe configuration behind control:payments.
//
// Reads and refunds delegate to the payments AdminService seam; refunds
// carry the header Idempotency-Key plus operator reason into the
// CreatePaymentRefundCommand. Legacy /api/admin/payments/* routes are
// untouched.

import (
	"errors"
	"net/http"
	"strings"

	creditsdomain "ideaboard/api/internal/credits/domain"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/pagination"

	"github.com/gin-gonic/gin"
)

func controlPaymentOrderListInput(c *gin.Context) (paymentcontracts.AdminPaymentOrderListInput, bool) {
	input := paymentcontracts.AdminPaymentOrderListInput{}
	if c == nil || c.Request == nil || c.Request.URL == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest20)
		return input, false
	}
	query := c.Request.URL.Query()
	allowed := map[string]struct{}{
		"order_number": {}, "user": {}, "status": {}, "anomaly_only": {}, "cursor": {}, "limit": {},
	}
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest20)
			return input, false
		}
	}
	input.OrderNumber = query.Get("order_number")
	input.User = query.Get("user")
	input.Status = paymentsdomain.PaymentOrderStatus(query.Get("status"))
	input.Cursor = query.Get("cursor")
	if raw, ok := query["anomaly_only"]; ok {
		if raw[0] != "true" && raw[0] != "false" {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest20)
			return input, false
		}
		input.AnomalyOnly = raw[0] == "true"
	}
	if raw, ok := query["limit"]; ok {
		limit, err := pagination.ParseLimit(raw[0])
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest20)
			return input, false
		}
		input.Limit = limit
	}
	return input, true
}

func (d *ContractControlDeps) listControlPaymentOrders(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable13)
		return
	}
	input, ok := controlPaymentOrderListInput(c)
	if !ok {
		return
	}
	page, err := d.Payments.ListAdminPaymentOrders(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest20)
			return
		}
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderCursor) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid2)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable14)
		return
	}
	contract.RenderSuccess(c, page)
}

func (d *ContractControlDeps) getControlPaymentOrder(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable13)
		return
	}
	detail, err := d.Payments.GetAdminPaymentOrder(c.Request.Context(), strings.TrimSpace(c.Param("order_number")))
	if err != nil {
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest21)
			return
		}
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound7)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable14)
		return
	}
	contract.RenderSuccess(c, detail)
}

func (d *ContractControlDeps) createControlPaymentRefund(c *gin.Context, ctx controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable15)
		return
	}
	var req struct {
		IdeaQuantity int64  `json:"idea_quantity"`
		Reason       string `json:"reason"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest22)
		return
	}
	reason := controlReasonOr(ctx, req.Reason)
	refund, err := d.Payments.CreatePaymentRefund(c.Request.Context(), paymentcontracts.CreatePaymentRefundCommand{
		ActorID: ctx.Operator.UserID(), OrderNumber: strings.TrimSpace(c.Param("order_number")),
		IdeaQuantity: req.IdeaQuantity, IdempotencyKey: ctx.IdemKey, Reason: reason,
	})
	if err != nil {
		writeControlPaymentRefundError(c, err)
		return
	}
	contract.RenderSuccess(c, refund)
}

func writeControlPaymentRefundError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentcontracts.ErrRefundAmountInvalid),
		errors.Is(err, paymentcontracts.ErrRefundIdempotency),
		errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest22)
	case errors.Is(err, paymentcontracts.ErrRefundExpired),
		errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible),
		errors.Is(err, paymentcontracts.ErrRefundAmountExceeds),
		errors.Is(err, paymentcontracts.ErrRefundProviderConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, msgConflict2)
	case errors.Is(err, creditsdomain.ErrInsufficientCredits):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, msgValidationFailed4)
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable),
		errors.Is(err, paymentcontracts.ErrRefundUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable16)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable17)
	}
}

// parseControlPaymentProvider resolves a {provider} path param against the
// implemented payment-provider allowlist. Only stripe is implemented;
// unknown or malformed slugs fail closed with 404 (invisible, never 403).
func parseControlPaymentProvider(c *gin.Context) bool {
	provider, ok := parseControlProviderSlug(c)
	if !ok {
		return false
	}
	if provider != "stripe" {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound6)
		return false
	}
	return true
}

func (d *ContractControlDeps) getControlStripeConfig(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable18)
		return
	}
	if !parseControlPaymentProvider(c) {
		return
	}
	summary, err := d.Payments.GetStripeConfiguration(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable19)
		return
	}
	contract.RenderSuccess(c, summary)
}

func (d *ContractControlDeps) updateControlStripeConfig(c *gin.Context, ctx controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable18)
		return
	}
	if !parseControlPaymentProvider(c) {
		return
	}
	var req struct {
		APIKey                       *string `json:"api_key"`
		PaymentMethodConfigurationID string  `json:"payment_method_configuration_id"`
		ProductTaxCode               string  `json:"product_tax_code"`
		PurchasesEnabled             *bool   `json:"purchases_enabled"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil || req.PurchasesEnabled == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest23)
		return
	}
	apiKey := ""
	if req.APIKey != nil {
		apiKey = *req.APIKey
	}
	summary, err := d.Payments.ConfigureStripe(c.Request.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: ctx.Operator.UserID(), APIKey: apiKey, ProductTaxCode: req.ProductTaxCode,
		PaymentMethodConfigurationID: req.PaymentMethodConfigurationID,
		PurchasesEnabled:             *req.PurchasesEnabled,
		WebhookURL:                   d.PaymentWebhookURL,
	})
	if err != nil {
		writeControlStripeConfigError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}

func writeControlStripeConfigError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentcontracts.ErrStripeAPIKeyRequired),
		errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey),
		errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest23)
	case errors.Is(err, paymentcontracts.ErrStripePermissionDenied):
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, msgForbidden2)
	case errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable),
		errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, msgValidationFailed5)
	case errors.Is(err, paymentcontracts.ErrStripeConfigurationChanged):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, msgConflict3)
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable16)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable20)
	}
}
