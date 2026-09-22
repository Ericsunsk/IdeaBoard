package http

import (
	"log/slog"
	"net/http"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// paymentErrorMapping binds one stable outcome to its wire rendering. Codes
// are machine-stable from here on; do not rename one without coordinating
// the frontend. Every table row must provide a machine code and a fixed safe
// message: the wire envelope never renders raw error text so wrapped provider,
// storage, or database details cannot leak. The original error stays
// attributable in server logs via slog.
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

func (m paymentErrorMapping) Matches(err error) bool {
	return errorrender.TargetsMatch(m.targets, err)
}

func (m paymentErrorMapping) render(c *gin.Context, err error) {
	msg := m.message
	if msg == "" {
		slog.Warn("payment error mapping missing safe message", "code", m.code, "status", m.status, "err", err)
		msg = "请求处理失败，请稍后重试"
	}
	errorrender.Render(c, m.status, m.code, msg)
}

func renderMappedPaymentError(c *gin.Context, err error, table []paymentErrorMapping, defaultStatus int, defaultMessage string) {
	if row, ok := errorrender.FirstMatch(table, err); ok {
		row.render(c, err)
		return
	}
	errorrender.RenderNumeric(c, defaultStatus, defaultMessage)
}

var stripeConfigurationErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentcontracts.ErrStripeAPIKeyRequired, paymentcontracts.ErrInvalidStripeAPIKey,
			paymentcontracts.ErrInvalidPaymentMethodConfiguration, paymentcontracts.ErrInvalidProductTaxCode,
			paymentcontracts.ErrInvalidWebhookURL,
		},
		status:  http.StatusBadRequest,
		code:    "stripe_configuration_invalid_input",
		message: "Stripe 配置参数错误",
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
		status:  http.StatusUnprocessableEntity,
		code:    "stripe_configuration_unprocessable",
		message: "当前 Stripe 配置不可用",
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
		status:  http.StatusBadRequest,
		code:    "checkout_invalid_input",
		message: "充值参数错误",
	},
	{
		targets: []error{
			paymentsdomain.ErrCheckoutIdempotencyConflict, paymentcontracts.ErrPendingCheckoutLimit,
			paymentcontracts.ErrPaymentAlreadyInProgress,
		},
		status:  http.StatusConflict,
		code:    "checkout_conflict",
		message: "充值请求冲突，请稍后重试",
	},
	{
		targets: []error{paymentcontracts.ErrPaymentAlreadyCompleted},
		status:  http.StatusUnprocessableEntity,
		code:    "payment_already_completed",
		message: "该充值已完成，请勿重复支付",
	},
	{
		targets: []error{paymentcontracts.ErrCheckoutSessionExpired},
		status:  http.StatusUnprocessableEntity,
		code:    "checkout_session_expired",
		message: "支付页面已过期，请重新发起充值",
	},
	{
		targets: []error{paymentsdomain.ErrWalletNotRechargeable},
		status:  http.StatusConflict,
		code:    "wallet_not_rechargeable",
		message: "当前账户暂不可充值",
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

var rechargeHistoryErrorTable = []paymentErrorMapping{
	{
		targets: []error{paymentcontracts.ErrRechargeHistoryLimit},
		status:  http.StatusBadRequest,
		code:    "recharge_history_limit_invalid",
		message: "充值记录 limit 无效",
	},
	{
		targets: []error{paymentcontracts.ErrRechargeHistoryCursor},
		status:  http.StatusBadRequest,
		code:    "recharge_history_cursor_invalid",
		message: "充值记录游标无效",
	},
}

var stripeWebhookErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentports.ErrInvalidWebhookSignature, paymentports.ErrWebhookSignatureExpired,
			paymentports.ErrMalformedWebhookPayload,
		},
		status:  http.StatusBadRequest,
		code:    "webhook_signature_invalid",
		message: "Stripe Webhook 签名无效",
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

// writeRechargeHistoryError renders recharge-history failures through the
// machine envelope so callers branch on stable codes instead of copy.
// Pagination outcomes (limit/cursor) are part of the shared pagination
// standard; unmapped failures keep the established safe message.
func writeRechargeHistoryError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, rechargeHistoryErrorTable, http.StatusInternalServerError, "读取充值记录失败")
}
