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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
		return input, false
	}
	query := c.Request.URL.Query()
	allowed := map[string]struct{}{
		"order_number": {}, "user": {}, "status": {}, "anomaly_only": {}, "cursor": {}, "limit": {},
	}
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
			return input, false
		}
	}
	input.OrderNumber = query.Get("order_number")
	input.User = query.Get("user")
	input.Status = paymentsdomain.PaymentOrderStatus(query.Get("status"))
	input.Cursor = query.Get("cursor")
	if raw, ok := query["anomaly_only"]; ok {
		if raw[0] != "true" && raw[0] != "false" {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
			return input, false
		}
		input.AnomalyOnly = raw[0] == "true"
	}
	if raw, ok := query["limit"]; ok {
		limit, err := pagination.ParseLimit(raw[0])
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
			return input, false
		}
		input.Limit = limit
	}
	return input, true
}

func (d *ContractControlDeps) listControlPaymentOrders(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	input, ok := controlPaymentOrderListInput(c)
	if !ok {
		return
	}
	page, err := d.Payments.ListAdminPaymentOrders(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
			return
		}
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderCursor) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "支付订单游标无效")
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "读取支付订单失败")
		return
	}
	contract.RenderSuccess(c, page)
}

func (d *ContractControlDeps) getControlPaymentOrder(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	detail, err := d.Payments.GetAdminPaymentOrder(c.Request.Context(), strings.TrimSpace(c.Param("order_number")))
	if err != nil {
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单号无效")
			return
		}
		if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, "支付订单不存在")
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "读取支付订单失败")
		return
	}
	contract.RenderSuccess(c, detail)
}

func (d *ContractControlDeps) createControlPaymentRefund(c *gin.Context, ctx controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "退款服务暂不可用")
		return
	}
	var req struct {
		IdeaQuantity int64  `json:"idea_quantity"`
		Reason       string `json:"reason"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "退款参数错误")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = ctx.Reason
	}
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "退款参数错误")
	case errors.Is(err, paymentcontracts.ErrRefundExpired),
		errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible),
		errors.Is(err, paymentcontracts.ErrRefundAmountExceeds),
		errors.Is(err, paymentcontracts.ErrRefundProviderConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "支付订单当前不满足退款条件")
	case errors.Is(err, creditsdomain.ErrInsufficientCredits):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, "Wallet 可用 Idea 不足")
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable),
		errors.Is(err, paymentcontracts.ErrRefundUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "Stripe 服务暂不可用")
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "发起退款失败")
	}
}

func (d *ContractControlDeps) getControlStripeConfig(c *gin.Context, _ controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	summary, err := d.Payments.GetStripeConfiguration(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "读取 Stripe 设置失败")
		return
	}
	contract.RenderSuccess(c, summary)
}

func (d *ContractControlDeps) updateControlStripeConfig(c *gin.Context, ctx controlContext) {
	if d == nil || d.Payments == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	var req struct {
		APIKey                       *string `json:"api_key"`
		PaymentMethodConfigurationID string  `json:"payment_method_configuration_id"`
		ProductTaxCode               string  `json:"product_tax_code"`
		PurchasesEnabled             *bool   `json:"purchases_enabled"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil || req.PurchasesEnabled == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Stripe 设置参数错误")
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Stripe 设置参数错误")
	case errors.Is(err, paymentcontracts.ErrStripePermissionDenied):
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, "当前 Stripe API Key 权限不足")
	case errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable),
		errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, "当前 Stripe 配置不可用")
	case errors.Is(err, paymentcontracts.ErrStripeConfigurationChanged):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "Stripe 配置已被其他操作更新，请重试")
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "Stripe 服务暂不可用")
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "保存 Stripe 设置失败")
	}
}
