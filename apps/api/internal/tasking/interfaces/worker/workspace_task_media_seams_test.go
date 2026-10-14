package worker

// Production media seam tests: the allowlisted result fetcher behind the
// worker finalize path. All bytes
// are local fixtures served by httptest; no secrets, no live network.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// pngHitServer serves the fixture PNG and counts inbound requests so
// reject-path tests can prove a non-allowlisted URL is never fetched.
func pngHitServer(t *testing.T, hits *int64, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
}

func fetcherTestRef(rawURL string) workspacetasks.OutputRef {
	return workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindRemote, URL: rawURL,
		Role: "result", ResultIndex: 0,
	}
}

// Allowlisted host (exact host:port from the executor policy) downloads.
func TestWorkspaceTaskResultFetcherAllowsListedHost(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{parsed.Host})
	data, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err != nil {
		t.Fatalf("FetchResultBytes: %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Fatalf("fetched %d bytes, want the %d fixture bytes", len(data), len(png))
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// Bare-host entries match any port (outbound policy normalization parity).
func TestWorkspaceTaskResultFetcherAllowsBareHostEntry(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{parsed.Hostname()})
	if _, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png")); err != nil {
		t.Fatalf("FetchResultBytes: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// Non-allowlisted hosts fail closed with an explicit error and are never
// fetched (hits stay 0); the error names the host but echoes no path.
func TestWorkspaceTaskResultFetcherRejectsNonAllowlistedHost(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{"cdn.example.com"})
	_, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("err = %v, want explicit not-allowlisted failure", err)
	}
	if strings.Contains(err.Error(), "/result.png") {
		t.Fatalf("err echoes the URL path: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Fatalf("server hits = %d, want 0 (never fetched)", got)
	}
}

type emptyOutboundPolicy struct{}

func (emptyOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{}, nil
}

// No allowlist anywhere (empty ctx override + empty outbound policy)
// fails closed before any network.
func TestWorkspaceTaskResultFetcherFailsClosedWithoutAllowlist(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(emptyOutboundPolicy{})
	defer restore()
	var hits int64
	server := pngHitServer(t, &hits, fixturePNGBytes(t))
	defer server.Close()
	_, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(context.Background(), rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err == nil || !strings.Contains(err.Error(), "allowlist is not configured") {
		t.Fatalf("err = %v, want fail-closed allowlist failure", err)
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Fatalf("server hits = %d, want 0 (never fetched)", got)
	}
}

// Closed-world matching unit table: exact-host semantics, entry shapes,
// and fail-closed inputs. No network.
func TestWorkspaceTaskResultHostAllowlistMatching(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rawURL     string
		allowHosts []string
		wantErr    string
	}{
		{name: "exact host", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "exact host:port", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com:8443"}},
		{name: "port mismatch fails", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com:9443"}, wantErr: "not allowlisted"},
		{name: "bare host matches explicit port", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "case insensitive", rawURL: "https://CDN.Example.COM/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "full URL entry shape", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"https://cdn.example.com/some/path"}},
		{name: "full URL default port is exact", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"https://cdn.example.com/some/path"}, wantErr: "not allowlisted"},
		{name: "lookalike host fails", rawURL: "https://cdn.example.com.attacker.example/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "not allowlisted"},
		{name: "unlisted host fails", rawURL: "https://other.example.org/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "not allowlisted"},
		{name: "empty allowlist fails closed", rawURL: "https://cdn.example.com/a.png", allowHosts: nil, wantErr: "not configured"},
		{name: "blank entries do not count", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"  ", ""}, wantErr: "not configured"},
		{name: "empty URL", rawURL: "", allowHosts: []string{"cdn.example.com"}, wantErr: "is empty"},
		{name: "non-http scheme fails", rawURL: "ftp://cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "is not allowed"},
		{name: "data URL fails", rawURL: "data:image/png;base64,abc", allowHosts: []string{"cdn.example.com"}, wantErr: "invalid"},
		{name: "userinfo fails", rawURL: "https://user:pass@cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := taskruntime.CheckWorkspaceResultHostAllowlist(tc.rawURL, tc.allowHosts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("check = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("check = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
