package http

import (
	"errors"
	"ideaboard/api/internal/credits/domain"
	"net/http"
	"strconv"

	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

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
		response.Error(c, http.StatusServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	input, err := adminPaymentOrderListInputFromQuery(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "支付订单筛选条件无效")
		return
	}
	page, err := h.service.ListAdminPaymentOrders(c.Request.Context(), input)
	if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) || errors.Is(err, paymentcontracts.ErrAdminPaymentOrderCursor) {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取支付订单失败")
		return
	}
	response.SuccessDTO(c, page)
}

func (h *AdminHandler) GetAdminPaymentOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "支付订单服务暂不可用")
		return
	}
	detail, err := h.service.GetAdminPaymentOrder(c.Request.Context(), c.Param("order_number"))
	if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter) {
		response.Error(c, http.StatusBadRequest, "支付订单号无效")
		return
	}
	if errors.Is(err, paymentcontracts.ErrAdminPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "支付订单不存在")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取支付订单失败")
		return
	}
	response.SuccessDTO(c, detail)
}

func (h *AdminHandler) CreateAdminPaymentRefund(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "退款服务暂不可用")
		return
	}
	var request createPaymentRefundRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxPaymentRefundRequestBytes, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "退款参数错误")
		return
	}
	refund, err := h.service.CreatePaymentRefund(c.Request.Context(), paymentcontracts.CreatePaymentRefundCommand{
		ActorID: admin.UserID(), OrderNumber: c.Param("order_number"), IdeaQuantity: request.IdeaQuantity,
		IdempotencyKey: request.IdempotencyKey, Reason: request.Reason,
	})
	if err != nil {
		switch {
		case errors.Is(err, paymentcontracts.ErrRefundAmountInvalid), errors.Is(err, paymentcontracts.ErrRefundIdempotency), errors.Is(err, paymentcontracts.ErrAdminPaymentOrderFilter):
			response.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, paymentcontracts.ErrRefundExpired), errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible), errors.Is(err, paymentcontracts.ErrRefundAmountExceeds):
			response.Error(c, http.StatusConflict, err.Error())
		case errors.Is(err, domain.ErrInsufficientCredits):
			response.Error(c, http.StatusConflict, "Wallet 可用 Idea 不足")
		case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
			response.Error(c, http.StatusServiceUnavailable, "Stripe 服务暂不可用")
		default:
			response.Error(c, http.StatusInternalServerError, "发起退款失败")
		}
		return
	}
	response.SuccessDTO(c, newAdminPaymentRefundResult(refund))
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
		limit, err := strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > maxAdminPaymentOrderLimit {
			return paymentcontracts.AdminPaymentOrderListInput{}, paymentcontracts.ErrAdminPaymentOrderFilter
		}
		input.Limit = limit
	}
	return input, nil
}
func (h *AdminHandler) GetStripeConfiguration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	summary, err := h.service.GetStripeConfiguration(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取 Stripe 设置失败")
		return
	}
	response.SuccessDTO(c, summary)
}

func (h *AdminHandler) UpdateStripeConfiguration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "支付设置服务暂不可用")
		return
	}
	var request updateStripeConfigurationRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxStripeConfigurationRequestBytes, &request); err != nil || request.PurchasesEnabled == nil {
		response.Error(c, http.StatusBadRequest, "Stripe 设置参数错误")
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
	response.SuccessDTO(c, summary)
}
