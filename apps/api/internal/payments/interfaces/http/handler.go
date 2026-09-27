package http

import (
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/config"
)

const (
	maxStripeConfigurationRequestBytes int64 = 16 << 10
	maxStripeWebhookRequestBytes       int64 = 128 << 10
	maxPaymentCheckoutRequestBytes     int64 = 16 << 10
	maxPaymentRefundRequestBytes       int64 = 16 << 10
)

type UserCheckoutHandler struct {
	service       paymentcontracts.UserCheckoutService
	publicOrigins config.PublicOriginsConfig
}

type AdminHandler struct {
	service       paymentcontracts.AdminService
	publicOrigins config.PublicOriginsConfig
}

type WebhookHandler struct {
	service paymentcontracts.WebhookService
}

func NewWebhookHandler(service paymentcontracts.WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}
