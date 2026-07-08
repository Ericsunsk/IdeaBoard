package providerendpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"ideaboard/api/internal/platform/outbound"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildModelsURL(t *testing.T) {
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
			got, err := buildModelsURL(tt.baseURL, tt.apiType)
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

func TestProviderEndpointCreateRejectsLocalhostAPIURL(t *testing.T) {
	t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "")
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
	t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "127.0.0.1")
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

	t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "models.internal")
	gin.SetMode(gin.ReleaseMode)
	prodHandler := NewProviderEndpointHandler(nil, "production")
	if got := strings.Join(prodHandler.detectModelsAllowHosts(), ","); got != "models.internal" {
		t.Fatalf("production allow hosts = %q, want models.internal", got)
	}

	devHandler := NewProviderEndpointHandler(nil, "development")
	got := strings.Join(devHandler.detectModelsAllowHosts(), ",")
	for _, want := range []string{"models.internal", "localhost", "127.0.0.1", "::1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("development allow hosts = %q, missing %q", got, want)
		}
	}
}
