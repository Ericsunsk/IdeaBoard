package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestProbeGatewayConnectionRejectsURLMetadata(t *testing.T) {
	for _, baseURL := range []string{
		"https://user:secret@example.com",
		"https://example.com/path?token=secret",
		"https://example.com/path#fragment",
	} {
		t.Run(baseURL, func(t *testing.T) {
			result := ProbeGatewayConnection(t.Context(), baseURL, "", "openai", "gpt-4o", "v1/models")
			if result.Reachable || result.Message != "网关地址必须是 http(s) URL" {
				t.Fatalf("ProbeGatewayConnection() = %+v, want URL validation failure", result)
			}
		})
	}
}

func TestProbeGatewayConnectionRootURLReachableOnAnyStatus(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	var capturedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	result := ProbeGatewayConnection(t.Context(), upstream.URL, "", "", "", "")
	if !result.Reachable || result.StatusCode != http.StatusNotFound {
		t.Fatalf("ProbeGatewayConnection() root = %+v, want reachable 404", result)
	}
	if capturedPath != "/" {
		t.Fatalf("root probe path = %q, want /", capturedPath)
	}
}

func TestProbeGatewayConnectionRootURLTransportFailure(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	result := ProbeGatewayConnection(t.Context(), "http://127.0.0.1:9", "", "", "", "")
	if result.Reachable {
		t.Fatalf("ProbeGatewayConnection() root unreachable = %+v, want reachable:false", result)
	}
	if !strings.Contains(result.Message, errorrender.MsgProbeGatewayConnection5) {
		t.Fatalf("ProbeGatewayConnection() root message = %q, want transport-failure prefix", result.Message)
	}
}

func TestProbeGatewayConnectionPartialRouteRejected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		path     string
	}{
		{"provider without path", "openai", ""},
		{"path without provider", "", "v1/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ProbeGatewayConnection(t.Context(), "https://gateway.example.com", "", tc.provider, "", tc.path)
			if result.Reachable || result.Message != errorrender.MsgProbeGatewayConnection3 {
				t.Fatalf("ProbeGatewayConnection() partial = %+v, want %q", result, errorrender.MsgProbeGatewayConnection3)
			}
		})
	}
}
