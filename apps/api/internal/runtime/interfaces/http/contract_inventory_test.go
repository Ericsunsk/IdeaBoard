package httpapi

// Contract inventory gate for issue #800 (API Contract Kernel).
//
// Scope: assert that every route registered by RegisterRoutes lands in
// exactly one documentation bucket {public, control, auth, infra}, while
// legacy /api/user and /api/admin routes are still present but only flagged
// x-legacy. Hard failure on legacy routes is deferred to #809, so this file
// logs warnings (undocumented endpoints, error codes, event types,
// generated-client drift) instead of failing CI while #801-808 fill in the
// skeleton contracts (openapi.public.yaml, openapi.control.yaml,
// openapi.events.yaml).

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ideaboard/api/internal/aigateway"
	billinghttp "ideaboard/api/internal/billing/interfaces/http"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/media/storage"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/rewards"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	taskhttp "ideaboard/api/internal/tasking/interfaces/http/task"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

// contractInventoryBucket is one documentation home for a registered route.
type contractInventoryBucket string

const (
	bucketLegacy  contractInventoryBucket = "legacy"  // /api/user + /api/admin, x-legacy until #809
	bucketPublic  contractInventoryBucket = "public"  // legacy public paths + new /api/v1 stubs
	bucketControl contractInventoryBucket = "control" // new /api/control/v1 stubs
	bucketAuth    contractInventoryBucket = "auth"    // Better Auth transport, owned by apps/auth
	bucketInfra   contractInventoryBucket = "infra"   // health/readiness, webhooks, internal, static bytes
)

type inventoryRateLimitStub struct{}

func (inventoryRateLimitStub) GetRateLimit(context.Context) (settings.RateLimitSetting, error) {
	return settings.RateLimitSetting{}, nil
}

// newContractInventoryRouter registers the full production route set with
// zero-value handler dependencies. Registration only captures method values
// and middleware closures, so it never touches a database or network.
func newContractInventoryRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, Dependencies{
		UserRoutes: UserRouteDeps{
			AuthHandler:            &auth.AuthHandler{},
			TaskHandler:            &taskhttp.Handler{},
			BoardHandler:           &boards.Handler{},
			BillingHandler:         &billinghttp.Handler{},
			BillingRecords:         &billingrecords.Handler{},
			CreditUserHandler:      &credithttp.UserHandler{},
			UsageHandler:           &usage.Handler{},
			RewardUserHandler:      &rewards.UserHandler{},
			PaymentUserHandler:     &paymenthttp.UserCheckoutHandler{},
			UserEventStreamHandler: &UserEventStreamHandler{},
			VisionHandler:          &searchhttp.VisionHandler{},
		},
		PublicSearchHandler: &searchhttp.SearchHandler{},
		AdminRoutes: AdminRouteDeps{
			AdminHandler:                &identityhttp.AdminHandler{},
			MailConfigHandler:           &auth.MailConfigAdminHandler{},
			SocialProviderHandler:       &auth.SocialProviderAdminHandler{},
			AIGatewayHandler:            &aigateway.Handler{},
			BillingHandler:              &billinghttp.Handler{},
			CreditAdminCommandHandler:   &credithttp.AdminCommandHandler{},
			CreditAdminReadHandler:      &credithttp.AdminReadHandler{},
			CreditReconciliationHandler: &credithttp.ReconciliationHandler{},
			TaskHandler:                 &taskhttp.Handler{},
			TaskStatsHandler:            &statisticshttp.Handler{},
			StorageHandler:              &storagehttp.Handler{},
			FeaturePromptHandler:        &prompts.FeaturePromptHandler{},
			StorageService:              &storage.Service{},
			RewardAdminCommandHandler:   &rewards.AdminCommandHandler{},
			RewardAdminReadHandler:      &rewards.AdminReadHandler{},
			PaymentAdminHandler:         &paymenthttp.AdminHandler{},
			SearchCatalogHandler:        &searchhttp.Handler{},
			AdminBrandLogoHandler:       &adminbranding.Handler{},
		},
		PaymentWebhook: &paymenthttp.WebhookHandler{},
		Internal:       InternalRouteDeps{UserHandler: &identityhttp.InternalHandler{}},
		CORSMiddleware: func(c *gin.Context) { c.Next() },
		RateLimit:      inventoryRateLimitStub{},
	}, nil, FrontendRouteOptions{})
	return router
}

