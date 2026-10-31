package control

// Control proxy envelope unification tests (parent #809, Baseline §12).
//
// Every controlProxy route must render the platform/contract envelope:
// success bodies carry exactly {data} (never the legacy top-level
// code/msg/message_safe), error bodies carry exactly
// {error:{code,message,request_id}} at the unchanged HTTP status with
// the stable per-family code. The binary brand-logo and the SSE monitor
// stream are protocol exceptions and bypass translation.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	storagedomain "ideaboard/api/internal/media/storage/domain"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	modelregistrydomain "ideaboard/api/internal/modelregistry/domain"
	modelregistryhttp "ideaboard/api/internal/modelregistry/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/response"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

// controlProxyTestContext runs method through the translator on a fresh
// test context and returns the recorded status + raw body.
func controlProxyTestContext(t *testing.T, method func(*gin.Context)) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/control/v1/probe", nil)
	c.Set(contract.RequestIDHeader, "test-proxy-req-1")
	c.Set(response.RequestIDHeader, "test-proxy-req-1")
	translateControlProxyResponse(c, method)
	return recorder.Code, recorder.Body.String()
}

func TestControlProxySuccessUnwrapsLegacyData(t *testing.T) {
	status, body := controlProxyTestContext(t, func(c *gin.Context) {
		response.SuccessDTO(c, map[string]string{"hello": "world"})
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if strings.Contains(body, `"msg"`) || strings.Contains(body, `"message_safe"`) || strings.Contains(body, `"code":0`) {
		t.Fatalf("body must not carry legacy top-level keys: %s", body)
	}
	if !strings.Contains(body, `"data":{"hello":"world"}`) {
		t.Fatalf("body must carry unwrapped data: %s", body)
	}
}

func TestControlProxyNilAckRendersEmptyObject(t *testing.T) {
	status, body := controlProxyTestContext(t, func(c *gin.Context) {
		response.Success(c, nil)
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, `"data":{}`) {
		t.Fatalf("nil-ack body = %s, want {\"data\":{}}", body)
	}
}

func TestControlProxyMachineErrorKeepsFamilyCode(t *testing.T) {
	status, body := controlProxyTestContext(t, func(c *gin.Context) {
		response.MachineError(c, http.StatusBadRequest, "reward_invalid_input", "参数错误")
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	for _, want := range []string{`"error"`, `"code":"reward_invalid_input"`, `"message":"参数错误"`, `"request_id":"test-proxy-req-1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %s", body, want)
		}
	}
	if strings.Contains(body, `"message_safe"`) || strings.Contains(body, `"msg"`) {
		t.Fatalf("body must not carry legacy keys: %s", body)
	}
}

func TestControlProxyNumericErrorMapsByStatus(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, string(contract.CodeBadRequest)},
		{http.StatusForbidden, string(contract.CodeForbidden)},
		{http.StatusNotFound, string(contract.CodeControlNotFound)},
		{http.StatusConflict, string(contract.CodeConflict)},
		{http.StatusInternalServerError, string(contract.CodeServiceUnavailable)},
		{http.StatusServiceUnavailable, string(contract.CodeServiceUnavailable)},
	}
	for _, tc := range cases {
		status, body := controlProxyTestContext(t, func(c *gin.Context) {
			response.Error(c, tc.status, "安全文案")
		})
		if status != tc.status {
			t.Fatalf("status = %d, want %d", status, tc.status)
		}
		if !strings.Contains(body, `"code":"`+tc.code+`"`) {
			t.Fatalf("status %d body = %s, want code %q", tc.status, body, tc.code)
		}
		if !strings.Contains(body, `"message":"安全文案"`) || !strings.Contains(body, `"request_id":"test-proxy-req-1"`) {
			t.Fatalf("status %d body = %s, want legacy message + request_id", tc.status, body)
		}
	}
}

func TestControlProxyNonJSONReplaysRaw(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	status, body := controlProxyTestContext(t, func(c *gin.Context) {
		c.Data(http.StatusOK, "image/svg+xml", []byte(svg))
	})
	if status != http.StatusOK || body != svg {
		t.Fatalf("status = %d body = %q, want 200 raw SVG", status, body)
	}
}

func TestControlProxyEmptyCaptureFailsClosed(t *testing.T) {
	status, body := controlProxyTestContext(t, func(c *gin.Context) {})
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if !strings.Contains(body, `"code":"`+string(contract.CodeServiceUnavailable)+`"`) {
		t.Fatalf("body = %s, want service_unavailable", body)
	}
}

// --- integration: proxied families render the contract envelope ---

type proxyEnvelopeFakeGatewayStore struct{}

func (proxyEnvelopeFakeGatewayStore) GetSummary(context.Context) (modelregistrydomain.GatewaySummary, error) {
	return modelregistrydomain.GatewaySummary{}, nil
}

func (proxyEnvelopeFakeGatewayStore) Update(context.Context, modelregistrydomain.UpdateInput, int64) error {
	return nil
}

func (proxyEnvelopeFakeGatewayStore) ListCatalog(context.Context) ([]modelregistrydomain.CatalogItem, error) {
	return nil, nil
}

func (proxyEnvelopeFakeGatewayStore) SaveCatalog(context.Context, []modelregistrydomain.CatalogItem, int64) error {
	return nil
}

func (proxyEnvelopeFakeGatewayStore) ListConfiguredProviders(context.Context) ([]workflow.ProviderEntry, error) {
	return nil, nil
}

func (proxyEnvelopeFakeGatewayStore) Resolve(context.Context) (modelregistrydomain.ResolvedGateway, error) {
	return modelregistrydomain.ResolvedGateway{}, modelregistrydomain.ErrAIGatewayNotConfigured
}

func (proxyEnvelopeFakeGatewayStore) ResolveVisionModel(context.Context) (string, error) {
	return "", modelregistrydomain.ErrVisionModelMissing
}

type proxyEnvelopeFakeStorage struct{}

func (proxyEnvelopeFakeStorage) ListProviderMeta() []storagedomain.StorageProviderMeta {
	return []storagedomain.StorageProviderMeta{}
}

func (proxyEnvelopeFakeStorage) ListProfiles(context.Context) ([]storagedomain.StorageProfile, error) {
	return []storagedomain.StorageProfile{}, nil
}

func (proxyEnvelopeFakeStorage) SaveProfile(_ context.Context, profile *storagedomain.StorageProfile) (*storagedomain.StorageProfile, error) {
	return profile, nil
}

func (proxyEnvelopeFakeStorage) SaveAndApplyProfile(_ context.Context, profile *storagedomain.StorageProfile) (*storagedomain.StorageProfile, error) {
	return profile, nil
}

func (proxyEnvelopeFakeStorage) DeleteProfile(context.Context, string) error { return nil }

func (proxyEnvelopeFakeStorage) ApplyProfile(_ context.Context, id string) (*storagedomain.StorageProfile, error) {
	return &storagedomain.StorageProfile{}, nil
}

func (proxyEnvelopeFakeStorage) TestProfile(context.Context, string) error { return nil }

func (proxyEnvelopeFakeStorage) GetBucketInfo(context.Context, string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

type proxyEnvelopeFakeBrandLogos struct{}

func (proxyEnvelopeFakeBrandLogos) List(context.Context) ([]adminbranding.Override, error) {
	return []adminbranding.Override{}, nil
}

func (proxyEnvelopeFakeBrandLogos) Upsert(_ context.Context, scope adminbranding.Scope, targetKey, logoKey string) (adminbranding.Override, error) {
	return adminbranding.Override{Scope: scope, TargetKey: targetKey, LogoKey: logoKey}, nil
}

func (proxyEnvelopeFakeBrandLogos) Delete(context.Context, adminbranding.Scope, string) (bool, error) {
	return true, nil
}

func assertControlProxyContractShape(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	if _, ok := payload["data"]; !ok {
		t.Fatalf("%s: success envelope requires data: %v", path, payload)
	}
	for _, legacy := range []string{"code", "msg", "message_safe"} {
		if _, ok := payload[legacy]; ok {
			t.Fatalf("%s: body must not carry legacy key %q: %v", path, legacy, payload)
		}
	}
	if _, hasError := payload["error"]; hasError {
		t.Fatalf("%s: success envelope must not carry error: %v", path, payload)
	}
}

func TestControlProxyFamiliesRenderContractEnvelope(t *testing.T) {
	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.AIGateway = modelregistryhttp.NewHandler(proxyEnvelopeFakeGatewayStore{})
		deps.Storage = storagehttp.NewHandler(proxyEnvelopeFakeStorage{})
		deps.BrandLogos = adminbranding.NewHandler(proxyEnvelopeFakeBrandLogos{}, func(*gin.Context) bool { return true })
	})
	admin := controlCall{userID: controlOperatorID, role: "admin"}
	reads := []string{
		"/api/control/v1/platform/ai-gateway",
		"/api/control/v1/platform/ai-gateway/catalog",
		"/api/control/v1/platform/ai-gateway/providers",
		"/api/control/v1/platform/storage/providers",
		"/api/control/v1/platform/storage/profiles",
		"/api/control/v1/platform/brand-logo-overrides",
	}
	for _, path := range reads {
		call := admin
		call.method = http.MethodGet
		call.path = path
		status, _, payload := h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("GET %s status = %d (%v), want 200", path, status, payload)
		}
		assertControlProxyContractShape(t, path, payload)
	}
}

func TestControlProxyRewardsErrorsRenderContractEnvelope(t *testing.T) {
	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.RewardReads = rewardhttp.NewAdminReadHandler(fakeRewardAdminReads{}, fakeRewardIdentityProjection{})
	})
	call := controlCall{
		method: http.MethodPost, path: "/api/control/v1/rewards/codes/search",
		body: `{"query":"x"}`, userID: controlOperatorID, role: "admin",
	}
	// Query strings are rejected by the legacy handler (400
	// reward_invalid_input); the control boundary must reshape it into
	// the contract error envelope at the same status with the same code.
	call.path = "/api/control/v1/rewards/codes/search?query=x"
	status, _, payload := h.serve(call)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := controlErrorCode(t, payload); code != "reward_invalid_input" {
		t.Fatalf("code = %q, want reward_invalid_input", code)
	}
}
