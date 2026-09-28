package http

import (
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
)

const (
	maxStripeWebhookRequestBytes int64 = 128 << 10
)

type WebhookHandler struct {
	service paymentcontracts.WebhookService
}

func NewWebhookHandler(service paymentcontracts.WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}
