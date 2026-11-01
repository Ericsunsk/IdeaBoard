package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"context"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
)

type stubOutboundPolicy struct {
	hosts []string
	err   error
}

func (s stubOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	if s.err != nil {
		return settings.OutboundPolicySetting{}, s.err
	}
	return settings.OutboundPolicySetting{AllowHosts: s.hosts}, nil
}

func TestProbeWorkerConnectionRejectsURLMetadata(t *testing.T) {
	for _, baseURL := range []string{
		"https://user:secret@example.com",
		"https://example.com/path?token=secret",
		"https://example.com/path#fragment",
	} {
		t.Run(baseURL, func(t *testing.T) {
			result := ProbeWorkerConnection(t.Context(), baseURL, "secret", "openai/gpt-image-1")
			if result.Reachable || result.Message != "Worker 地址必须是 http(s) URL" {
				t.Fatalf("ProbeWorkerConnection() = %+v, want URL validation failure", result)
			}
		})
	}
}

func TestProbeWorkerConnectionDryRunReachableOnKnownPathSource(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	var capturedPath string
	var capturedBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"provider":"fal","model":"m","target":{"url":"https://up.example/v1","method":"POST","body":{}},"path_source":"fal:text-to-image"}`))
	}))
	defer upstream.Close()

	result := ProbeWorkerConnection(t.Context(), upstream.URL, "secret", "fal/m")
	if !result.Reachable || result.StatusCode != http.StatusOK {
		t.Fatalf("ProbeWorkerConnection() dry-run = %+v, want reachable 200", result)
	}
	if capturedPath != "/v1/dry-run" {
		t.Fatalf("dry-run path = %q, want /v1/dry-run", capturedPath)
	}
	for _, key := range []string{"task_id", "provider", "model", "prompt"} {
		if _, ok := capturedBody[key]; !ok {
			t.Fatalf("dry-run body missing %q: %v", key, capturedBody)
		}
	}
	if _, ok := capturedBody["provider_options"]; ok {
		t.Fatalf("dry-run body must omit empty provider_options: %v", capturedBody)
	}
}

func TestProbeWorkerConnectionDryRunUnreachableOnNon200(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	// Any non-200 dry-run proves no usable routing: unreachable, status kept.
	result := ProbeWorkerConnection(t.Context(), upstream.URL, "secret", "openai/gpt-image-1")
	if result.Reachable || result.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ProbeWorkerConnection() dry-run = %+v, want unreachable 401", result)
	}
}

func TestProbeWorkerConnectionDryRunReachableOnUnknownPathSource(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"provider":"openai","model":"m","target":{"url":"https://up.example/v1","method":"POST","body":{}},"path_source":"bogus:unexplainable"}`))
	}))
	defer upstream.Close()

	result := ProbeWorkerConnection(t.Context(), upstream.URL, "secret", "openai/gpt-image-1")
	if !result.Reachable || result.StatusCode != http.StatusOK {
		t.Fatalf("ProbeWorkerConnection() dry-run = %+v, want reachable 200 on unknown path_source", result)
	}
}

// Go 不枚举 path_source：Worker 能回 200 + 非空来源即证明认识该路由，
// 未来新增来源无需改 Go。此单测用从未存在过的来源锁死该设计。
func TestProbeWorkerConnectionDryRunReachableOnFuturePathSource(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"provider":"airun","model":"m","target":{"url":"https://up.example/v1","method":"POST","body":{}},"path_source":"airun:future:whatever"}`))
	}))
	defer upstream.Close()

	result := ProbeWorkerConnection(t.Context(), upstream.URL, "secret", "openai/gpt-image-1")
	if !result.Reachable || result.StatusCode != http.StatusOK {
		t.Fatalf("ProbeWorkerConnection() dry-run = %+v, want reachable 200 on future path_source", result)
	}
}

func TestProbeWorkerConnectionDryRunUnreachableOnEmptyPathSource(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"provider":"openai","model":"m","target":{"url":"https://up.example/v1","method":"POST","body":{}}}`))
	}))
	defer upstream.Close()

	result := ProbeWorkerConnection(t.Context(), upstream.URL, "secret", "openai/gpt-image-1")
	if result.Reachable || result.StatusCode != http.StatusOK {
		t.Fatalf("ProbeWorkerConnection() dry-run = %+v, want unreachable 200 on empty path_source", result)
	}
}

func TestProbeWorkerConnectionTransportFailure(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	result := ProbeWorkerConnection(t.Context(), "http://127.0.0.1:9", "secret", "openai/gpt-image-1")
	if result.Reachable {
		t.Fatalf("ProbeWorkerConnection() unreachable = %+v, want reachable:false", result)
	}
	if !strings.Contains(result.Message, errorrender.MsgProbeWorkerConnection5) {
		t.Fatalf("ProbeWorkerConnection() message = %q, want transport-failure prefix", result.Message)
	}
}

func TestProbeWorkerConnectionRequiresModel(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	result := ProbeWorkerConnection(t.Context(), "https://worker.example.com", "secret", "")
	if result.Reachable {
		t.Fatalf("ProbeWorkerConnection() empty model = %+v, want unreachable", result)
	}
}

func TestValidateWorkerModel(t *testing.T) {
	provider, model, err := ValidateWorkerModel("openai/gpt-image-1", "")
	if err != nil || provider != "openai" || model != "gpt-image-1" {
		t.Fatalf("ValidateWorkerModel = %q/%q, %v; want openai/gpt-image-1, nil", provider, model, err)
	}
	if _, _, err := ValidateWorkerModel("bare-model", ""); err == nil {
		t.Fatal("ValidateWorkerModel(bare) = nil error, want fail-closed")
	}
	if _, _, err := ValidateWorkerModel("openai/gpt-image-1", "{invalid"); err == nil {
		t.Fatal("ValidateWorkerModel(bad params) = nil error, want fail-closed")
	}
	// Non-object JSON follows the shared workflow.ParseProviderOptions rule:
	// ignored, never an error (execution behaves identically).
	if _, _, err := ValidateWorkerModel("openai/gpt-image-1", "[1,2]"); err != nil {
		t.Fatalf("ValidateWorkerModel(non-object params) = %v, want ignored", err)
	}
	if _, _, err := ValidateWorkerModel("openai/gpt-image-1", `{"quality":"high"}`); err != nil {
		t.Fatalf("ValidateWorkerModel(object params) = %v, want nil", err)
	}
}

func TestPreviewWorkerRouteValidationOnly(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	preview := PreviewWorkerRoute(t.Context(), "", "", "openai/gpt-image-1", "")
	if !preview.Valid || preview.Reachable {
		t.Fatalf("preview = %+v, want valid validation-only", preview)
	}
	preview = PreviewWorkerRoute(t.Context(), "", "", "bare-model", "")
	if preview.Valid {
		t.Fatalf("preview = %+v, want invalid ref", preview)
	}
}