// classifyContractRoute maps a registered path to exactly one bucket.
// Frontend SPA routes are not registered when frontendFS is nil.
func classifyContractRoute(path string) (contractInventoryBucket, bool) {
	switch {
	case path == "/api/user" || strings.HasPrefix(path, "/api/user/"):
		return bucketLegacy, true
	case path == "/api/admin" || strings.HasPrefix(path, "/api/admin/"):
		return bucketLegacy, true
	case path == ContractControlGroupPath || strings.HasPrefix(path, ContractControlGroupPath+"/"):
		return bucketControl, true
	case path == ContractV1GroupPath || strings.HasPrefix(path, ContractV1GroupPath+"/"):
		return bucketPublic, true
	case path == "/api/health" || path == "/api/livez" || path == "/api/readyz" ||
		path == "/api/settings/branding" || path == "/api/search":
		return bucketPublic, true
	case path == "/api/auth" || strings.HasPrefix(path, "/api/auth/"):
		return bucketAuth, true
	case path == "/api/webhooks" || strings.HasPrefix(path, "/api/webhooks/") ||
		path == "/internal" || strings.HasPrefix(path, "/internal/") ||
		path == "/uploads" || strings.HasPrefix(path, "/uploads/"):
		return bucketInfra, true
	case path == "/":
		return bucketInfra, true
	default:
		return "", false
	}
}

func TestContractInventoryEveryRouteHasExactlyOneBucket(t *testing.T) {
	router := newContractInventoryRouter()
	counts := map[contractInventoryBucket]int{}
	var legacy, unclassified []string
	for _, route := range router.Routes() {
		bucket, ok := classifyContractRoute(route.Path)
		if !ok {
			unclassified = append(unclassified, route.Method+" "+route.Path)
			continue
		}
		counts[bucket]++
		if bucket == bucketLegacy {
			legacy = append(legacy, route.Method+" "+route.Path)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("routes without a documentation bucket (CI must fail on undocumented endpoints):\n%s",
			strings.Join(unclassified, "\n"))
	}
	if counts[bucketLegacy] == 0 {
		t.Fatal("expected legacy /api/user and /api/admin routes to still be registered until #809")
	}
	sort.Strings(legacy)
	t.Logf("route inventory buckets: public=%d control=%d legacy=%d auth=%d infra=%d",
		counts[bucketPublic], counts[bucketControl], counts[bucketLegacy], counts[bucketAuth], counts[bucketInfra])
	t.Logf("legacy routes still present (x-legacy: true, hard fail deferred to #809): %d routes, e.g.\n%s",
		len(legacy), strings.Join(legacy[:min(5, len(legacy))], "\n"))
}

func TestContractInventorySkeletonContractsExist(t *testing.T) {
	// Skeleton contracts must exist with their kernel markers. Full resource
	// migration happens in #801-808; here we only pin the kernel shape.
	want := map[string][]string{
		"openapi.public.yaml": {
			"url: /api/v1",
			"Idempotency-Key",
			"Request-Id",
			"operationId: getMe",
			"operationId: listWorkspaces",
			"operationId: createWorkspace",
			"operationId: listBoards",
			"operationId: createBoard",
			"operationId: joinBoardByToken",
			"operationId: getBoardDocument",
			"operationId: replaceBoardDocument",
			// #820/#821 (#809 Phase 4-03/4-04): workspace upload/media
			// surface restored (contract_v1 upload/media routes live);
			// the 8 upload/media operationIds are pinned here.
			"operationId: createWorkspaceUpload",
			"operationId: getWorkspaceUpload",
			"operationId: completeWorkspaceUpload",
			"operationId: listWorkspaceMediaAssets",
			"operationId: getWorkspaceMediaAsset",
			"operationId: contentWorkspaceMediaAsset",
			"operationId: derivativeWorkspaceMediaAsset",
			"operationId: archiveWorkspaceMediaAsset",
			"operationId: getWorkspaceBillingAccount",
			"operationId: getWorkspaceCreditsBalance",
			"operationId: createWorkspaceCheckoutSession",
			"operationId: createWorkspaceTask",
			"operationId: listWorkspaceTasks",
			"operationId: getWorkspaceTask",
			"operationId: cancelWorkspaceTask",
			"operationId: listWorkspaceTaskOutputs",
			"operationId: quoteWorkspaceTask",
			"operationId: listWorkspaceLibraryFolders",
			"operationId: createWorkspaceLibraryFolder",
			"operationId: renameWorkspaceLibraryFolder",
			"operationId: deleteWorkspaceLibraryFolder",
			"operationId: listWorkspaceLibraryAssets",
			"operationId: saveWorkspaceLibraryAsset",
			"operationId: moveWorkspaceLibraryAsset",
			"operationId: deleteWorkspaceLibraryAsset",
			"operationId: resolveWorkspaceLibraryAssets",
			"operationId: createWorkspaceLibraryMaterialization",
			"operationId: getWorkspaceLibraryMaterialization",
			"operationId: searchWorkspace",
			"operationId: createWorkspaceVisionQuery",
		},
		"openapi.control.yaml": {
			"url: /api/control/v1",
			"x-audit:",
			"x-capability:",
			"operationId: listControlUsers",
			"operationId: impersonateControlUser",
			"operationId: listControlBillingAccounts",
			"operationId: createControlCreditGrant",
			"operationId: createControlCreditDeduction",
			"operationId: createControlCreditRefund",
			"operationId: createControlCreditCorrection",
			"operationId: cancelControlTask",
			"operationId: getControlMonitor",
			"operationId: getControlMailSetting",
			"operationId: updateControlMailSetting",
		},
		"openapi.events.yaml": {
			"pattern: '^evt_",
			"task.completed",
			"Last-Event-ID",
			"X-Webhook-Signature",
		},
		// Legacy openapi.yaml was deleted by the #809 hard cut; its absence
		// is asserted by TestContractInventoryLegacyMarkersStayDeleted and
		// TestContractInventoryLegacyGrepWarning below, not by markers here.
	}
	files := make([]string, 0, len(want))
	for name := range want {
		files = append(files, name)
	}
	sort.Strings(files)
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, marker := range want[name] {
			if !strings.Contains(string(raw), marker) {
				t.Errorf("%s must contain kernel marker %q (contract kernel regression)", name, marker)
			}
		}
	}
}

