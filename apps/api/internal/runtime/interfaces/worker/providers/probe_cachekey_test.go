package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providerinfra "ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
)

type gatewayCapture struct {
	method      string
	path        string
	header      http.Header
	body        []byte
	contentType string
}

func newGatewayCaptureServer(t *testing.T, capture *gatewayCapture, status int, responseHeaders map[string]string, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.method = r.Method
		capture.path = r.URL.Path
		capture.header = r.Header.Clone()
		capture.contentType = r.Header.Get("Content-Type")
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		capture.body = payload
		for key, value := range responseHeaders {
			w.Header().Set(key, value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
}

func TestProbeModelSuccess(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK,
		map[string]string{"cf-aig-model": "openai/gpt-4o-mini"}, `{"data":[]}`)
	defer server.Close()

	client, err := providerinfra.NewGatewayClient(providerinfra.GatewayClientConfig{BaseURL: server.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("providerinfra.NewGatewayClient() error = %v", err)
	}
	verdict, err := ProbeModel(context.Background(), client, "openrouter", "google/gemini-3.1-flash-lite-image", "api/v1/models", 5*time.Second)
	if err != nil {
		t.Fatalf("ProbeModel() error = %v", err)
	}
	if !verdict.Reachable || verdict.StatusCode != http.StatusOK {
		t.Fatalf("verdict = %+v, want reachable 200", verdict)
	}
	if capture.method != http.MethodGet {
		t.Fatalf("method = %s, want GET", capture.method)
	}
	if capture.path != "/openrouter/api/v1/models" {
		t.Fatalf("path = %q, want provider-native probe path", capture.path)
	}
	if got := capture.header.Get("cf-aig-max-attempts"); got != "1" {
		t.Fatalf("max-attempts = %q, want single-shot", got)
	}
	if got := capture.header.Get("cf-aig-skip-cache"); got != "true" {
		t.Fatalf("skip-cache = %q, want true", got)
	}
}

func TestProbeModelErrorResponseIsReachable(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusUnauthorized, nil, `{"error":"bad key"}`)
	defer server.Close()

	client, err := providerinfra.NewGatewayClient(providerinfra.GatewayClientConfig{BaseURL: server.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("providerinfra.NewGatewayClient() error = %v", err)
	}
	verdict, err := ProbeModel(context.Background(), client, "openrouter", "google/gemini-3.1-flash-lite-image", "api/v1/models", 5*time.Second)
	if err == nil {
		t.Fatal("ProbeModel() succeeded on 401, want error")
	}
	// An HTTP response — even an error — proves the target is reachable.
	// Only transport failures report unreachable.
	if !verdict.Reachable || verdict.StatusCode != http.StatusUnauthorized {
		t.Fatalf("verdict = %+v, want reachable 401", verdict)
	}
	if !strings.Contains(verdict.BodyPreview, "bad key") {
		t.Fatalf("preview = %q, want raw error for capability verdicts", verdict.BodyPreview)
	}
}

func TestProbeModelRejectsBadInput(t *testing.T) {
	client, err := providerinfra.NewGatewayClient(providerinfra.GatewayClientConfig{BaseURL: "http://127.0.0.1:1", Token: "test-token"})
	if err != nil {
		t.Fatalf("providerinfra.NewGatewayClient() error = %v", err)
	}
	if _, err := ProbeModel(context.Background(), nil, "openrouter", "m", "api/v1/models", time.Second); err == nil {
		t.Fatal("nil client accepted")
	}
	if _, err := ProbeModel(context.Background(), client, "openai", "m", "", time.Second); err == nil {
		t.Fatal("empty path accepted")
	}
}
