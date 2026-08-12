package payments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxStripeConfigurationRequestBytes int64 = 16 << 10
	maxStripeWebhookRequestBytes       int64 = 128 << 10
	maxPaymentCheckoutRequestBytes     int64 = 16 << 10
	maxPaymentRefundRequestBytes       int64 = 16 << 10
	paymentEventsHeartbeatInterval           = 15 * time.Second
)

type UserCheckoutService interface {
	GetPurchaseOptions(context.Context, int64) (PurchaseOptions, error)
	CreateCheckout(context.Context, CreateCheckoutCommand) (CheckoutResult, error)
	GetPaymentOrderReturnStatus(context.Context, int64, string) (PaymentOrderReturnStatus, error)
	ListPendingOrders(context.Context, int64) (PendingPaymentOrderList, error)
	ContinuePendingOrder(context.Context, int64, string) (PendingPaymentOrder, error)
	AbandonPendingOrder(context.Context, int64, string) (PendingPaymentOrder, error)
	ListRecharges(context.Context, int64, string, int) (RechargeHistoryPage, error)
}

type AdminService interface {
	GetStripeConfiguration(context.Context) (StripeConfigurationSummary, error)
	ConfigureStripe(context.Context, ConfigureStripeCommand) (StripeConfigurationSummary, error)
	ListAdminPaymentOrders(context.Context, AdminPaymentOrderListInput) (AdminPaymentOrderPage, error)
	GetAdminPaymentOrder(context.Context, string) (AdminPaymentOrderDetail, error)
	CreatePaymentRefund(context.Context, CreatePaymentRefundCommand) (PaymentRefund, error)
}

type WebhookService interface {
	ReceiveStripeWebhook(context.Context, []byte, string) (StripeWebhookReceipt, error)
}

type UserPaymentEventStream interface {
	Subscribe(int64) (<-chan PaymentEvent, func())
}

type UserCheckoutHandler struct {
	service       UserCheckoutService
	publicOrigins config.PublicOriginsConfig
}

type AdminHandler struct {
	service       AdminService
	publicOrigins config.PublicOriginsConfig
}

type WebhookHandler struct {
	service WebhookService
}

type EventStreamHandler struct {
	events UserPaymentEventStream
}

type updateStripeConfigurationRequest struct {
	APIKey           *string `json:"api_key"`
	ProductTaxCode   string  `json:"product_tax_code"`
	PurchasesEnabled *bool   `json:"purchases_enabled"`
}

type createPaymentCheckoutRequest struct {
	IdempotencyKey      string `json:"idempotency_key"`
	IdeaQuantity        int64  `json:"idea_quantity"`
	TermsAccepted       bool   `json:"terms_accepted"`
	TermsVersion        string `json:"terms_version"`
	RefundPolicyVersion string `json:"refund_policy_version"`
}

