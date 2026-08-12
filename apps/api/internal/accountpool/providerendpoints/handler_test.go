package providerendpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type testProviderEndpointOutboundPolicy struct{ hosts []string }

func (p testProviderEndpointOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func setProviderEndpointTestAllowHosts(t *testing.T, hosts ...string) {
	t.Helper()
	restore := outbound.ConfigurePolicyProvider(testProviderEndpointOutboundPolicy{hosts: hosts})
	t.Cleanup(restore)
}

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
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		const prefix = "/v1/publishers/google/models/"
		const suffix = ":countTokens"
		if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, suffix) {
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
		seenModelsMu.Lock()
		seenModels[model] = true
		seenModelsMu.Unlock()
		if got := r.URL.Query().Get("key"); got != "google-cloud-key" {
			t.Fatalf("key query = %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "" {
			t.Fatalf("x-goog-api-key = %q, want empty", got)
		}
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Contents) != 1 || payload.Contents[0].Role != "user" || len(payload.Contents[0].Parts) != 1 || payload.Contents[0].Parts[0].Text == "" {
			t.Fatalf("unexpected request payload: %+v", payload)
		}
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

	handler := NewProviderEndpointHandler(nil, "production")
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

	handler := NewProviderEndpointHandler(nil, "production")
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

func TestProviderEndpointCreateRejectsLocalhostAPIURL(t *testing.T) {
	setProviderEndpointTestAllowHosts(t)
	if _, err := outbound.ValidateConfiguredEndpoint("http://127.0.0.1:8080/v1"); err == nil {
		t.Fatal("expected localhost base_url to be rejected")
	}
}

func TestRespondProviderEndpointConflict(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "duplicate name",
			err:     fmt.Errorf("wrapped: %w", ErrProviderEndpointNameConflict),
			wantMsg: "ProviderEndpoint 名称已存在，请使用其他名称",
		},
		{
			name:    "duplicate api type and base url",
			err:     fmt.Errorf("wrapped: %w", ErrProviderEndpointURLConflict),
			wantMsg: "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			if !respondProviderEndpointConflict(c, tt.err) {
				t.Fatal("respondProviderEndpointConflict returned false")
			}
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
	}
}

