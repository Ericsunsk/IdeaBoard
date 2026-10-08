package infrastructure

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/outboundhttp"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

type providerHTTPStub func(context.Context, *http.Request, outboundhttp.RequestOptions) (*http.Response, error)

func (stub providerHTTPStub) Do(ctx context.Context, request *http.Request, options outboundhttp.RequestOptions) (*http.Response, error) {
	return stub(ctx, request, options)
}

func stubProviderHTTP(t *testing.T, stub providerHTTPStub) {
	t.Helper()
	previous := providerOutbound
	providerOutbound = stub
	t.Cleanup(func() { providerOutbound = previous })
}

func TestProviderAdaptersRequestAndMapping(t *testing.T) {
	unsplashPhoto := `{"id":"photo-1","description":"Title","width":640,"height":480,"urls":{"thumb":"thumb","small":"small","raw":"original"},"user":{"name":"Author","links":{"html":"author"}},"links":{"html":"source"}}`
	pexelsPhoto := `{"id":42,"alt":"Title","width":640,"height":480,"photographer":"Author","photographer_url":"author","url":"source","src":{"medium":"thumb","large":"preview","original":"original"}}`
	for _, test := range []struct {
		name                                             string
		adapter                                          ports.SearchProviderAdapter
		query, path, authorization, payload, id, preview string
		hasMore                                          bool
	}{
		{"unsplash curated", NewUnsplashAdapter(), "", "/photos", "Client-ID test-key", "[" + unsplashPhoto + "]", "photo-1", "small", true},
		{"unsplash search last page", NewUnsplashAdapter(), " warm shadows ", "/search/photos", "Client-ID test-key", `{"results":[` + unsplashPhoto + `],"total_pages":2}`, "photo-1", "small", false},
		{"pexels curated", NewPexelsAdapter(), "", "/v1/curated", "test-key", `{"photos":[` + pexelsPhoto + `],"next_page":"next"}`, "42", "preview", true},
		{"pexels search last page", NewPexelsAdapter(), " warm shadows ", "/v1/search", "test-key", `{"photos":[` + pexelsPhoto + `]}`, "42", "preview", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			stubProviderHTTP(t, func(_ context.Context, request *http.Request, options outboundhttp.RequestOptions) (*http.Response, error) {
				called = true
				if request.Method != http.MethodGet || request.URL.Path != test.path || request.Header.Get("Authorization") != test.authorization {
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
				}
				query := request.URL.Query()
				if query.Get("page") != "2" || query.Get("per_page") != "1" || query.Get("query") != strings.TrimSpace(test.query) {
					t.Fatalf("query = %v", query)
				}
				if options.Timeout != providerSearchTimeout || options.Target != string(test.adapter.Provider()) {
					t.Fatalf("outbound options = %#v", options)
				}
				hostAllowed := false
				for _, host := range options.AllowHosts {
					hostAllowed = hostAllowed || host == request.URL.Hostname()
				}
				if !hostAllowed {
					t.Fatal("API host missing from allowlist")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.payload))}, nil
			})
			page, err := test.adapter.Search(t.Context(), "test-key", ports.SearchRequest{Query: test.query, Page: 2, Limit: 1})
			if err != nil || !called || len(page.Results) != 1 || page.HasMore != test.hasMore {
				t.Fatalf("page = %#v, err = %v", page, err)
			}
			result := page.Results[0]
			if result.ID != test.id || result.Provider != test.adapter.Provider() || result.Title != "Title" || result.Author != "Author" || result.AuthorURL != "author" || result.SourceURL != "source" || result.ThumbnailURL != "thumb" || result.PreviewURL != test.preview || result.OriginalURL != "original" || result.Width != 640 || result.Height != 480 || result.MediaKind != "image" {
				t.Fatalf("mapping = %#v", result)
			}
		})
	}
}

type failedProviderBody struct{}

func (failedProviderBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedProviderBody) Close() error             { return nil }

func TestProviderAdaptersPropagateFailures(t *testing.T) {
	for _, adapter := range []ports.SearchProviderAdapter{NewUnsplashAdapter(), NewPexelsAdapter()} {
		for _, failure := range []string{"status", "outbound status", "body", "json", "body cap"} {
			t.Run(string(adapter.Provider())+"/"+failure, func(t *testing.T) {
				stubProviderHTTP(t, func(context.Context, *http.Request, outboundhttp.RequestOptions) (*http.Response, error) {
					if failure == "outbound status" {
						return nil, &outboundhttp.HTTPStatusError{StatusCode: http.StatusForbidden}
					}
					response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("invalid json"))}
					switch failure {
					case "status":
						response.StatusCode = http.StatusForbidden
					case "body":
						response.Body = failedProviderBody{}
					case "body cap":
						response.Body = io.NopCloser(strings.NewReader(strings.Repeat(" ", maxProviderBody) + `{}`))
					}
					return response, nil
				})
				_, err := adapter.Search(t.Context(), "test-key", ports.SearchRequest{Query: "texture", Page: 1, Limit: 1})
				if err == nil {
					t.Fatal("expected failure")
				}
				if strings.Contains(failure, "status") && err.Error() != string(adapter.Provider())+" status 403" {
					t.Fatalf("status error = %v", err)
				}
				if failure == "body" && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("body error = %v", err)
				}
			})
		}
	}
}

func TestProviderConstructorIdentity(t *testing.T) {
	if NewUnsplashAdapter().Provider() != domain.ProviderUnsplash || NewPexelsAdapter().Provider() != domain.ProviderPexels {
		t.Fatal("provider identity changed")
	}
}
