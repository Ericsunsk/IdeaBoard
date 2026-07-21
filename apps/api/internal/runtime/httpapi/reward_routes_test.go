package httpapi

import (
	"testing"

	"ideaboard/api/internal/rewards"

	"github.com/gin-gonic/gin"
)

func TestAdminRewardRoutesExposeOnlyCanonicalRewardCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAdminRewardRoutes(router.Group("/api/admin"), AdminRouteDeps{
		RewardHandler: rewards.NewHandler(nil),
	})
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		"GET /api/admin/rewards/codes",
		"POST /api/admin/rewards/codes",
		"PATCH /api/admin/rewards/codes/:id",
		"PATCH /api/admin/rewards/codes/:id/enabled",
		"DELETE /api/admin/rewards/codes/:id",
	} {
		if _, exists := routes[expected]; !exists {
			t.Fatalf("route %q is not registered: %#v", expected, routes)
		}
	}
	if len(routes) != 5 {
		t.Fatalf("unexpected legacy Reward routes remain registered: %#v", routes)
	}
}
