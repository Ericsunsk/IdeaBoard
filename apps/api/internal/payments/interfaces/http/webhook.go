package http

import (
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *WebhookHandler) ReceiveStripeWebhook(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, errorrender.MsgReceiveStripeWebhook)
		return
	}
	if c.Request == nil || c.Request.Body == nil || c.Request.ContentLength > maxStripeWebhookRequestBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, errorrender.MsgReceiveStripeWebhook2)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStripeWebhookRequestBytes+1))
	if err != nil {
		response.Error(c, http.StatusBadRequest, errorrender.MsgReceiveStripeWebhook3)
		return
	}
	if int64(len(raw)) > maxStripeWebhookRequestBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, errorrender.MsgReceiveStripeWebhook2)
		return
	}
	receipt, err := h.service.ReceiveStripeWebhook(c.Request.Context(), raw, c.GetHeader("Stripe-Signature"))
	if err != nil {
		writeStripeWebhookError(c, err)
		return
	}
	response.SuccessDTO(c, receipt)
}

// merged from internal/payments/interfaces/http/handler.go (structural consolidation; symbols unchanged).
const (
	maxStripeWebhookRequestBytes int64 = 128 << 10
)

type WebhookHandler struct {
	service paymentcontracts.WebhookService
}

func NewWebhookHandler(service paymentcontracts.WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}
