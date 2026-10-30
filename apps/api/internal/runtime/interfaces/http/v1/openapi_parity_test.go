package v1

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
)

func TestPublicRouterOpenAPIParity(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "..", "openapi.public.yaml"))
	if err != nil {
		t.Fatalf("load openapi.public.yaml: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterContractV1RoutesOnEngine(router, runtimehttp.AuthDeps{}, ContractV1Deps{})
	RegisterContractV1BillingRoutesOnEngine(router, runtimehttp.AuthDeps{}, ContractV1BillingDeps{})

	// OpenAPI operations: set of "METHOD /api/v1/normalized/path"
	openAPIRoutes := map[string]string{}
	paramRegex := regexp.MustCompile(`\{([^}]+)\}`)

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			normalizedPath := paramRegex.ReplaceAllString(path, ":$1")
			fullPath := runtimehttp.ContractV1GroupPath + normalizedPath
			key := strings.ToUpper(method) + " " + fullPath
			opID := ""
			if op != nil {
				opID = op.OperationID
			}
			openAPIRoutes[key] = opID
		}
	}

	// Gin registered routes
	ginRoutes := map[string]bool{}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		ginRoutes[key] = true
	}

	var missingInGin, missingInOpenAPI []string
	for routeKey, opID := range openAPIRoutes {
		if !ginRoutes[routeKey] {
			missingInGin = append(missingInGin, routeKey+" (operationId: "+opID+")")
		}
	}
	for routeKey := range ginRoutes {
		if _, ok := openAPIRoutes[routeKey]; !ok {
			missingInOpenAPI = append(missingInOpenAPI, routeKey)
		}
	}

	// Documented protocol exceptions:
	// - The 2 SSE streaming endpoints (streamWorkspaceEvents, streamUserNotificationEvents)
	//   and the 3 workspace webhook subscription management endpoints are documented by
	//   openapi.events.yaml / ADR-0009, not openapi.public.yaml (which is request/response JSON).
	allowedNonPublicSpecRoutes := map[string]string{
		"GET /api/v1/user/notifications/events":                                        "ADR-0009 / openapi.events.yaml SSE stream",
		"GET /api/v1/workspaces/:workspace_id/events":                                  "ADR-0009 / openapi.events.yaml SSE stream",
		"GET /api/v1/workspaces/:workspace_id/event-subscriptions":                     "openapi.events.yaml webhook subscriptions",
		"POST /api/v1/workspaces/:workspace_id/event-subscriptions":                    "openapi.events.yaml webhook subscriptions",
		"DELETE /api/v1/workspaces/:workspace_id/event-subscriptions/:subscription_id": "openapi.events.yaml webhook subscriptions",
	}

	var unexpectedMissingInOpenAPI []string
	for _, routeKey := range missingInOpenAPI {
		if _, ok := allowedNonPublicSpecRoutes[routeKey]; !ok {
			unexpectedMissingInOpenAPI = append(unexpectedMissingInOpenAPI, routeKey)
		}
	}

	sort.Strings(missingInGin)
	sort.Strings(unexpectedMissingInOpenAPI)

	if len(missingInGin) > 0 {
		t.Fatalf("OpenAPI routes missing in Gin router (contract kernel violation):\n%s", strings.Join(missingInGin, "\n"))
	}
	if len(unexpectedMissingInOpenAPI) > 0 {
		t.Fatalf("Gin routes missing in OpenAPI spec (undocumented public endpoint violation):\n%s", strings.Join(unexpectedMissingInOpenAPI, "\n"))
	}
}