// TestContractInventoryLegacyGrepWarning lists legacy references, undocumented
// endpoints/error codes/event types and generated-client drift as warnings.
// Post-#809 the legacy openapi.yaml file itself must be gone (hard failure);
// the remaining route-catalog warnings stay advisory.
func TestContractInventoryLegacyGrepWarning(t *testing.T) {
	// The legacy spec was deleted by the #809 hard cut: fail if it returns.
	legacySpec := filepath.Join("..", "..", "..", "..", "openapi.yaml")
	if _, err := os.Stat(legacySpec); err == nil {
		t.Fatalf("legacy openapi.yaml must stay deleted post-#809 hard cut")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacy openapi.yaml: %v", err)
	}

	router := newContractInventoryRouter()
	var undocumented []string
	for _, route := range router.Routes() {
		bucket, ok := classifyContractRoute(route.Path)
		if !ok || bucket == bucketLegacy {
			undocumented = append(undocumented, route.Method+" "+route.Path)
		}
	}
	sort.Strings(undocumented)
	t.Logf("WARNING (filled in by #801-808): %d registered routes not yet in the new Public/Control/Event "+
		"skeleton contracts", len(undocumented))

	t.Log("WARNING (placeholder): error-code and event-type catalog checks land with #801-808; " +
		"current known codes include task_idempotency_conflict, search_unavailable; " +
		"current known event types include task.completed, task.failed, board.member_invited")

	generatedDir := filepath.Join("..", "..", "..", "..", "..", "..", "apps", "web", "src", "shared", "api", "generated-query")
	if _, err := os.Stat(generatedDir); err != nil {
		t.Logf("WARNING (placeholder): generated client dir not inspected from this package (%v); "+
			"generated drift enforcement lands with #801-808 and must never hand-edit generated files", err)
	} else {
		t.Log("WARNING (placeholder): generated-client drift check is a warning until #801-808 regenerate " +
			"Public/Control clients from the new contracts")
	}
}
