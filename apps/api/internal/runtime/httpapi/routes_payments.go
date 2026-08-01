package httpapi

import (
	"ideaboard/api/internal/payments"

	"github.com/gin-gonic/gin"
)

func registerPaymentWebhookRoutes(r *gin.Engine, handler *payments.Handler) {
	if handler == nil {
		return
	}
	r.POST(RoutePaymentStripeWebhook, handler.ReceiveStripeWebhook)
}
