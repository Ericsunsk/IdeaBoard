package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type fakeGatewayStore struct {
	summary  GatewaySummary
	updated  []UpdateInput
	actors   []int64
	getErr   error
	setErr   error
	mappings map[string]string
	saved    []savedMapping
	saveErr  error
	catalog  []CatalogItem
	detected []DetectedModel
}

type savedMapping struct {
	feature string
	model   string
	actor   int64
}

func (f *fakeGatewayStore) GetSummary(context.Context) (GatewaySummary, error) {
	return f.summary, f.getErr
}

func (f *fakeGatewayStore) Update(_ context.Context, input UpdateInput, actorID int64) error {
	f.updated = append(f.updated, input)
	f.actors = append(f.actors, actorID)
	return f.setErr
}

func (f *fakeGatewayStore) ListModelMappings(context.Context) (map[string]string, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	out := make(map[string]string, len(f.mappings))
	for feature, model := range f.mappings {
		out[feature] = model
	}
	return out, nil
}

func (f *fakeGatewayStore) SaveModelMapping(_ context.Context, feature, model string, actorID int64) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.mappings == nil {
		f.mappings = map[string]string{}
	}
	f.mappings[feature] = model
	f.saved = append(f.saved, savedMapping{feature: feature, model: model, actor: actorID})
	return nil
}

func (f *fakeGatewayStore) ListCatalog(context.Context) ([]CatalogItem, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return f.catalog, nil
}

func (f *fakeGatewayStore) SaveCatalog(_ context.Context, items []CatalogItem, _ int64) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.catalog = items
	return nil
}

func (f *fakeGatewayStore) DetectModels(_ context.Context, provider string) (ModelDetection, error) {
	if f.saveErr != nil {
		return ModelDetection{}, f.saveErr
	}
	return ModelDetection{Models: f.detected, ConfiguredProviders: len(f.detected)}, nil
}

func (f *fakeGatewayStore) GetModelReference(_ context.Context, modelID, provider, model string, refresh bool) (ModelReference, error) {
	if f.saveErr != nil {
		return ModelReference{}, f.saveErr
	}
	return ModelReference{ModelID: modelID, Source: ModelsReferenceSource, Note: modelsReferenceNote, Found: true}, nil
}

func TestGatewayHandlerGetMasksSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeGatewayStore{summary: GatewaySummary{AccountID: "acc", GatewayID: "prod", TokenConfigured: true}}
	router := gin.New()
	router.GET("/settings/ai-gateway", NewHandler(store).Get)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/ai-gateway", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token-value") {
		t.Fatalf("response leaks secret material: %s", rec.Body.String())
	}
	var payload struct {
		Data GatewaySummary `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not the envelope: %v", err)
	}
	if !payload.Data.TokenConfigured || payload.Data.GatewayID != "prod" {
		t.Fatalf("summary = %+v", payload.Data)
	}
}

func TestGatewayHandlerUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Non-admin gating rides the outer Control Plane guard (controlProxy +
	// guard enforces the operator capability); the handler resolves the
	// actor from the request principal without re-checking the role, so
	// there is no in-handler 403 case to pin here. Guard-level denial is
	// covered by TestControlAuthMatrix in runtime/interfaces/http.
	t.Run("rejects bad JSON", func(t *testing.T) {
		store := &fakeGatewayStore{}
		router := gin.New()
		router.PUT("/settings/ai-gateway", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
			NewHandler(store).Update(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway", strings.NewReader(`{`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("forwards actor and input", func(t *testing.T) {
		store := &fakeGatewayStore{}
		router := gin.New()
		router.PUT("/settings/ai-gateway", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
			NewHandler(store).Update(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway", strings.NewReader(`{"token":"tok","account_id":"a","gateway_id":"g"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(store.updated) != 1 || store.actors[0] != 9 || store.updated[0].GatewayID != "g" {
			t.Fatalf("store calls = %+v actors = %v", store.updated, store.actors)
		}
	})

	t.Run("surfaces store errors", func(t *testing.T) {
		store := &fakeGatewayStore{setErr: errors.New("网关 account id 必须是 32 位小写十六进制")}
		router := gin.New()
		router.PUT("/settings/ai-gateway", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
			NewHandler(store).Update(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway", strings.NewReader(`{"account_id":"a","gateway_id":"g"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestGatewayHandlerMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("lists mappings", func(t *testing.T) {
		store := &fakeGatewayStore{mappings: map[string]string{"generation": "openai/gpt-image-1"}}
		router := gin.New()
		router.GET("/settings/ai-gateway/models", NewHandler(store).ListMappings)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/models", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "openai/gpt-image-1") {
			t.Fatalf("body = %s, want mapping", rec.Body.String())
		}
	})

	t.Run("save forwards actor and mapping", func(t *testing.T) {
		store := &fakeGatewayStore{}
		router := gin.New()
		router.PUT("/settings/ai-gateway/models/:feature", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
			NewHandler(store).SaveMapping(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway/models/generation", strings.NewReader(`{"model":"openai/gpt-image-1"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(store.saved) != 1 || store.saved[0].actor != 9 || store.saved[0].model != "openai/gpt-image-1" {
			t.Fatalf("store calls = %+v", store.saved)
		}
	})
}

func TestGatewayHandlerCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("list returns catalog items", func(t *testing.T) {
		store := &fakeGatewayStore{catalog: []CatalogItem{
			{ID: "test/model-1", Name: "Model 1", Provider: "Test"},
		}}
		router := gin.New()
		router.GET("/settings/ai-gateway/catalog", NewHandler(store).ListCatalog)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/catalog", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "test/model-1") {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})

	t.Run("save catalog succeeds for admin", func(t *testing.T) {
		store := &fakeGatewayStore{}
		router := gin.New()
		router.PUT("/settings/ai-gateway/catalog", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
			NewHandler(store).SaveCatalog(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway/catalog", strings.NewReader(`{"items":[{"id":"m1","name":"Model 1","provider":"Test"}]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(store.catalog) != 1 || store.catalog[0].ID != "m1" {
			t.Fatalf("store catalog = %+v", store.catalog)
		}
	})

	t.Run("save catalog rejects legacy price fields", func(t *testing.T) {
		store := &fakeGatewayStore{}
		router := gin.New()
		router.PUT("/settings/ai-gateway/catalog", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
			NewHandler(store).SaveCatalog(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/settings/ai-gateway/catalog", strings.NewReader(`{"items":[{"id":"m1","name":"Model 1","provider":"Test","input_cost_per_token":0.0000025}]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		if len(store.catalog) != 0 {
			t.Fatalf("store catalog = %+v, want untouched", store.catalog)
		}
	})
}

func TestGatewayHandlerRoutePreview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	servePreview := func(t *testing.T, role principal.Role, body string) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.POST("/settings/ai-gateway/route-preview", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 7, role)
			NewHandler(&fakeGatewayStore{}).PreviewGatewayRoute(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/settings/ai-gateway/route-preview", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("preview resolves first-party namespace and default path", func(t *testing.T) {
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{`"provider_slug":"openai"`, `"is_first_party":true`, `"target_path":"v1/images/generations"`, `"path_source":"generations"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("body missing %s: %s", want, body)
			}
		}
	})

	t.Run("preview honors path override and custom namespace", func(t *testing.T) {
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":"myco/my-model","custom_path":"v1/custom/go","custom_params":"{\"type\":\"chat\"}"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{`"provider_slug":"custom-myco"`, `"is_first_party":false`, `"target_path":"v1/custom/go"`, `"path_source":"override"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("body missing %s: %s", want, body)
			}
		}
	})

	t.Run("preview rejects malformed JSON", func(t *testing.T) {
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})
}

func TestGatewayHandlerListProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serve := func(t *testing.T, role principal.Role) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/settings/ai-gateway/providers", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, role)
			NewHandler(&fakeGatewayStore{}).ListProviders(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/providers", nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	// Non-admin gating rides the outer Control Plane guard; see
	// TestControlAuthMatrix for the guard-level 403 matrix.
	rec := serve(t, principal.RoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	// Registry 直出：fal 取社区前缀 fal_ai/ 作 ID 提示，vertex 取 vertex_ai/。
	for _, want := range []string{`"slug":"fal"`, `"id_prefix":"fal_ai/"`, `"slug":"google-vertex-ai"`, `"id_prefix":"vertex_ai/"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %s: %s", want, body)
		}
	}
}

func TestGatewayHandlerDetectModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Non-admin gating rides the outer Control Plane guard (see
	// TestControlAuthMatrix); the handler performs no role check.
	t.Run("returns detected models for admin", func(t *testing.T) {
		store := &fakeGatewayStore{
			detected: []DetectedModel{
				{ID: "@cf/black-forest-labs/flux-1-schnell", Name: "FLUX.1 Schnell", Provider: "Workers AI"},
				{ID: "gpt-4o", Name: "GPT-4o", Provider: "OpenAI"},
			},
		}
		router := gin.New()
		router.GET("/settings/ai-gateway/detect-models", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
			NewHandler(store).DetectModels(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/detect-models?provider=openai", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "flux-1-schnell") || !strings.Contains(rec.Body.String(), "gpt-4o") {
			t.Fatalf("unexpected body: %s", rec.Body.String())
		}
	})
}

func TestGatewayHandlerTestConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restorePolicy := outbound.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restorePolicy)

	serveTest := func(t *testing.T, role principal.Role, body string) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.POST("/settings/ai-gateway/test-connection", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 7, role)
			NewHandler(&fakeGatewayStore{}).TestConnection(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/settings/ai-gateway/test-connection", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("missing store returns service unavailable", func(t *testing.T) {
		router := gin.New()
		router.POST("/settings/ai-gateway/test-connection", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
			NewHandler(nil).TestConnection(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/settings/ai-gateway/test-connection", strings.NewReader(`{"base_url":"http://127.0.0.1:9"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
	})

	t.Run("reachable server reports status even on 4xx", func(t *testing.T) {
		var captured *http.Request
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = r.Clone(r.Context())
			w.WriteHeader(http.StatusNotFound)
		}))
		defer upstream.Close()
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"`+upstream.URL+`","provider":"openai","model":"gpt-4o","path":"v1/models"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		for _, want := range []string{`"reachable":true`, `"status_code":404`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body missing %s: %s", want, rec.Body.String())
			}
		}
		if captured == nil || captured.Method != http.MethodGet || captured.URL.Path != "/openai/v1/models" {
			t.Fatalf("probe request = %#v, want GET /openai/v1/models", captured)
		}
		if captured.Header.Get("cf-aig-max-attempts") != "1" || captured.Header.Get("cf-aig-skip-cache") != "true" {
			t.Fatalf("probe headers = %v", captured.Header)
		}
	})

	t.Run("unreachable host reports failure", func(t *testing.T) {
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"http://127.0.0.1:9"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"reachable":false`) {
			t.Fatalf("body missing unreachable: %s", rec.Body.String())
		}
	})

	t.Run("non-http URL is rejected", func(t *testing.T) {
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"ftp://example.com/x"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"reachable":false`) {
			t.Fatalf("body missing unreachable: %s", rec.Body.String())
		}
	})
}

func TestGatewayHandlerModelReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serve := func(t *testing.T, role principal.Role, target string) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/settings/ai-gateway/models-reference", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, role)
			NewHandler(&fakeGatewayStore{}).GetModelReference(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/models-reference"+target, nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("returns reference for admin", func(t *testing.T) {
		rec := serve(t, principal.RoleAdmin, "?model_id=openai/gpt-5")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		for _, want := range []string{`"model_id":"openai/gpt-5"`, `"source":"models.dev"`, `"found":true`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body missing %s: %s", want, rec.Body.String())
			}
		}
	})
}
