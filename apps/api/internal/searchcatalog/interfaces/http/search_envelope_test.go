package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/searchcatalog/application"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"

	"github.com/gin-gonic/gin"
)

type stubSearchConfigs struct {
	configs []domain.ProviderConfig
	key     string
}

func (s stubSearchConfigs) List(context.Context) ([]domain.ProviderConfig, error) {
	return s.configs, nil
}

func (s stubSearchConfigs) APIKey(context.Context, domain.Provider) (string, error) {
	return s.key, nil
}

type stubSearchProvider struct {
	provider domain.Provider
	page     ports.SearchProviderPage
	err      error
}

func (s stubSearchProvider) Provider() domain.Provider { return s.provider }

func (s stubSearchProvider) Search(context.Context, string, ports.SearchRequest) (ports.SearchProviderPage, error) {
	return s.page, s.err
}

func enabledSearchConfigs() stubSearchConfigs {
	return stubSearchConfigs{
		configs: []domain.ProviderConfig{
			{Provider: domain.ProviderUnsplash, Enabled: true, TestState: domain.TestStatePassed},
		},
		key: "test-key",
	}
}

func servePublicSearch(t *testing.T, service *application.SearchService, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/search", NewSearchHandler(service).Search)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// TestPublicSearchRendersContractEnvelope pins the /api/search wire break:
// successes render the single {data} envelope (never the legacy numeric
// {code,msg,message_safe} shape) and provider outages render the
// {error:{code,message}} envelope at the same 502 status.
func TestPublicSearchRendersContractEnvelope(t *testing.T) {
	hit := domain.Result{
		ID: "r1", Provider: domain.ProviderUnsplash, Title: "t",
		Author: "a", AuthorURL: "https://a", SourceURL: "https://s",
		ThumbnailURL: "https://th", PreviewURL: "https://p", OriginalURL: "https://o",
		MediaKind: "image",
	}
	success := application.NewSearchService(enabledSearchConfigs(), []ports.SearchProviderAdapter{
		stubSearchProvider{provider: domain.ProviderUnsplash, page: ports.SearchProviderPage{Results: []domain.Result{hit}}},
	})
	recorder := servePublicSearch(t, success, "/api/search?q=test")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"data":{"query":"test"`) {
		t.Fatalf("body = %s, want contract {data} envelope", body)
	}
	if strings.Contains(body, `"message_safe"`) {
		t.Fatalf("body = %s, must not render the legacy numeric envelope", body)
	}

	failing := application.NewSearchService(enabledSearchConfigs(), []ports.SearchProviderAdapter{
		stubSearchProvider{provider: domain.ProviderUnsplash, err: errors.New("provider down")},
	})
	recorder = servePublicSearch(t, failing, "/api/search?q=test")
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 body=%s", recorder.Code, recorder.Body.String())
	}
	body = recorder.Body.String()
	if !strings.Contains(body, `"error":{"code":"search_unavailable"`) {
		t.Fatalf("body = %s, want contract {error} envelope with search_unavailable", body)
	}
}
