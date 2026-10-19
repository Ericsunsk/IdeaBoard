package http

import (
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
// Message: the wire envelope never renders raw error text so wrapped provider,
// storage, or database details cannot leak. The original error stays
// attributable in server logs via slog.
//
// The tables below are the single source of truth for payments HTTP error
// rendering. Domain, ports, and contracts sentinels stay dependency-free by
// architecture guard (no internal imports allowed there), so classification
// lives here at the edge instead of on the sentinels themselves.
type paymentErrorMapping = errorrender.Mapping

func renderMappedPaymentError(c *gin.Context, err error, table []paymentErrorMapping, defaultStatus int, defaultMessage string) {
	errorrender.RenderMapped(c, err, table, errorrender.MappedOptions{
		DefaultStatus:   defaultStatus,
		DefaultMessage:  defaultMessage,
		FallbackMessage: errorrender.MsgPaymentsInterfacesHTTPRender,
		// The webhook default predates the machine-code contract: it
		// keeps the legacy numeric envelope and stays silent, while
		// every mapped row renders the machine envelope.
		NumericDefault: true,
	})
}

// stripeWebhookErrorTable is the single source of truth for the wired
// Stripe webhook edge (the only handler left in this package;
// NewWebhookHandler). User-checkout and admin handlers were deleted
// with their tables: only WebhookHandler is wired in composition.
var stripeWebhookErrorTable = []paymentErrorMapping{
	{
		Targets: []error{
			paymentports.ErrInvalidWebhookSignature, paymentports.ErrWebhookSignatureExpired,
			paymentports.ErrMalformedWebhookPayload,
		},
		Status:  http.StatusBadRequest,
		Code:    string(contract.CodeWebhookSignatureInvalid),
		Message: errorrender.MsgRenderMappedPaymentError,
	},
	{
		Targets: []error{paymentcontracts.ErrStripeConfigurationNotFound, paymentcontracts.ErrStripeConfigurationInvalid},
		Status:  http.StatusServiceUnavailable,
		Code:    string(contract.CodeWebhookConfigurationMissing),
		Message: errorrender.MsgRenderMappedPaymentError2,
	},
}

func writeStripeWebhookError(c *gin.Context, err error) {
	renderMappedPaymentError(c, err, stripeWebhookErrorTable, http.StatusInternalServerError, errorrender.MsgWriteStripeWebhookError)
}
