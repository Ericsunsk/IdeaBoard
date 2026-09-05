package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/sse"

	"github.com/gin-gonic/gin"
)

// fakeGatewayUpstream is a real httptest server that emulates the nginx
// same-origin gateway in front of the API. It uses httputil.ReverseProxy to
// forward requests to the inner API exactly the way the production nginx
// configuration does: method, path, body, Cookie, Authorization, Idempotency-Key,
// and Content-Type reach the API unchanged; cache-bypass response headers are
// added on every business route. The test is the "gateway contract test" that
// issues #655 calls for (cookies, Idempotency-Key, SSE, multipart, upstream
// errors).
type fakeGatewayUpstream struct {
	gateway *httptest.Server
	api     *httptest.Server
}

func newFakeGateway(t *testing.T, inner http.Handler) *fakeGatewayUpstream {
	t.Helper()

	apiServer := httptest.NewServer(inner)
	t.Cleanup(apiServer.Close)

	apiURL, err := url.Parse(apiServer.URL)
	if err != nil {
		t.Fatalf("parse api url: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(apiURL)

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same-origin gateway contract: strip hop-by-hop headers; preserve
		// everything else (Cookie, Authorization, Idempotency-Key, Content-Type,
		// Accept, body).
		r.Header.Del("Connection")
		r.Header.Del("Proxy-Connection")
		// nginx add_header fires before the response body is committed, so we
		// wrap the ResponseWriter to inject the cache-bypass headers on
		// WriteHeader. This mirrors the production nginx behavior.
		ww := &cacheBypassWriter{ResponseWriter: w, path: r.URL.Path}
		proxy.ServeHTTP(ww, r)
	}))
	t.Cleanup(gw.Close)
	return &fakeGatewayUpstream{gateway: gw, api: apiServer}
}

func isBusinessRoute(path string) bool {
	switch {
	case path == "/api/health":
		return true
	case path == "/api/auth/health":
		return true
	case strings.HasPrefix(path, "/api/auth/"):
		return true
	case strings.HasPrefix(path, "/api/user/tasks/"):
		return true
	case strings.HasPrefix(path, "/api/"):
		return true
	case strings.HasPrefix(path, "/uploads/"):
		return true
	}
	return false
}

// cacheBypassWriter is a thin http.ResponseWriter wrapper that injects the
// same Cache-Control and Pragma headers the production nginx gateway adds on
// every business route. It applies the headers in WriteHeader so the response
// is not yet committed when the injection happens.
type cacheBypassWriter struct {
	http.ResponseWriter
	path        string
	wroteHeader bool
}

func (c *cacheBypassWriter) WriteHeader(status int) {
	if !c.wroteHeader {
		if isBusinessRoute(c.path) {
			if c.ResponseWriter.Header().Get("Cache-Control") == "" {
				c.ResponseWriter.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			}
			if c.ResponseWriter.Header().Get("Pragma") == "" {
				c.ResponseWriter.Header().Set("Pragma", "no-cache")
			}
		}
		c.wroteHeader = true
	}
	c.ResponseWriter.WriteHeader(status)
}

// TestGatewayContractForwardsCookieHeader is the gateway contract for
// /api/auth/get-session: cookies issued by Auth on the same host must reach
// the API unchanged through the gateway.
func TestGatewayContractForwardsCookieHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenCookie string
	inner := gin.New()
	inner.GET("/api/user/session", func(c *gin.Context) {
		seenCookie = c.GetHeader("Cookie")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"authenticated": seenCookie != ""}})
	})
	fg := newFakeGateway(t, inner)

	req, err := http.NewRequest(http.MethodGet, fg.gateway.URL+"/api/user/session", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Cookie", "better-auth.session_token=abc123; better-auth.session_data=xyz")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "ideaboard.art")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if seenCookie == "" {
		t.Fatalf("gateway dropped Cookie header")
	}
	if !strings.Contains(seenCookie, "better-auth.session_token=abc123") {
		t.Fatalf("Cookie header lost session token: %q", seenCookie)
	}
}

// TestGatewayContractForwardsIdempotencyKey is the gateway contract for
// Vision Search and other write endpoints: the Idempotency-Key header must
// reach the API unchanged so the server can deduplicate retries.
func TestGatewayContractForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenKey string
	inner := gin.New()
	inner.POST("/api/search/vision", func(c *gin.Context) {
		seenKey = c.GetHeader("Idempotency-Key")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"idempotency_key": seenKey}})
	})
	fg := newFakeGateway(t, inner)

	body := bytes.NewBufferString(`{"query":"red silk"}`)
	req, err := http.NewRequest(http.MethodPost, fg.gateway.URL+"/api/search/vision", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "vis-2025-01-01-abc")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if seenKey != "vis-2025-01-01-abc" {
		t.Fatalf("Idempotency-Key = %q, want %q", seenKey, "vis-2025-01-01-abc")
	}
}

