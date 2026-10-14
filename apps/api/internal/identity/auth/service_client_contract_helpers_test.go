package auth

import (
	"encoding/json"
	"ideaboard/api/internal/platform/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const contractInternalKey = "contract-key"

func newContractServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

func newContractClient(server *httptest.Server) *ServiceClient {
	return NewServiceClient(config.AuthServiceConfig{
		BaseURL:        server.URL,
		InternalAPIKey: contractInternalKey,
		TimeoutSeconds: 1,
	})
}

func assertContractRequest(t *testing.T, r *http.Request, method, path string) {
	t.Helper()
	if r.Method != method {
		t.Fatalf("method = %s, want %s", r.Method, method)
	}
	if r.URL.Path != path {
		t.Fatalf("path = %s, want %s", r.URL.Path, path)
	}
	if got := r.Header.Get("X-Internal-Key"); got != contractInternalKey {
		t.Fatalf("X-Internal-Key = %q, want %q", got, contractInternalKey)
	}
}

func assertContractInternalKey(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("X-Internal-Key"); got != contractInternalKey {
		t.Fatalf("X-Internal-Key = %q, want %q", got, contractInternalKey)
	}
}

func assertJSONContentType(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func decodeContractBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body failed: %v", err)
	}
	return body
}

func assertContractField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	got, ok := body[key]
	if !ok {
		t.Fatalf("body missing %q field: %#v", key, body)
	}
	if got != want {
		t.Fatalf("body[%q] = %#v, want %#v", key, got, want)
	}
}

func writeContractJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func socialProviderContractEnvelope(version int) map[string]any {
	return map[string]any{
		"code": 0,
		"msg":  "success",
		"data": map[string]any{
			"provider": "google", "enabled": true, "configured": true, "available": true,
			"client_id": "google-client.apps.googleusercontent.com", "client_secret_configured": true,
			"scopes": []string{"openid", "email", "profile"}, "config_version": version,
			"callback_url": "https://example.com/api/auth/callback/google", "updated_at": "2026-08-06T00:00:00Z",
		},
	}
}
