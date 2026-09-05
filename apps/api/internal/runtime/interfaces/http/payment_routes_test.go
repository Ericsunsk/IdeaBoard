package httpapi

import (
	"testing"

	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/config"

	"github.com/gin-gonic/gin"
)

func TestAdminPaymentRoutesExposeStripeConfigurationAndOrderReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAdminPaymentRoutes(router.Group("/api/admin"), AdminRouteDeps{
		PaymentAdminHandler: paymenthttp.NewAdminHandler(nil, config.PublicOriginsConfig{}),
	})
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		"GET /api/admin/payments/stripe-config",
		"PUT /api/admin/payments/stripe-config",
		"GET /api/admin/payments/orders",
		"GET /api/admin/payments/orders/:order_number",
	} {
		if _, exists := routes[expected]; !exists {
			t.Fatalf("route %q is not registered: %#v", expected, routes)
		}
	}
}
