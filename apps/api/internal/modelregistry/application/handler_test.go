package application

import (
	"context"
	"errors"
	"ideaboard/api/internal/modelregistry/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	modelregistryports "ideaboard/api/internal/modelregistry/ports"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

type fakeGatewayStore struct {
	saveErr   error
	catalog   []domain.CatalogItem
	providers []workflow.ProviderEntry
	provErr   error
}

func (f *fakeGatewayStore) ListCatalog(context.Context) ([]domain.CatalogItem, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return f.catalog, nil
}

func (f *fakeGatewayStore) SaveCatalog(_ context.Context, items []domain.CatalogItem, _ int64) error {
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

func (f *fakeGatewayStore) ResolveVisionModel(context.Context) (string, error) {
	return "", domain.ErrVisionModelMissing
}

func TestGatewayHandlerCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("list returns catalog items", func(t *testing.T) {
		store := &fakeGatewayStore{catalog: []domain.CatalogItem{
			{ID: "test/model-1", Name: "Model 1", Provider: "Test"},
		}}
		router := gin.New()
		router.GET("/settings/ai-gateway/catalog", NewHandler(store, store).ListCatalog)

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
			NewHandler(store, store).SaveCatalog(c)
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
			NewHandler(store, store).SaveCatalog(c)
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
			NewHandler(&fakeGatewayStore{}, nil).PreviewWorkerRoute(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/settings/ai-gateway/route-preview", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("preview validates the Worker model ref", func(t *testing.T) {
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{`"provider":"openai"`, `"valid":true`, `"reachable":false`} {
			if !strings.Contains(body, want) {
				t.Fatalf("body missing %s: %s", want, body)
			}
		}
	})

	t.Run("preview rejects bare models", func(t *testing.T) {
		rec := servePreview(t, principal.RoleAdmin, `{"model_id":"gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"valid":false`) {
			t.Fatalf("body missing invalid verdict: %s", rec.Body.String())
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
	serve := func(t *testing.T, role principal.Role, store modelregistryports.ConfiguredProviderReader) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/settings/ai-gateway/providers", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, role)
			NewHandler(nil, store).ListProviders(c)
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
			NewHandler(&fakeGatewayStore{}, nil).TestConnection(c)
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
			NewHandler(nil, nil).TestConnection(c)
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
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"http://127.0.0.1:9","secret":"s","model":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"reachable":false`) {
			t.Fatalf("body missing unreachable: %s", rec.Body.String())
		}
	})

	t.Run("missing model is rejected", func(t *testing.T) {
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"http://127.0.0.1:9","secret":"s"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("dry-run probes the Worker endpoint", func(t *testing.T) {
		var capturedPath string
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"provider":"openai","model":"gpt-image-1","target":{"url":"https://up.example/v1","method":"POST","body":{}},"path_source":"openai:images:generations"}`))
		}))
		defer upstream.Close()
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"`+upstream.URL+`","secret":"s","model":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		// Reachable requires dry-run 200 with a non-empty path_source.
		for _, want := range []string{`"reachable":true`, `"status_code":200`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body missing %s: %s", want, rec.Body.String())
			}
		}
		if capturedPath != "/v1/dry-run" {
			t.Fatalf("dry-run path = %q, want /v1/dry-run", capturedPath)
		}
	})

	t.Run("dry-run non-200 is unreachable", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer upstream.Close()
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"`+upstream.URL+`","secret":"s","model":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		for _, want := range []string{`"reachable":false`, `"status_code":401`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body missing %s: %s", want, rec.Body.String())
			}
		}
	})

	t.Run("non-http URL is rejected", func(t *testing.T) {
		rec := serveTest(t, principal.RoleAdmin, `{"base_url":"ftp://example.com/x","secret":"s","model":"openai/gpt-image-1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"reachable":false`) {
			t.Fatalf("body missing unreachable: %s", rec.Body.String())
		}
	})
}
