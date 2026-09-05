package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBuildModelDetectionURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiType string
		want    string
		wantErr bool
	}{
		{name: "with v1", baseURL: "https://api.example.com/v1", want: "https://api.example.com/v1/models"},
		{name: "google root path", baseURL: "https://generativelanguage.googleapis.com", apiType: "google-ai", want: "https://generativelanguage.googleapis.com/v1beta/models"},
		{name: "google v1beta", baseURL: "https://generativelanguage.googleapis.com/v1beta", apiType: "google-ai", want: "https://generativelanguage.googleapis.com/v1beta/models"},
		{name: "trailing slash", baseURL: "https://api.example.com/v1/", want: "https://api.example.com/v1/models"},
		{name: "root path", baseURL: "https://api.example.com", want: "https://api.example.com/models"},
		{name: "drop query", baseURL: "https://api.example.com/v1?x=1", want: "https://api.example.com/v1/models"},
		{name: "invalid", baseURL: "://bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildModelDetectionURL(tt.baseURL, tt.apiType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildGoogleCloudModelProbeURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		model   string
		want    string
	}{
		{
			name:    "root",
			baseURL: "https://aiplatform.googleapis.com",
			model:   "gemini-3.1-flash-lite-image",
			want:    "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-3.1-flash-lite-image:countTokens",
		},
		{
			name:    "v1",
			baseURL: "https://aiplatform.googleapis.com/v1",
			model:   "gemini-2.5-flash-image",
			want:    "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.5-flash-image:countTokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGoogleCloudModelProbeURL(tt.baseURL, tt.model)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectModelsTimeoutAllowsProxyRouteWarmup(t *testing.T) {
	if detectModelsTimeout < 15*time.Second {
		t.Fatalf("detectModelsTimeout = %s, want at least 15s for proxy/TUN DNS and TLS warmup", detectModelsTimeout)
	}
}

func TestProviderModelDetectionTransportFailureCategoryDoesNotExposeRequestURL(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "canceled", err: fmt.Errorf("request with ?key=secret failed: %w", context.Canceled), want: "request_canceled"},
		{name: "timeout", err: fmt.Errorf("request with ?key=secret failed: %w", context.DeadlineExceeded), want: "request_timeout"},
		{name: "network", err: fmt.Errorf("request with ?key=secret failed"), want: "network_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerModelDetectionTransportFailureCategory(tt.err); got != tt.want {
				t.Fatalf("category = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectModelsProbesSupportedGoogleCloudImageModels(t *testing.T) {
	seenModels := make(map[string]bool)
	var seenModelsMu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := verifyGoogleCloudCountTokensRequest(t, r)
		seenModelsMu.Lock()
		seenModels[model] = true
		seenModelsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if model == "gemini-3.1-flash-image" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalTokens":1}`))
	}))
	defer upstream.Close()

	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	setProviderEndpointTestAllowHosts(t, parsed.Hostname())

	handler := NewHandler(nil, "production")
	router := gin.New()
	router.POST("/provider-endpoints/detect-models", handler.DetectModels)
	rec := serveProviderEndpointJSON(t, router, http.MethodPost, "/provider-endpoints/detect-models", DetectModelsRequest{
		BaseURL: upstream.URL,
		APIKey:  "google-cloud-key",
		APIType: "google-cloud",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data detectedModelsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := strings.Join(body.Data.Models, ","), "gemini-3.1-flash-lite-image,gemini-2.5-flash-image"; got != want {
		t.Fatalf("models = %q, want %q", got, want)
	}

	seenModelsMu.Lock()
	defer seenModelsMu.Unlock()
	for _, model := range googleCloudImageModelCandidates {
		if !seenModels[model] {
			t.Errorf("model %q was not probed", model)
		}
	}
}

func verifyGoogleCloudCountTokensRequest(t *testing.T, r *http.Request) string {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", r.Method)
	}
	const prefix = "/v1/publishers/google/models/"
	const suffix = ":countTokens"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, suffix) {
		t.Fatalf("unexpected path = %q", r.URL.Path)
	}
	if got := r.URL.Query().Get("key"); got != "google-cloud-key" {
		t.Fatalf("key query = %q", got)
	}
	if got := r.Header.Get("x-goog-api-key"); got != "" {
		t.Fatalf("x-goog-api-key = %q, want empty", got)
	}
	verifyGoogleCloudCountTokensPayload(t, r.Body)
	return strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
}

