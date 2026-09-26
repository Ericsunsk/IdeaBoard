package httpapi

// Contract inventory gate for issue #800 (API Contract Kernel), flipped to
// legacy-ZERO by the #809 hard cut.
//
// Scope: assert that every route registered by RegisterRoutes lands in
// exactly one documentation bucket {public, control, auth, infra}, and that
// NO legacy /api/user or /api/admin route is registered anymore. There are
// no compatibility aliases, no dual-read/dual-write, and no x-legacy
// bucket: any legacy path is a hard CI failure.
//
// Notes on deliberate non-bans (verified live, not legacy):
//   - platform/response + message_safe: the shared error envelope used by
//     every live handler (v1, control, health, static, webhook).
//   - principal.RequireAdmin + auth.AdminMiddleware: still referenced by
//     domain admin handlers (mail/social providers, billing, credits,
//     payments, search catalog) whose methods the Control Plane proxies
//     under its capability guard, plus their in-package unit tests.
//   - "/api/user..." string literals inside domain unit tests (auth,
//     credits, storage, tasking, metrics, cors): opaque test URLs for
//     middleware/handler units, not registered routes. This gate asserts
//     on the registered router, never on source text.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/settings"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"

	"github.com/gin-gonic/gin"
)

// contractInventoryBucket is one documentation home for a registered route.
type contractInventoryBucket string

const (
	bucketPublic  contractInventoryBucket = "public"  // new /api/v1 + legacy public paths
	bucketControl contractInventoryBucket = "control" // new /api/control/v1
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
		PublicSearchHandler: &searchhttp.SearchHandler{},
		PaymentWebhook:      nil,
		Internal:            InternalRouteDeps{},
		CORSMiddleware:      func(c *gin.Context) { c.Next() },
		RateLimit:           inventoryRateLimitStub{},
	}, nil, FrontendRouteOptions{})
	return router
}

