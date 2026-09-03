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

const (
	maxAdminPaymentOrderLimit = 100
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

func NewUserCheckoutHandler(service paymentcontracts.UserCheckoutService, publicOrigins config.PublicOriginsConfig) *UserCheckoutHandler {
	return &UserCheckoutHandler{service: service, publicOrigins: publicOrigins}
}

func NewAdminHandler(service paymentcontracts.AdminService, publicOrigins config.PublicOriginsConfig) *AdminHandler {
	return &AdminHandler{service: service, publicOrigins: publicOrigins}
}

func NewWebhookHandler(service paymentcontracts.WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}
