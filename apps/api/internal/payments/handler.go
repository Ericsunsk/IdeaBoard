package payments

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

const (
	maxStripeConfigurationRequestBytes int64 = 16 << 10
	maxStripeWebhookRequestBytes       int64 = 128 << 10
)

type Handler struct {
	service    *Service
	publicURLs settings.PublicURLSettingProvider
}

type updateStripeConfigurationRequest struct {
	APIKey           *string `json:"api_key"`
	ProductTaxCode   string  `json:"product_tax_code"`
	PurchasesEnabled *bool   `json:"purchases_enabled"`
}

func NewHandler(service *Service, publicURLs settings.PublicURLSettingProvider) *Handler {
	return &Handler{service: service, publicURLs: publicURLs}
}

func (h *Handler) GetStripeConfiguration(c *gin.Context) {
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

func (h *Handler) UpdateStripeConfiguration(c *gin.Context) {
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
	baseURL := settings.ResolvePublicBaseURL(c.Request.Context(), h.publicURLs, paymentRequestScheme(c), c.Request.Host)
	apiKey := ""
	if request.APIKey != nil {
		apiKey = *request.APIKey
	}
	summary, err := h.service.ConfigureStripe(c.Request.Context(), ConfigureStripeCommand{
		ActorID: admin.UserID(), APIKey: apiKey, ProductTaxCode: request.ProductTaxCode,
		PurchasesEnabled: *request.PurchasesEnabled,
		WebhookURL:       strings.TrimRight(baseURL, "/") + "/api/webhooks/stripe",
	})
	if err != nil {
		writeStripeConfigurationError(c, err)
		return
	}
	response.SuccessDTO(c, summary)
}

func (h *Handler) ReceiveStripeWebhook(c *gin.Context) {
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
	default:
		response.Error(c, http.StatusInternalServerError, "保存 Stripe 设置失败")
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

func paymentRequestScheme(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "http"
	}
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(strings.Split(proto, ",")[0])
	}
	return "http"
}
