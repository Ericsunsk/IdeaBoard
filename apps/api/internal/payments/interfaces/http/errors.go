package http

import (
	"errors"
	"net/http"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func writeStripeConfigurationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentcontracts.ErrStripeAPIKeyRequired), errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey),
		errors.Is(err, paymentcontracts.ErrInvalidPaymentMethodConfiguration), errors.Is(err, paymentcontracts.ErrInvalidProductTaxCode), errors.Is(err, paymentcontracts.ErrInvalidWebhookURL):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, paymentcontracts.ErrStripePermissionDenied):
		response.Error(c, http.StatusForbidden, paymentcontracts.ErrStripePermissionDenied.Error())
	case errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable), errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable), errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid):
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
		response.Error(c, http.StatusServiceUnavailable, paymentcontracts.ErrStripeProviderUnavailable.Error())
	case errors.Is(err, paymentcontracts.ErrStripeConfigurationChanged):
		response.Error(c, http.StatusConflict, "Stripe 配置已被其他操作更新，请重试")
	default:
		response.Error(c, http.StatusInternalServerError, "保存 Stripe 设置失败")
	}
}

func writeCheckoutError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentsdomain.ErrInvalidIdeaQuantity), errors.Is(err, paymentsdomain.ErrTermsAcceptanceRequired), errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyKey):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict), errors.Is(err, paymentcontracts.ErrPendingCheckoutLimit), errors.Is(err, paymentcontracts.ErrPaymentAlreadyInProgress):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, paymentcontracts.ErrPaymentAlreadyCompleted):
		response.MachineError(c, http.StatusUnprocessableEntity, "payment_already_completed", err.Error())
	case errors.Is(err, paymentcontracts.ErrCheckoutSessionExpired):
		response.MachineError(c, http.StatusUnprocessableEntity, "checkout_session_expired", err.Error())
	case errors.Is(err, paymentsdomain.ErrWalletNotRechargeable):
		response.Error(c, http.StatusLocked, err.Error())
	case errors.Is(err, paymentsdomain.ErrCheckoutRequestRejected):
		response.MachineError(c, http.StatusUnprocessableEntity, "checkout_request_rejected", "无法创建支付页面，请重试")
	case errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
	default:
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
	}
}

func writeStripeWebhookError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentports.ErrInvalidWebhookSignature), errors.Is(err, paymentports.ErrWebhookSignatureExpired), errors.Is(err, paymentports.ErrMalformedWebhookPayload):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, paymentcontracts.ErrStripeConfigurationNotFound), errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid):
		response.Error(c, http.StatusServiceUnavailable, "Stripe Webhook 尚未配置")
	default:
		response.Error(c, http.StatusInternalServerError, "Stripe Webhook 处理失败")
	}
}