func TestProviderEndpointHandlerCreateDuplicateReturnsConflict(t *testing.T) {
	tests := []struct {
		name    string
		seed    *ProviderEndpoint
		request CreateProviderEndpointRequest
		wantMsg string
	}{
		{
			name: "duplicate name",
			seed: &ProviderEndpoint{
				APIType: "openai-compatible",
				Name:    "Duplicate Create",
				BaseURL: "http://127.0.0.1:19081/v1",
				Status:  1,
			},
			request: CreateProviderEndpointRequest{
				APIType: "openai-compatible",
				Name:    "duplicate create",
				BaseURL: "http://127.0.0.1:19082/v1",
				Status:  1,
			},
			wantMsg: "ProviderEndpoint 名称已存在，请使用其他名称",
		},
		{
			name: "duplicate api type and base url",
			seed: &ProviderEndpoint{
				APIType: "openai-compatible",
				Name:    "Duplicate URL A",
				BaseURL: "http://127.0.0.1:19083/v1",
				Status:  1,
			},
			request: CreateProviderEndpointRequest{
				APIType: "openai-compatible",
				Name:    "Duplicate URL B",
				BaseURL: "http://127.0.0.1:19083/v1",
				Status:  1,
			},
			wantMsg: "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, repo := newProviderEndpointHandlerTestRouter(t)
			seedProviderEndpoint(t, context.Background(), repo, tt.seed)

			rec := serveProviderEndpointJSON(t, router, http.MethodPost, "/provider-endpoints", tt.request)
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
	}
}

func TestProviderEndpointHandlerUpdateDuplicateReturnsConflict(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(first *ProviderEndpoint, second *ProviderEndpoint) UpdateProviderEndpointRequest
		wantMsg   string
		firstURL  string
		secondURL string
	}{
		{
			name: "duplicate name",
			mutate: func(first *ProviderEndpoint, second *ProviderEndpoint) UpdateProviderEndpointRequest {
				return UpdateProviderEndpointRequest{
					APIType: second.APIType,
					Name:    first.Name,
					BaseURL: second.BaseURL,
					Status:  second.Status,
				}
			},
			wantMsg:   "ProviderEndpoint 名称已存在，请使用其他名称",
			firstURL:  "http://127.0.0.1:19084/v1",
			secondURL: "http://127.0.0.1:19085/v1",
		},
		{
			name: "duplicate api type and base url",
			mutate: func(first *ProviderEndpoint, second *ProviderEndpoint) UpdateProviderEndpointRequest {
				return UpdateProviderEndpointRequest{
					APIType: second.APIType,
					Name:    second.Name,
					BaseURL: first.BaseURL,
					Status:  second.Status,
				}
			},
			wantMsg:   "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
			firstURL:  "http://127.0.0.1:19086/v1",
			secondURL: "http://127.0.0.1:19087/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, repo := newProviderEndpointHandlerTestRouter(t)
			first := seedProviderEndpoint(t, context.Background(), repo, &ProviderEndpoint{
				APIType: "openai-compatible",
				Name:    "Update First " + tt.name,
				BaseURL: tt.firstURL,
				Status:  1,
			})
			second := seedProviderEndpoint(t, context.Background(), repo, &ProviderEndpoint{
				APIType: "openai-compatible",
				Name:    "Update Second " + tt.name,
				BaseURL: tt.secondURL,
				Status:  1,
			})

			target := fmt.Sprintf("/provider-endpoints/%d", second.ID)
			rec := serveProviderEndpointJSON(t, router, http.MethodPut, target, tt.mutate(first, second))
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
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

func newProviderEndpointHandlerTestRouter(t *testing.T) (*gin.Engine, *ProviderEndpointRepository) {
	t.Helper()
	setProviderEndpointTestAllowHosts(t, "127.0.0.1")
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewProviderEndpointRepository(db)
	handler := NewProviderEndpointHandler(repo, "production")
	router := gin.New()
	router.POST("/provider-endpoints", handler.Create)
	router.PUT("/provider-endpoints/:id", handler.Update)
	return router, repo
}

func serveProviderEndpointJSON(t *testing.T, router http.Handler, method string, target string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertProviderEndpointConflictResponse(t *testing.T, rec *httptest.ResponseRecorder, wantMsg string) {
	t.Helper()
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, rec.Body.String())
	}
	if body.Code != http.StatusConflict {
		t.Fatalf("response code = %d, want 409", body.Code)
	}
	if body.Msg != wantMsg {
		t.Fatalf("response msg = %q, want %q", body.Msg, wantMsg)
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

	handler := NewProviderEndpointHandler(nil)

	gin.SetMode(gin.ReleaseMode)
	handler.appEnv = "development"
	if !handler.isDevLikeEnvironment() {
		t.Fatal("appEnv=development with release mode should be dev-like")
	}

	gin.SetMode(gin.ReleaseMode)
	handler.appEnv = "production"
	if handler.isDevLikeEnvironment() {
		t.Fatal("appEnv=production should not be dev-like")
	}

	gin.SetMode(gin.DebugMode)
	handler.appEnv = ""
	if !handler.isDevLikeEnvironment() {
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
	prodHandler := NewProviderEndpointHandler(nil, "production")
	if got := strings.Join(prodHandler.detectModelsAllowHosts(context.Background()), ","); got != "models.internal" {
		t.Fatalf("production allow hosts = %q, want models.internal", got)
	}

	devHandler := NewProviderEndpointHandler(nil, "development")
	got := strings.Join(devHandler.detectModelsAllowHosts(context.Background()), ",")
	for _, want := range []string{"models.internal", "localhost", "127.0.0.1", "::1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("development allow hosts = %q, missing %q", got, want)
		}
	}
}
