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

// paymentErrorMapping binds one stable outcome to its wire rendering. Codes
// are machine-stable from here on; do not rename one without coordinating
// the frontend. An empty message renders err.Error(); every table row must
// provide a machine code.
//
// The tables below are the single source of truth for payments HTTP error
// rendering. Domain, ports, and contracts sentinels stay dependency-free by
// architecture guard (no internal imports allowed there), so classification
// lives here at the edge instead of on the sentinels themselves.
type paymentErrorMapping struct {
	targets []error
	status  int
	code    string
	message string
}

func (m paymentErrorMapping) matches(err error) bool {
	for _, target := range m.targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (m paymentErrorMapping) render(c *gin.Context, err error) {
	msg := m.message
	if msg == "" {
		msg = err.Error()
	}
	response.MachineError(c, m.status, m.code, msg)
}

func renderMappedPaymentError(c *gin.Context, err error, table []paymentErrorMapping, defaultStatus int, defaultMessage string) {
	for _, m := range table {
		if m.matches(err) {
			m.render(c, err)
			return
		}
	}
	response.Error(c, defaultStatus, defaultMessage)
}

var stripeConfigurationErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentcontracts.ErrStripeAPIKeyRequired, paymentcontracts.ErrInvalidStripeAPIKey,
			paymentcontracts.ErrInvalidPaymentMethodConfiguration, paymentcontracts.ErrInvalidProductTaxCode,
			paymentcontracts.ErrInvalidWebhookURL,
		},
		status: http.StatusBadRequest,
		code:   "stripe_configuration_invalid_input",
	},
	{
		targets: []error{paymentcontracts.ErrStripePermissionDenied},
		status:  http.StatusForbidden,
		code:    "stripe_permission_denied",
		message: paymentcontracts.ErrStripePermissionDenied.Error(),
	},
	{
		targets: []error{
			paymentcontracts.ErrStripeAccountUnavailable, paymentcontracts.ErrStripeCapabilityUnavailable,
			paymentcontracts.ErrStripeConfigurationInvalid,
		},
		status: http.StatusUnprocessableEntity,
		code:   "stripe_configuration_unprocessable",
	},
	{
		targets: []error{paymentcontracts.ErrStripeProviderUnavailable},
		status:  http.StatusServiceUnavailable,
		code:    "stripe_provider_unavailable",
		message: paymentcontracts.ErrStripeProviderUnavailable.Error(),
	},
	{
		targets: []error{paymentcontracts.ErrStripeConfigurationChanged},
		status:  http.StatusConflict,
		code:    "stripe_configuration_changed",
		message: "Stripe 配置已被其他操作更新，请重试",
	},
}

var checkoutErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentsdomain.ErrInvalidIdeaQuantity, paymentsdomain.ErrTermsAcceptanceRequired,
			paymentsdomain.ErrCheckoutIdempotencyKey,
		},
		status: http.StatusBadRequest,
		code:   "checkout_invalid_input",
	},
	{
		targets: []error{
			paymentsdomain.ErrCheckoutIdempotencyConflict, paymentcontracts.ErrPendingCheckoutLimit,
			paymentcontracts.ErrPaymentAlreadyInProgress,
		},
		status: http.StatusConflict,
		code:   "checkout_conflict",
	},
	{
		targets: []error{paymentcontracts.ErrPaymentAlreadyCompleted},
		status:  http.StatusUnprocessableEntity,
		code:    "payment_already_completed",
	},
	{
		targets: []error{paymentcontracts.ErrCheckoutSessionExpired},
		status:  http.StatusUnprocessableEntity,
		code:    "checkout_session_expired",
	},
	{
		targets: []error{paymentsdomain.ErrWalletNotRechargeable},
		status:  http.StatusLocked,
		code:    "wallet_not_rechargeable",
	},
	{
		targets: []error{paymentsdomain.ErrCheckoutRequestRejected},
		status:  http.StatusUnprocessableEntity,
		code:    "checkout_request_rejected",
		// Provider details stay out of the response: the joined error may
		// carry raw decline payloads, so render the fixed safe copy instead.
		message: "无法创建支付页面，请重试",
	},
	{
		targets: []error{paymentcontracts.ErrStripeProviderUnavailable},
		status:  http.StatusServiceUnavailable,
		code:    "payment_service_unavailable",
		message: "支付服务暂不可用",
	},
	{
		targets: []error{paymentsdomain.ErrPurchasesUnavailable},
		status:  http.StatusServiceUnavailable,
		code:    "purchases_unavailable",
		message: "充值服务暂不可用",
	},
}

var stripeWebhookErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentports.ErrInvalidWebhookSignature, paymentports.ErrWebhookSignatureExpired,
			paymentports.ErrMalformedWebhookPayload,
		},
		status: http.StatusBadRequest,
		code:   "webhook_signature_invalid",
	},
	{
		targets: []error{paymentcontracts.ErrStripeConfigurationNotFound, paymentcontracts.ErrStripeConfigurationInvalid},
		status:  http.StatusServiceUnavailable,
		code:    "webhook_configuration_missing",
		message: "Stripe Webhook 尚未配置",
	},
}

func writeStripeConfigurationError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, stripeConfigurationErrorTable, http.StatusInternalServerError, "保存 Stripe 设置失败")
}

func writeCheckoutError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, checkoutErrorTable, http.StatusServiceUnavailable, "充值服务暂不可用")
}

func writeStripeWebhookError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, stripeWebhookErrorTable, http.StatusInternalServerError, "Stripe Webhook 处理失败")
}
