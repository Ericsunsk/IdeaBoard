package control

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

func TestControlRouterOpenAPIParity(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "..", "openapi.control.yaml"))
	if err != nil {
		t.Fatalf("load openapi.control.yaml: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterContractControlRoutesOnEngine(router, runtimehttp.AuthDeps{}, ContractControlDeps{})

	openAPIRoutes := map[string]string{}
	paramRegex := regexp.MustCompile(`\{([^}]+)\}`)

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			normalizedPath := paramRegex.ReplaceAllString(path, ":$1")
			fullPath := runtimehttp.ContractControlGroupPath + normalizedPath
			key := strings.ToUpper(method) + " " + fullPath
			opID := ""
			if op != nil {
				opID = op.OperationID
			}
			openAPIRoutes[key] = opID
		}
	}

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

	sort.Strings(missingInGin)
	sort.Strings(missingInOpenAPI)

	t.Logf("Total OpenAPI routes: %d, Total Gin routes: %d", len(openAPIRoutes), len(ginRoutes))
	if len(missingInGin) > 0 {
		t.Errorf("Missing in Gin router:\n%s", strings.Join(missingInGin, "\n"))
	}
	if len(missingInOpenAPI) > 0 {
		t.Errorf("Missing in OpenAPI spec:\n%s", strings.Join(missingInOpenAPI, "\n"))
	}
}