type createPaymentRefundRequest struct {
	IdeaQuantity   int64  `json:"idea_quantity"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

func NewUserCheckoutHandler(service UserCheckoutService, publicOrigins config.PublicOriginsConfig) *UserCheckoutHandler {
	return &UserCheckoutHandler{service: service, publicOrigins: publicOrigins}
}

func NewAdminHandler(service AdminService, publicOrigins config.PublicOriginsConfig) *AdminHandler {
	return &AdminHandler{service: service, publicOrigins: publicOrigins}
}

func NewWebhookHandler(service WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}

func NewEventStreamHandler(events UserPaymentEventStream) *EventStreamHandler {
	return &EventStreamHandler{events: events}
}

func (h *UserCheckoutHandler) GetPurchaseOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	options, err := h.service.GetPurchaseOptions(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取充值选项失败")
		return
	}
	response.SuccessDTO(c, options)
}

func (h *UserCheckoutHandler) CreateCheckoutSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	var request createPaymentCheckoutRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxPaymentCheckoutRequestBytes, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "充值参数错误")
		return
	}
	returnBaseURL, err := h.checkoutReturnBaseURL()
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	result, err := h.service.CreateCheckout(c.Request.Context(), CreateCheckoutCommand{
		UserID: identityPrincipal.UserID(), ClientIdempotencyKey: request.IdempotencyKey,
		IdeaQuantity: request.IdeaQuantity, TermsAccepted: request.TermsAccepted,
		TermsVersion: request.TermsVersion, RefundPolicyVersion: request.RefundPolicyVersion,
		ReturnBaseURL: returnBaseURL,
	})
	if err != nil {
		writeCheckoutError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *UserCheckoutHandler) GetPaymentOrderReturnStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	result, err := h.service.GetPaymentOrderReturnStatus(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取充值订单失败")
		return
	}
	response.SuccessDTO(c, result)
}

func (h *UserCheckoutHandler) ListPendingOrders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	orders, err := h.service.ListPendingOrders(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取待支付订单失败")
		return
	}
	response.SuccessDTO(c, orders)
}

func (h *UserCheckoutHandler) ContinuePendingOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	if !requireEmptyPaymentActionBody(c) {
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	order, err := h.service.ContinuePendingOrder(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if errors.Is(err, ErrWalletNotRechargeable) {
		response.Error(c, http.StatusLocked, "当前账户暂不可继续充值")
		return
	}
	if errors.Is(err, ErrStripeProviderUnavailable) || errors.Is(err, ErrPurchasesUnavailable) {
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "继续充值订单失败")
		return
	}
	response.SuccessDTO(c, order)
}

func (h *UserCheckoutHandler) AbandonPendingOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	if !requireEmptyPaymentActionBody(c) {
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	order, err := h.service.AbandonPendingOrder(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if errors.Is(err, ErrStripeProviderUnavailable) || errors.Is(err, ErrPurchasesUnavailable) {
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "放弃充值订单失败")
		return
	}
	response.SuccessDTO(c, order)
}

// paymentOrderPublicToken intentionally maps malformed and inaccessible tokens
// to the same response, so the public token is never an ownership oracle.
func paymentOrderPublicToken(c *gin.Context) (string, bool) {
	publicToken := strings.TrimSpace(c.Param("public_token"))
	parsed, err := uuid.Parse(publicToken)
	if err != nil || parsed.String() != strings.ToLower(publicToken) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return "", false
	}
	return publicToken, true
}

// Action routes intentionally have no request schema. Reject a body rather
// than silently accepting undeclared fields and creating a second command API.
func requireEmptyPaymentActionBody(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPaymentCheckoutRequestBytes+1))
	if err != nil || int64(len(body)) > maxPaymentCheckoutRequestBytes || strings.TrimSpace(string(body)) != "" {
		response.Error(c, http.StatusBadRequest, "充值操作参数错误")
		return false
	}
	return true
}

func (h *UserCheckoutHandler) ListRecharges(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return
		}
		limit = value
	}
	page, err := h.service.ListRecharges(c.Request.Context(), identityPrincipal.UserID(), c.Query("cursor"), limit)
	if errors.Is(err, ErrRechargeHistoryCursor) || errors.Is(err, ErrRechargeHistoryLimit) {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取充值记录失败")
		return
	}
	response.SuccessDTO(c, page)
}

func (h *EventStreamHandler) StreamUserPaymentEvents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	var events UserPaymentEventStream
	if h != nil {
		events = h.events
	}
	if events == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值事件服务暂不可用")
		return
	}
	eventChannel, unsubscribe := events.Subscribe(identityPrincipal.UserID())
	if eventChannel == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值事件服务暂不可用")
		return
	}
	defer unsubscribe()
	if err := sse.Prepare(c); err != nil {
		// A write-deadline reset is an optional transport optimization. The stream
		// remains usable on ResponseWriter implementations that do not expose it.
		slog.Warn("clear payment SSE write deadline failed", "err", err)
	}
	c.Header("Cache-Control", "no-store")
	heartbeat := time.NewTicker(paymentEventsHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event := <-eventChannel:
			if err := sse.WriteEvent(c, "payment", event); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := c.Writer.Write([]byte(":keepalive\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
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
	if errors.Is(err, ErrAdminPaymentOrderFilter) || errors.Is(err, ErrAdminPaymentOrderCursor) {
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
	if errors.Is(err, ErrAdminPaymentOrderFilter) {
		response.Error(c, http.StatusBadRequest, "支付订单号无效")
		return
	}
	if errors.Is(err, ErrAdminPaymentOrderNotFound) {
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
	refund, err := h.service.CreatePaymentRefund(c.Request.Context(), CreatePaymentRefundCommand{
		ActorID: admin.UserID(), OrderNumber: c.Param("order_number"), IdeaQuantity: request.IdeaQuantity,
		IdempotencyKey: request.IdempotencyKey, Reason: request.Reason,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrRefundAmountInvalid), errors.Is(err, ErrRefundIdempotency), errors.Is(err, ErrAdminPaymentOrderFilter):
			response.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrRefundExpired), errors.Is(err, ErrRefundOrderNotEligible), errors.Is(err, ErrRefundAmountExceeds):
			response.Error(c, http.StatusConflict, err.Error())
		case errors.Is(err, credits.ErrInsufficientCredits):
			response.Error(c, http.StatusConflict, "Wallet 可用 Idea 不足")
		case errors.Is(err, ErrStripeProviderUnavailable):
			response.Error(c, http.StatusServiceUnavailable, "Stripe 服务暂不可用")
		default:
			response.Error(c, http.StatusInternalServerError, "发起退款失败")
		}
		return
	}
	response.SuccessDTO(c, newAdminPaymentRefundResult(refund))
}

func adminPaymentOrderListInputFromQuery(c *gin.Context) (AdminPaymentOrderListInput, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return AdminPaymentOrderListInput{}, ErrAdminPaymentOrderFilter
	}
	query := c.Request.URL.Query()
	allowed := map[string]struct{}{
		"order_number": {}, "user": {}, "status": {}, "anomaly_only": {}, "cursor": {}, "limit": {},
	}
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			return AdminPaymentOrderListInput{}, ErrAdminPaymentOrderFilter
		}
	}
	input := AdminPaymentOrderListInput{
		OrderNumber: query.Get("order_number"), User: query.Get("user"),
		Status: PaymentOrderStatus(query.Get("status")), Cursor: query.Get("cursor"),
	}
	if raw, ok := query["anomaly_only"]; ok {
		if raw[0] != "true" && raw[0] != "false" {
			return AdminPaymentOrderListInput{}, ErrAdminPaymentOrderFilter
		}
		input.AnomalyOnly = raw[0] == "true"
	}
	if raw, ok := query["limit"]; ok {
		limit, err := strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > maxAdminPaymentOrderLimit {
			return AdminPaymentOrderListInput{}, ErrAdminPaymentOrderFilter
		}
		input.Limit = limit
	}
	return input, nil
}

func (h *UserCheckoutHandler) checkoutReturnBaseURL() (string, error) {
	if h == nil || strings.TrimSpace(h.publicOrigins.Web) == "" {
		return "", ErrPurchasesUnavailable
	}
	return h.publicOrigins.WebURL("/user/boards"), nil
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
	summary, err := h.service.ConfigureStripe(c.Request.Context(), ConfigureStripeCommand{
		ActorID: admin.UserID(), APIKey: apiKey, ProductTaxCode: request.ProductTaxCode,
		PurchasesEnabled: *request.PurchasesEnabled,
		WebhookURL:       h.publicOrigins.APIURL("/api/webhooks/stripe"),
	})
	if err != nil {
		writeStripeConfigurationError(c, err)
		return
	}
	response.SuccessDTO(c, summary)
}

func (h *WebhookHandler) ReceiveStripeWebhook(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Stripe Webhook 暂不可用")
		return
	}
	if c.Request == nil || c.Request.Body == nil || c.Request.ContentLength > maxStripeWebhookRequestBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "请求体过大")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStripeWebhookRequestBytes+1))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Stripe Webhook 数据无效")
		return
	}
	if int64(len(raw)) > maxStripeWebhookRequestBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "请求体过大")
		return
	}
	receipt, err := h.service.ReceiveStripeWebhook(c.Request.Context(), raw, c.GetHeader("Stripe-Signature"))
	if err != nil {
		writeStripeWebhookError(c, err)
		return
	}
	response.SuccessDTO(c, receipt)
}

func writeStripeConfigurationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrStripeAPIKeyRequired), errors.Is(err, ErrInvalidStripeAPIKey),
		errors.Is(err, ErrInvalidProductTaxCode), errors.Is(err, ErrInvalidWebhookURL):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrStripePermissionDenied):
		response.Error(c, http.StatusForbidden, ErrStripePermissionDenied.Error())
	case errors.Is(err, ErrStripeAccountUnavailable), errors.Is(err, ErrStripeCapabilityUnavailable), errors.Is(err, ErrStripeConfigurationInvalid):
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrStripeProviderUnavailable):
		response.Error(c, http.StatusServiceUnavailable, ErrStripeProviderUnavailable.Error())
	case errors.Is(err, ErrStripeConfigurationChanged):
		response.Error(c, http.StatusConflict, ErrStripeConfigurationChanged.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "保存 Stripe 设置失败")
	}
}

func writeCheckoutError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidIdeaQuantity), errors.Is(err, ErrTermsAcceptanceRequired), errors.Is(err, ErrCheckoutIdempotencyKey):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrCheckoutIdempotencyConflict), errors.Is(err, ErrPendingCheckoutLimit), errors.Is(err, ErrPaymentAlreadyInProgress):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, ErrPaymentAlreadyCompleted):
		response.MachineError(c, http.StatusUnprocessableEntity, "payment_already_completed", err.Error())
	case errors.Is(err, ErrCheckoutSessionExpired):
		response.MachineError(c, http.StatusUnprocessableEntity, "checkout_session_expired", err.Error())
	case errors.Is(err, ErrWalletNotRechargeable):
		response.Error(c, http.StatusLocked, err.Error())
	case errors.Is(err, ErrCheckoutRequestRejected):
		response.MachineError(c, http.StatusUnprocessableEntity, "checkout_request_rejected", "无法创建支付页面，请重试")
	case errors.Is(err, ErrStripeProviderUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
	default:
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
	}
}

func writeStripeWebhookError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidWebhookSignature), errors.Is(err, ErrWebhookSignatureExpired), errors.Is(err, ErrMalformedWebhookPayload):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrStripeConfigurationNotFound), errors.Is(err, ErrStripeConfigurationInvalid):
		response.Error(c, http.StatusServiceUnavailable, "Stripe Webhook 尚未配置")
	default:
		response.Error(c, http.StatusInternalServerError, "Stripe Webhook 处理失败")
	}
}
