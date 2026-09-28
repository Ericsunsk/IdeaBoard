package http

import (
	"log/slog"
	"net/http"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/contract"
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
		slog.Error("payment error mapping missing safe message", "code", m.code, "status", m.status, "err", err)
		msg = msgRender
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

// stripeWebhookErrorTable is the single source of truth for the wired
// Stripe webhook edge (the only handler left in this package;
// NewWebhookHandler). User-checkout and admin handlers were deleted
// with their tables: only WebhookHandler is wired in composition.
var stripeWebhookErrorTable = []paymentErrorMapping{
	{
		targets: []error{
			paymentports.ErrInvalidWebhookSignature, paymentports.ErrWebhookSignatureExpired,
			paymentports.ErrMalformedWebhookPayload,
		},
		status:  http.StatusBadRequest,
		code:    string(contract.CodeWebhookSignatureInvalid),
		message: msgRenderMappedPaymentError,
	},
	{
		targets: []error{paymentcontracts.ErrStripeConfigurationNotFound, paymentcontracts.ErrStripeConfigurationInvalid},
		status:  http.StatusServiceUnavailable,
		code:    string(contract.CodeWebhookConfigurationMissing),
		message: msgRenderMappedPaymentError2,
	},
}

func writeStripeWebhookError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, stripeWebhookErrorTable, http.StatusInternalServerError, msgWriteStripeWebhookError)
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgRender                    = "请求处理失败，请稍后重试"        // Render @ errors.go
	msgRenderMappedPaymentError  = "Stripe Webhook 签名无效" // RenderMappedPaymentError @ errors.go
	msgRenderMappedPaymentError2 = "Stripe Webhook 尚未配置" // RenderMappedPaymentError @ errors.go
	msgWriteStripeWebhookError   = "Stripe Webhook 处理失败" // WriteStripeWebhookError @ errors.go
	msgReceiveStripeWebhook      = "Stripe Webhook 暂不可用" // ReceiveStripeWebhook @ webhook.go
	msgReceiveStripeWebhook2     = "请求体过大"               // ReceiveStripeWebhook @ webhook.go
	msgReceiveStripeWebhook3     = "Stripe Webhook 数据无效" // ReceiveStripeWebhook @ webhook.go
)
