package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mediadomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
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
	router.GET("/api/search", NewSearchHandler(service, nil).Search)
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

func TestPublicSearchProjectsOpaqueDisplayWithoutImport(t *testing.T) {
	hit := domain.Result{ID: "image", Provider: domain.ProviderUnsplash, SourceURL: "https://unsplash.com/image", ThumbnailURL: "https://images.unsplash.com/thumb", PreviewURL: "https://images.unsplash.com/preview", OriginalURL: "https://images.unsplash.com/original", MediaKind: "image", Width: 4000, Height: 3000}
	service := application.NewSearchService(enabledSearchConfigs(), []ports.SearchProviderAdapter{stubSearchProvider{provider: domain.ProviderUnsplash, page: ports.SearchProviderPage{Results: []domain.Result{hit}}}})
	config := imgproxy.Config{BaseURL: "http://unused.test", KeyHex: strings.Repeat("ab", 32), SaltHex: strings.Repeat("cd", 32)}
	previews, err := preview.NewService(config, t.TempDir(), imgproxy.NewRenderer(config, nil))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewSearchHandler(service, previews)
	router := gin.New()
	router.GET("/api/search", handler.Search)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/search?q=image", nil))
	var response struct {
		Data struct {
			Results []struct {
				Original string              `json:"original_url"`
				Display  mediadomain.Display `json:"display"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || len(response.Data.Results) != 1 || len(response.Data.Results[0].Display.Variants) != 3 || response.Data.Results[0].Display.ExpiresAt == "" || response.Data.Results[0].Original != hit.OriginalURL {
		t.Fatal("public display or original identity missing")
	}
	for _, variant := range response.Data.Results[0].Display.Variants {
		if !strings.HasPrefix(variant.URL, "/api/media/previews/") {
			t.Fatal("external display URL exposed")
		}
	}
}
