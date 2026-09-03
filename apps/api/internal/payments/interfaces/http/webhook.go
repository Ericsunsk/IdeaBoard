package http

import (
	"io"
	"net/http"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

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
