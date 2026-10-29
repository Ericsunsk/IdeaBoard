package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

type fakeGatewayStore struct {
	summary   GatewaySummary
	updated   []UpdateInput
	actors    []int64
	getErr    error
	setErr    error
	saveErr   error
	catalog   []CatalogItem
	providers []workflow.ProviderEntry
	provErr   error
}

func (f *fakeGatewayStore) GetSummary(context.Context) (GatewaySummary, error) {
	return f.summary, f.getErr
}

func (f *fakeGatewayStore) Update(_ context.Context, input UpdateInput, actorID int64) error {
	f.updated = append(f.updated, input)
	f.actors = append(f.actors, actorID)
	return f.setErr
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

func (f *fakeGatewayStore) ListConfiguredProviders(context.Context) ([]workflow.ProviderEntry, error) {
	if f.provErr != nil {
		return nil, f.provErr
	}
	return f.providers, nil
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
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":"custom-myco/my-model","custom_path":"v1/custom/go","custom_params":"{\"type\":\"chat\"}"}`)
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
	serve := func(t *testing.T, role principal.Role, store configStore) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/settings/ai-gateway/providers", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, role)
			NewHandler(store).ListProviders(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/providers", nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	// Non-admin gating rides the outer ControlPlane guard; see
	// TestControlAuthMatrix for the guard-level 403 matrix.
	t.Run("returns configured providers plus workers-ai", func(t *testing.T) {
		openai, _ := workflow.ResolveProvider("openai")
		store := &fakeGatewayStore{providers: []workflow.ProviderEntry{openai}}
		// workers-ai is a first-party constant: the store may omit it,
		// the repository adds it. Fake it here to assert the mapping.
		workersAI, _ := workflow.ResolveProvider("workers-ai")
		store.providers = append(store.providers, workersAI)
		rec := serve(t, principal.RoleAdmin, store)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{`"slug":"openai"`, `"slug":"workers-ai"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("body missing %s: %s", want, body)
			}
		}
	})

	t.Run("surfaces provider-config errors", func(t *testing.T) {
		store := &fakeGatewayStore{provErr: errors.New("control plane unavailable")}
		rec := serve(t, principal.RoleAdmin, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("missing store returns service unavailable", func(t *testing.T) {
		rec := serve(t, principal.RoleAdmin, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
	})
}

func TestGatewayHandlerTestConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restorePolicy := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
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

	t.Run("unreachable host reports failure", func(t *testing.T) {
		// The probe reports transport failure as reachable:false with HTTP 200.
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"http://127.0.0.1:9"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"reachable":false`) {
			t.Fatalf("body missing unreachable: %s", rec.Body.String())
		}
	})

	t.Run("partial provider or path is rejected", func(t *testing.T) {
		for _, body := range []string{
			`{"base_url":"http://127.0.0.1:9","provider":"openai"}`,
			`{"base_url":"http://127.0.0.1:9","path":"v1/models"}`,
		} {
			rec := serveTest(t, principal.RoleAdmin, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		}
	})

	t.Run("empty provider and path probe the gateway root", func(t *testing.T) {
		var capturedPath string
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusNotFound)
		}))
		defer upstream.Close()
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"`+upstream.URL+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		for _, want := range []string{`"reachable":true`, `"status_code":404`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body missing %s: %s", want, rec.Body.String())
			}
		}
		if capturedPath != "/" {
			t.Fatalf("root probe path = %q, want /", capturedPath)
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
