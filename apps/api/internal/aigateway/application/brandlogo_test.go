package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

const testBrandLogoSVG = `<svg viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg"><circle cx="20" cy="20" r="18"/></svg>`

// stubBrandLogoUpstream points the logo seam at a local server under an
// allowlisted policy (mirrors stubModelsDevUpstream for the reference seam).
func stubBrandLogoUpstream(t *testing.T, policy []string, handler http.HandlerFunc) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: policy})
	t.Cleanup(restore)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previousURL := modelsDevLogoBaseURL
	modelsDevLogoBaseURL = server.URL
	t.Cleanup(func() { modelsDevLogoBaseURL = previousURL })
	resetBrandLogoCache()
	t.Cleanup(resetBrandLogoCache)
}

func TestNormalizeBrandLogoProvider(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  string
		valid bool
	}{
		{"openai", "openai", true},
		{" OpenAI ", "openai", true},
		{"cloudflare-r2", "cloudflare-r2", true},
		{"a", "a", true},
		{strings.Repeat("a", 64), strings.Repeat("a", 64), true},
		{"", "", false},
		{"../openai", "", false},
		{"a/b", "", false},
		{"a_b", "", false},
		{"a.b", "", false},
		{"-lead", "", false},
		{"trail-", "", false},
		{"has space", "", false},
		{"https://models.dev", "", false},
		{strings.Repeat("a", 65), "", false},
	} {
		t.Run("input "+tc.raw, func(t *testing.T) {
			got, err := normalizeBrandLogoProvider(tc.raw)
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("normalize(%q) = %q, %v; want %q", tc.raw, got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("normalize(%q) = %q, want error", tc.raw, got)
			}
		})
	}
}

func TestLoadBrandLogoAllowlistReject(t *testing.T) {
	// models.dev 不在 egress allowlist：直接回默认图，不发起任何上游请求。
	stubBrandLogoUpstream(t, []string{"example.com"}, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logo fetch must not run without allowlist")
	})
	body, fallback, err := loadBrandLogo(context.Background(), "openai")
	if err != nil {
		t.Fatalf("loadBrandLogo() error = %v", err)
	}
	if !fallback || string(body) != defaultBrandLogoSVG {
		t.Fatalf("body = %q, fallback = %v; want embedded default mark", body, fallback)
	}
}

func TestLoadBrandLogoPassthroughAndCache(t *testing.T) {
	var hits atomic.Int64
	stubBrandLogoUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/openai.svg" {
			t.Errorf("upstream path = %s, want /openai.svg", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(testBrandLogoSVG))
	})
	ctx := context.Background()

	first, fallback, err := loadBrandLogo(ctx, "openai")
	if err != nil || fallback || string(first) != testBrandLogoSVG {
		t.Fatalf("first = %q, fallback = %v, err = %v", first, fallback, err)
	}

	// TTL 内：缓存命中，不再请求上游。
	second, fallback, err := loadBrandLogo(ctx, "openai")
	if err != nil || fallback || string(second) != testBrandLogoSVG {
		t.Fatalf("second = %q, fallback = %v, err = %v", second, fallback, err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (cache hit)", got)
	}

	// TTL 过期：下一次自动重抓。
	brandLogoCache.Lock()
	for key, entry := range brandLogoCache.entries {
		entry.fetchedAt = time.Now().Add(-brandLogoTTL - time.Minute)
		brandLogoCache.entries[key] = entry
	}
	brandLogoCache.Unlock()
	if _, _, err := loadBrandLogo(ctx, "openai"); err != nil {
		t.Fatalf("refetch error = %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("upstream hits = %d, want 2 (refetch after expiry)", got)
	}
}

func TestLoadBrandLogoFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantLog string
	}{
		// fal 链路：上游缺失（404）与失败（500）都回默认图，卡片不断图。
		{"upstream 404 miss serves default", http.StatusNotFound, "nope", "404"},
		{"upstream 500 failure serves default", http.StatusInternalServerError, "boom", "500"},
		{"upstream 200 non-svg serves default", http.StatusOK, "<html>error page</html>", "non-svg"},
		{"upstream 200 empty serves default", http.StatusOK, "", "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubBrandLogoUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			body, fallback, err := loadBrandLogo(context.Background(), "fal")
			if err != nil {
				t.Fatalf("loadBrandLogo() error = %v", err)
			}
			if !fallback || string(body) != defaultBrandLogoSVG {
				t.Fatalf("body = %q, fallback = %v; want embedded default mark (%s)", body, fallback, tc.wantLog)
			}
		})
	}

	// 无效 provider 直接 400 语义（error），不回默认图。
	stubBrandLogoUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logo fetch must not run for invalid provider")
	})
	if _, _, err := loadBrandLogo(context.Background(), "../../etc"); err == nil {
		t.Fatal("path traversal succeeded, want error")
	}
}

func TestGatewayHandlerBrandLogo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubBrandLogoUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fal.svg" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(testBrandLogoSVG))
	})
	serve := func(t *testing.T, role principal.Role, target string) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/settings/ai-gateway/brand-logo", func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 9, role)
			NewHandler(&fakeGatewayStore{}).GetBrandLogo(c)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/brand-logo"+target, nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("rejects invalid provider", func(t *testing.T) {
		if rec := serve(t, principal.RoleAdmin, "?provider=../../x"); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("proxies upstream svg", func(t *testing.T) {
		rec := serve(t, principal.RoleAdmin, "?provider=openai")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ctype := rec.Header().Get("Content-Type"); ctype != brandLogoContentType {
			t.Fatalf("content-type = %q, want %q", ctype, brandLogoContentType)
		}
		if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "private, max-age=86400" {
			t.Fatalf("cache-control = %q, want private admin response", cacheControl)
		}
		if rec.Header().Get(brandLogoFallbackHeader) != "" {
			t.Fatalf("unexpected fallback header on upstream bytes")
		}
		if rec.Body.String() != testBrandLogoSVG {
			t.Fatalf("body = %q, want upstream svg", rec.Body.String())
		}
	})

	t.Run("fal miss degrades to default mark", func(t *testing.T) {
		rec := serve(t, principal.RoleAdmin, "?provider=fal")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ctype := rec.Header().Get("Content-Type"); ctype != brandLogoContentType {
			t.Fatalf("content-type = %q, want %q", ctype, brandLogoContentType)
		}
		if rec.Header().Get(brandLogoFallbackHeader) == "" {
			t.Fatal("missing fallback header on degraded logo")
		}
		if rec.Body.String() != defaultBrandLogoSVG {
			t.Fatalf("body = %q, want default mark", rec.Body.String())
		}
	})
}