// classifyContractRoute maps a registered path to exactly one bucket.
// Legacy /api/user and /api/admin paths are unclassified on purpose:
// they must not be registered anymore (see the legacy-zero test below).
// Frontend SPA routes are not registered when frontendFS is nil.
func classifyContractRoute(path string) (contractInventoryBucket, bool) {
	switch {
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

// isLegacyCutPath reports the deleted legacy groups. Any registered route
// under them is a hard-cut violation, not an undocumented endpoint.
func isLegacyCutPath(path string) bool {
	return path == "/api/user" || strings.HasPrefix(path, "/api/user/") ||
		path == "/api/admin" || strings.HasPrefix(path, "/api/admin/")
}

func TestContractInventoryEveryRouteHasExactlyOneBucket(t *testing.T) {
	router := newContractInventoryRouter()
	counts := map[contractInventoryBucket]int{}
	var legacy, unclassified []string
	for _, route := range router.Routes() {
		if isLegacyCutPath(route.Path) {
			legacy = append(legacy, route.Method+" "+route.Path)
			continue
		}
		bucket, ok := classifyContractRoute(route.Path)
		if !ok {
			unclassified = append(unclassified, route.Method+" "+route.Path)
			continue
		}
		counts[bucket]++
	}
	if len(legacy) > 0 {
		sort.Strings(legacy)
		t.Fatalf("legacy /api/user and /api/admin routes must stay deleted post-#809 hard cut:\n%s",
			strings.Join(legacy, "\n"))
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("routes without a documentation bucket (CI must fail on undocumented endpoints):\n%s",
			strings.Join(unclassified, "\n"))
	}
	t.Logf("route inventory buckets: public=%d control=%d auth=%d infra=%d (legacy=0)",
		counts[bucketPublic], counts[bucketControl], counts[bucketAuth], counts[bucketInfra])
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
			// Backend gap batch B (#809-close): purchase options,
			// pending orders (continue/abandon), recharge history,
			// billing quotes, and rewards redeem/history.
			"operationId: getWorkspacePurchaseOptions",
			"operationId: listWorkspacePendingOrders",
			"operationId: continueWorkspacePendingOrder",
			"operationId: abandonWorkspacePendingOrder",
			"operationId: listWorkspaceRecharges",
			"operationId: createWorkspaceBillingQuote",
			"operationId: redeemWorkspaceReward",
			"operationId: listWorkspaceRewardRedemptions",
			// Media gap batch: staged-bytes PUT, bounded batch read,
			// user-scoped avatar PUT (complete/vision branches covered
			// by the pinned completeWorkspaceUpload /
			// createWorkspaceVisionQuery operations).
			"operationId: stageWorkspaceUploadBytes",
			"operationId: batchGetWorkspaceMediaAssets",
			"operationId: uploadAvatar",
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
			// Control gap batch (#809-close): OAuth provider channels,
			// branding logo upload, AI gateway extras (detect, model
			// reference, brand logo, observed, route preview, test
			// connection), brand-logo overrides, reward-code
			// administration plus admin redemption reads, and the
			// monitor SSE stream.
			"operationId: getControlOAuthProvider",
			"operationId: updateControlOAuthProvider",
			"operationId: uploadControlBrandingLogo",
			"operationId: detectControlAIGatewayModels",
			"operationId: getControlAIGatewayModelReference",
			"operationId: getControlAIGatewayBrandLogo",
			"operationId: listControlAIGatewayModelsObserved",
			"operationId: previewControlAIGatewayRoute",
			"operationId: testControlAIGatewayConnection",
			"operationId: listControlBrandLogoOverrides",
			"operationId: saveControlBrandLogoOverride",
			"operationId: deleteControlBrandLogoOverride",
			"operationId: searchControlRewardCodes",
			"operationId: createControlRewardCode",
			"operationId: updateControlRewardCode",
			"operationId: setControlRewardCodeEnabled",
			"operationId: deleteControlRewardCode",
			"operationId: listControlRewardRedemptions",
			"operationId: getControlRewardRedemption",
			"operationId: streamControlMonitor",
		},
		"openapi.events.yaml": {
			"pattern: '^evt_",
			"task.completed",
			"Last-Event-ID",
			"svix-signature",
		},
		// Legacy openapi.yaml was deleted by the #809 hard cut; its absence
		// is asserted by TestContractInventoryLegacySurfacesStayDeleted.
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

// TestContractInventoryLegacySurfacesStayDeleted pins the #809 hard cut:
// the legacy route files, the fail-closed task-store stub, and the legacy
// spec must stay deleted. (Error-code / event-type catalogs and
// generated-client drift stay advisory warnings below, not hard gates.)
func TestContractInventoryLegacySurfacesStayDeleted(t *testing.T) {
	// The legacy spec was deleted by the #809 hard cut: fail if it returns.
	legacySpec := filepath.Join("..", "..", "..", "..", "openapi.yaml")
	if _, err := os.Stat(legacySpec); err == nil {
		t.Fatalf("legacy openapi.yaml must stay deleted post-#809 hard cut")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacy openapi.yaml: %v", err)
	}

	deleted := []string{
		// Legacy route groups + handlers that existed only to serve them.
		"routes_user.go",
		"routes_admin.go",
		"user_events.go",
		"upload_handler.go",
		"download_handler.go",
		"media_asset_handler.go",
		// Fail-closed legacy task-store stub (+ its pin test): the
		// workspace-scoped store replaced every seam it covered.
		filepath.Join("..", "composition", "legacy_task_store_stub.go"),
		filepath.Join("..", "composition", "legacy_task_store_stub_test.go"),
	}
	for _, name := range deleted {
		if _, err := os.Stat(name); err == nil {
			t.Fatalf("legacy file %s must stay deleted post-#809 hard cut", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}

	router := newContractInventoryRouter()
	counts := map[contractInventoryBucket]int{}
	for _, route := range router.Routes() {
		if isLegacyCutPath(route.Path) {
			t.Fatalf("registered legacy route %s %s must stay deleted post-#809 hard cut", route.Method, route.Path)
		}
		if bucket, ok := classifyContractRoute(route.Path); ok {
			counts[bucket]++
		}
	}
	t.Logf("WARNING (placeholder): error-code and event-type catalog checks land with #801-808; " +
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