func verifyGoogleCloudCountTokensPayload(t *testing.T, body io.Reader) {
	t.Helper()
	var payload struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if len(payload.Contents) != 1 || payload.Contents[0].Role != "user" || len(payload.Contents[0].Parts) != 1 || payload.Contents[0].Parts[0].Text == "" {
		t.Fatalf("unexpected request payload: %+v", payload)
	}
}

func TestDetectModelsExplainsGoogleCloudServiceAccountKeyRequirement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 401,
				"message": "API keys are not supported by this API. Expected OAuth2 access token.",
				"status": "UNAUTHENTICATED",
				"details": [{
					"@type": "type.googleapis.com/google.rpc.ErrorInfo",
					"reason": "CREDENTIALS_MISSING",
					"domain": "googleapis.com",
					"metadata": {
						"service": "aiplatform.googleapis.com",
						"method": "google.cloud.aiplatform.v1.PredictionService.CountTokens"
					}
				}]
			}
		}`))
	}))
	defer upstream.Close()

	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	setProviderEndpointTestAllowHosts(t, parsed.Hostname())

	handler := NewHandler(nil, "production")
	router := gin.New()
	router.POST("/provider-endpoints/detect-models", handler.DetectModels)
	rec := serveProviderEndpointJSON(t, router, http.MethodPost, "/provider-endpoints/detect-models", DetectModelsRequest{
		BaseURL: upstream.URL,
		APIKey:  "test-key",
		APIType: "google-cloud",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Msg != "模型检测失败：请使用关联 Service Account 的 Google Cloud API Key，并授予模型调用权限" {
		t.Fatalf("msg = %q", body.Msg)
	}
}

func TestParseModelIDs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "openai data",
			body: `{"data":[{"id":"model-a"},{"id":"model-b"}]}`,
			want: []string{"model-a", "model-b"},
		},
		{
			name: "models mixed",
			body: `{"models":["model-a",{"id":"model-b"}," ",{"id":"model-a"}]}`,
			want: []string{"model-a", "model-b"},
		},
		{
			name: "google model names",
			body: `{"models":[{"name":"models/gemini-2.5-flash-image"},{"name":"models/imagen-4.0-generate-001"}]}`,
			want: []string{"gemini-2.5-flash-image", "imagen-4.0-generate-001"},
		},
		{
			name: "top level array",
			body: `[{"id":"model-a"},"model-b"]`,
			want: []string{"model-a", "model-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseModelIDs([]byte(tt.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got len %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d]=%q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseModelIDsLimit(t *testing.T) {
	items := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		items = append(items, fmt.Sprintf(`{"id":"m-%03d"}`, i))
	}
	body := `{"data":[` + strings.Join(items, ",") + `]}`
	got, err := parseModelIDs([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != maxDetectedModels {
		t.Fatalf("got len %d, want %d", len(got), maxDetectedModels)
	}
}

func TestIsDevLikeEnvironment(t *testing.T) {
	prevMode := gin.Mode()
	t.Cleanup(func() {
		gin.SetMode(prevMode)
	})

	svc := NewService(nil)

	gin.SetMode(gin.ReleaseMode)
	svc.SetAppEnv("development")
	if !svc.IsDevLikeEnvironment() {
		t.Fatal("appEnv=development with release mode should be dev-like")
	}

	gin.SetMode(gin.ReleaseMode)
	svc.SetAppEnv("production")
	if svc.IsDevLikeEnvironment() {
		t.Fatal("appEnv=production should not be dev-like")
	}

	gin.SetMode(gin.DebugMode)
	svc.SetAppEnv("")
	if !svc.IsDevLikeEnvironment() {
		t.Fatal("empty appEnv with debug mode should be dev-like")
	}
}

func TestDetectModelsAllowHostsMergesEnvironmentAndDevDefaults(t *testing.T) {
	prevMode := gin.Mode()
	t.Cleanup(func() {
		gin.SetMode(prevMode)
	})

	setProviderEndpointTestAllowHosts(t, "models.internal")
	gin.SetMode(gin.ReleaseMode)
	prodSvc := NewService(nil, "production")
	if got := strings.Join(prodSvc.DetectModelsAllowHosts(context.Background()), ","); got != "models.internal" {
		t.Fatalf("production allow hosts = %q, want models.internal", got)
	}

	devSvc := NewService(nil, "development")
	got := strings.Join(devSvc.DetectModelsAllowHosts(context.Background()), ",")
	for _, want := range []string{"models.internal", "localhost", "127.0.0.1", "::1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("development allow hosts = %q, missing %q", got, want)
		}
	}
}
