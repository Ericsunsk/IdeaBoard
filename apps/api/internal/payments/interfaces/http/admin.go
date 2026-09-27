package http

import (
	"errors"
	"log/slog"
	"net/http"

	"ideaboard/api/internal/credits/domain"

	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/pagination"

	"github.com/gin-gonic/gin"
)

func newAdminPaymentRefundResult(refund paymentsdomain.PaymentRefund) paymentcontracts.AdminPaymentRefundResult {
	return paymentcontracts.AdminPaymentRefundResult{
		PublicOrderNumber: refund.PublicOrderNumber,
		IdeaQuantity:      refund.IdeaQuantity,
		AmountMinor:       refund.AmountMinor,
		Currency:          refund.Currency,
		Status:            refund.Status,
		CreatedAt:         refund.CreatedAt,
		UpdatedAt:         refund.UpdatedAt,
	}
}

type updateStripeConfigurationRequest struct {
	APIKey                       *string `json:"api_key"`
	PaymentMethodConfigurationID string  `json:"payment_method_configuration_id"`
	ProductTaxCode               string  `json:"product_tax_code"`
	PurchasesEnabled             *bool   `json:"purchases_enabled"`
}

type createPaymentRefundRequest struct {
	IdeaQuantity   int64  `json:"idea_quantity"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

func (h *AdminHandler) ListAdminPaymentOrders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.service == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	input, err := adminPaymentOrderListInputFromQuery(c)
	if err != nil {
		writeAdminPaymentOrderListError(c, err)
		return
	}
	page, err := h.service.ListAdminPaymentOrders(c.Request.Context(), input)
	if err != nil {
		writeAdminPaymentOrderListError(c, err)
		return
	}
	contract.RenderSuccess(c, page)
}

func (h *AdminHandler) GetAdminPaymentOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.service == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	detail, err := h.service.GetAdminPaymentOrder(c.Request.Context(), c.Param("order_number"))
	if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单号无效")
		return
	}
	if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderNotFound) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "支付订单不存在")
		return
	}
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "读取支付订单失败")
		return
	}
	contract.RenderSuccess(c, detail)
}

func (h *AdminHandler) CreateAdminPaymentRefund(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "退款服务暂不可用")
		return
	}
	var request createPaymentRefundRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxPaymentRefundRequestBytes, &request); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "退款参数错误")
		return
	}
	refund, err := h.service.CreatePaymentRefund(c.Request.Context(), paymentcontracts.CreatePaymentRefundCommand{
		ActorID: admin.UserID(), OrderNumber: c.Param("order_number"), IdeaQuantity: request.IdeaQuantity,
		IdempotencyKey: request.IdempotencyKey, Reason: request.Reason,
	})
	if err != nil {
		switch {
		case errors.Is(err, paymentcontracts.ErrRefundAmountInvalid):
			slog.Warn("admin payment refund rejected", "outcome", "invalid_amount", "err", err)
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "退款 Idea 数量无效")
		case errors.Is(err, paymentcontracts.ErrRefundIdempotency):
			slog.Warn("admin payment refund rejected", "outcome", "invalid_idempotency", "err", err)
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "退款幂等键无效")
		case errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter):
			slog.Warn("admin payment refund rejected", "outcome", "invalid_filter", "err", err)
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "支付订单筛选条件无效")
		case errors.Is(err, paymentcontracts.ErrRefundExpired):
			slog.Warn("admin payment refund rejected", "outcome", "refund_expired", "err", err)
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "支付订单已超过 14 天退款期限")
		case errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible):
			slog.Warn("admin payment refund rejected", "outcome", "order_not_eligible", "err", err)
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "支付订单当前不满足退款条件")
		case errors.Is(err, paymentcontracts.ErrRefundAmountExceeds):
			slog.Warn("admin payment refund rejected", "outcome", "amount_exceeds", "err", err)
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "退款 Idea 数量超过可退余额")
		case errors.Is(err, domain.ErrInsufficientCredits):
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "Wallet 可用 Idea 不足")
		case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "Stripe 服务暂不可用")
		default:
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "发起退款失败")
		}
		return
	}
	contract.RenderSuccess(c, newAdminPaymentRefundResult(refund))
}

func adminPaymentOrderListInputFromQuery(c *gin.Context) (paymentcontracts.AdminPaymentOrderListInput, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return paymentcontracts.AdminPaymentOrderListInput{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	query := c.Request.URL.Query()
	allowed := map[string]struct{}{
		"order_number": {}, "user": {}, "status": {}, "anomaly_only": {}, "cursor": {}, "limit": {},
	}
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			return paymentcontracts.AdminPaymentOrderListInput{}, paymentcontracts.ErrAdminPaymentOrderFilter
		}
	}
	input := paymentcontracts.AdminPaymentOrderListInput{
		OrderNumber: query.Get("order_number"), User: query.Get("user"),
		Status: paymentsdomain.PaymentOrderStatus(query.Get("status")), Cursor: query.Get("cursor"),
	}
	if raw, ok := query["anomaly_only"]; ok {
		if raw[0] != "true" && raw[0] != "false" {
			return paymentcontracts.AdminPaymentOrderListInput{}, paymentcontracts.ErrAdminPaymentOrderFilter
		}
		input.AnomalyOnly = raw[0] == "true"
	}
	if raw, ok := query["limit"]; ok {
		limit, err := pagination.ParseLimit(raw[0])
		if err != nil {
			return paymentcontracts.AdminPaymentOrderListInput{}, paymentcontracts.ErrAdminPaymentOrderFilter
		}
		input.Limit = limit
	}
	return input, nil
}

// adminPaymentOrderListErrorTable binds admin order list outcomes to stable
// machine codes so callers branch on codes instead of copy. Limit and filter
// failures share the filter code; cursor failures keep their own code, both
// tracking the shared pagination standard.
var adminPaymentOrderListErrorTable = []paymentErrorMapping{
	{
		targets: []error{paymentcontracts.ErrAdminPaymentOrderFilter},
		status:  http.StatusBadRequest,
		code:    string(contract.CodeAdminPaymentOrderFilterInvalid),
		message: "支付订单筛选条件无效",
	},
	{
		targets: []error{paymentcontracts.ErrAdminPaymentOrderCursor},
		status:  http.StatusBadRequest,
		code:    string(contract.CodeAdminPaymentOrderCursorInvalid),
		message: "支付订单游标无效",
	},
}

func writeAdminPaymentOrderListError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, adminPaymentOrderListErrorTable, http.StatusInternalServerError, "读取支付订单失败")
}

func (h *AdminHandler) GetStripeConfiguration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.service == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	summary, err := h.service.GetStripeConfiguration(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "读取 Stripe 设置失败")
		return
	}
	contract.RenderSuccess(c, summary)
}

func (h *AdminHandler) UpdateStripeConfiguration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	var request updateStripeConfigurationRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxStripeConfigurationRequestBytes, &request); err != nil || request.PurchasesEnabled == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Stripe 设置参数错误")
		return
	}
	apiKey := ""
	if request.APIKey != nil {
		apiKey = *request.APIKey
	}
	summary, err := h.service.ConfigureStripe(c.Request.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: admin.UserID(), APIKey: apiKey, ProductTaxCode: request.ProductTaxCode,
		PaymentMethodConfigurationID: request.PaymentMethodConfigurationID,
		PurchasesEnabled:             *request.PurchasesEnabled,
		WebhookURL:                   h.publicOrigins.APIURL("/api/webhooks/stripe"),
	})
	if err != nil {
		writeStripeConfigurationError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}
