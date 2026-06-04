package channels

import (
	"fmt"
	"ideaboard/api/internal/platform/outbound"
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

func TestChannelCreateRejectsLocalhostAPIURL(t *testing.T) {
	t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "")
	if _, err := outbound.ValidateConfiguredEndpoint("http://127.0.0.1:8080/v1"); err == nil {
		t.Fatal("expected localhost api_url to be rejected")
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

	handler := NewChannelHandler(nil)

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
	prodHandler := NewChannelHandler(nil, "production")
	if got := strings.Join(prodHandler.detectModelsAllowHosts(), ","); got != "models.internal" {
		t.Fatalf("production allow hosts = %q, want models.internal", got)
	}

	devHandler := NewChannelHandler(nil, "development")
	got := strings.Join(devHandler.detectModelsAllowHosts(), ",")
	for _, want := range []string{"models.internal", "localhost", "127.0.0.1", "::1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("development allow hosts = %q, missing %q", got, want)
		}
	}
}
