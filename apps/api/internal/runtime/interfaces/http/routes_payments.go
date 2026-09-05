package httpapi

import (
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"

	"github.com/gin-gonic/gin"
)

func registerPaymentWebhookRoutes(r *gin.Engine, handler *paymenthttp.WebhookHandler) {
	if handler == nil {
		return
	}
	r.POST(RoutePaymentStripeWebhook, handler.ReceiveStripeWebhook)
}