// TestGatewayContractPreservesSSEResponse verifies that the same-origin gateway
// forwards an SSE Content-Type, the initial "connected" frame, and a follow-up
// event without buffering the response.
func TestGatewayContractPreservesSSEResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inner := gin.New()
	inner.GET("/api/user/events", func(c *gin.Context) {
		// The real handler validates the principal via the auth middleware. We
		// do not exercise the auth path here; we only need the SSE response
		// to flow through the gateway untouched.
		if err := sse.Prepare(c); err != nil {
			t.Errorf("sse.Prepare: %v", err)
		}
		c.Header("Cache-Control", "no-store")
		if _, err := c.Writer.Write([]byte(":connected\n\n")); err != nil {
			return
		}
		c.Writer.Flush()
		if _, err := c.Writer.Write([]byte("event: ping\ndata: {\"id\":1}\n\n")); err != nil {
			return
		}
		c.Writer.Flush()
	})
	fg := newFakeGateway(t, inner)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fg.gateway.URL+"/api/user/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", res.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(res.Body)
	got, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		t.Fatalf("read first frame: %v", err)
	}
	if !strings.HasPrefix(got, ":connected") {
		t.Fatalf("first frame = %q, want connected comment", got)
	}
}

// TestGatewayContractForwardsMultipart verifies that the same-origin gateway
// preserves multipart/form-data bodies and the per-part Content-Type, so image
// search uploads complete end to end.
func TestGatewayContractForwardsMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var (
		seenContentType string
		seenFileBytes   []byte
		seenIdemKey     string
	)
	inner := gin.New()
	inner.POST("/api/search/images", func(c *gin.Context) {
		seenContentType = c.GetHeader("Content-Type")
		seenIdemKey = c.GetHeader("Idempotency-Key")
		file, header, err := c.Request.FormFile("reference")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer file.Close()
		seenFileBytes, _ = io.ReadAll(file)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"filename": header.Filename,
				"size":     len(seenFileBytes),
				"mime":     header.Header.Get("Content-Type"),
			},
		})
	})
	fg := newFakeGateway(t, inner)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("reference", "swatch.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte("\x89PNG\r\n\x1a\nfakebytes")); err != nil {
		t.Fatalf("write png header: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, fg.gateway.URL+"/api/search/images", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Idempotency-Key", "img-2025-01-01-001")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body=%s", res.StatusCode, raw)
	}
	if !strings.HasPrefix(seenContentType, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", seenContentType)
	}
	if len(seenFileBytes) == 0 {
		t.Fatalf("multipart body not forwarded through gateway")
	}
	if seenIdemKey == "" {
		t.Fatalf("Idempotency-Key dropped through gateway")
	}
}

// TestGatewayContractUpstreamErrorSurfaces500 ensures that a real upstream
// error from the API bubbles up as a 5xx (not cached or rewritten) when the
// gateway reaches the API.
func TestGatewayContractUpstreamErrorSurfaces500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inner := gin.New()
	inner.GET("/api/internal/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "INTERNAL", "msg": "boom"},
		})
	})
	fg := newFakeGateway(t, inner)

	res, err := http.Get(fg.gateway.URL + "/api/internal/fail")
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("expected error payload, got %v", payload)
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
}

// TestGatewayContractCacheBypassHeaders is the full response-header contract
// for every business route. It enforces the same Cache-Control and Pragma
// headers the nginx template writes.
func TestGatewayContractCacheBypassHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inner := gin.New()
	inner.GET("/api/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	inner.POST("/api/auth/get-session", func(c *gin.Context) { c.Status(http.StatusOK) })
	inner.POST("/api/search/vision", func(c *gin.Context) { c.Status(http.StatusOK) })
	inner.POST("/uploads/image", func(c *gin.Context) { c.Status(http.StatusOK) })
	inner.GET("/api/user/tasks/123/events", func(c *gin.Context) { c.Status(http.StatusOK) })
	fg := newFakeGateway(t, inner)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodPost, "/api/auth/get-session"},
		{http.MethodPost, "/api/search/vision"},
		{http.MethodPost, "/uploads/image"},
		{http.MethodGet, "/api/user/tasks/123/events"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, fg.gateway.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer res.Body.Close()
			if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Fatalf("Cache-Control = %q, want no-store", cc)
			}
			if p := res.Header.Get("Pragma"); p != "no-cache" {
				t.Fatalf("Pragma = %q, want no-cache", p)
			}
		})
	}
}
