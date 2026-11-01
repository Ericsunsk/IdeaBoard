package safehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustConfiguredEndpoint(t *testing.T, raw string) ConfiguredEndpoint {
	t.Helper()
	endpoint, err := ParseConfiguredEndpoint(raw)
	if err != nil {
		t.Fatalf("ParseConfiguredEndpoint(%q) error = %v", raw, err)
	}
	return endpoint
}

func TestParseConfiguredEndpointValidation(t *testing.T) {
	for _, raw := range []string{
		"https://worker.internal",
		"http://127.0.0.1:6001",
		"https://[::1]:8443",
		"http://localhost:6000/v1",
	} {
		endpoint, err := ParseConfiguredEndpoint(raw)
		if err != nil {
			t.Fatalf("ParseConfiguredEndpoint(%q) error = %v, want nil", raw, err)
		}
		if !endpoint.Valid() {
			t.Fatalf("ParseConfiguredEndpoint(%q) is invalid, want valid", raw)
		}
	}
	for _, raw := range []string{
		"",
		"   ",
		"ftp://worker.internal",
		"gopher://127.0.0.1:70",
		"https://",
		"://missing-scheme",
	} {
		if endpoint, err := ParseConfiguredEndpoint(raw); err == nil || endpoint.Valid() {
			t.Fatalf("ParseConfiguredEndpoint(%q) = %#v, %v; want rejection", raw, endpoint, err)
		}
	}
}

func TestParseConfiguredEndpointNormalizesBase(t *testing.T) {
	endpoint := mustConfiguredEndpoint(t, "http://127.0.0.1:6001/")
	if got := endpoint.BaseURL(); got != "http://127.0.0.1:6001" {
		t.Fatalf("BaseURL = %q, want trimmed base", got)
	}
	if got := endpoint.Hostname(); got != "127.0.0.1" {
		t.Fatalf("Hostname = %q, want 127.0.0.1", got)
	}
}

func TestConfiguredClientReachesLoopbackWithoutAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		MaxAttempts: 1,
		Configured:  mustConfiguredEndpoint(t, server.URL),
		Target:      "configured-test",
	})
	if err != nil {
		t.Fatalf("configured loopback Do() error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestConfiguredClientRejectsOffEndpointHost(t *testing.T) {
	var otherHits atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer origin.Close()

	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, other), RequestOptions{
		MaxAttempts: 1,
		Configured:  mustConfiguredEndpoint(t, origin.URL),
		Target:      "configured-test",
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("off-endpoint Do() error = %v, want ErrPolicyViolation", err)
	}
	if got := otherHits.Load(); got != 0 {
		t.Fatalf("off-endpoint hits = %d, want 0 (fail closed before send)", got)
	}
}

func TestConfiguredClientRejectsCrossHostRedirect(t *testing.T) {
	var finalHits atomic.Int64
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHits.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer final.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, origin), RequestOptions{
		MaxAttempts: 1,
		Configured:  mustConfiguredEndpoint(t, origin.URL),
		Target:      "configured-test",
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("cross-host redirect Do() error = %v, want ErrPolicyViolation", err)
	}
	if got := finalHits.Load(); got != 0 {
		t.Fatalf("cross-host redirect final hits = %d, want 0", got)
	}
}

func TestStrictClientRejectsLoopbackWithoutAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		MaxAttempts: 1,
		Target:      "strict-test",
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("unlisted loopback Do() error = %v, want ErrPolicyViolation", err)
	}
}

func TestStrictRedirectChainRevalidatesEveryHop(t *testing.T) {
	var finalHits atomic.Int64
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHits.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer final.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	client, _ := testClient(t, Config{})
	// Only the origin hop is allowlisted: a first-hop-only check would let
	// the redirect through, per-hop validation must fail it closed.
	resp, err := client.Do(context.Background(), postRequest(t, origin), RequestOptions{
		MaxAttempts: 1,
		AllowHosts:  []string{origin.URL},
		Target:      "strict-test",
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("unlisted redirect hop Do() error = %v, want ErrPolicyViolation", err)
	}
	if got := finalHits.Load(); got != 0 {
		t.Fatalf("unlisted redirect hop final hits = %d, want 0", got)
	}
}

func TestAmbiguousEgressPolicyFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		MaxAttempts: 1,
		AllowHosts:  []string{server.URL},
		Configured:  mustConfiguredEndpoint(t, server.URL),
		Target:      "ambiguous-test",
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("ambiguous policy Do() error = %v, want ErrPolicyViolation", err)
	}
}

func TestParseConfiguredEndpointRejectsUserinfoHost(t *testing.T) {
	endpoint := mustConfiguredEndpoint(t, "http://127.0.0.1:6001")
	req, err := http.NewRequest(http.MethodGet, "http://user@127.0.0.1:6001/v1", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if err := checkConfiguredTarget(req.URL, endpoint); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("userinfo target check = %v, want ErrPolicyViolation", err)
	}
	if strings.TrimSpace(endpoint.BaseURL()) == "" {
		t.Fatal("expected non-empty endpoint base")
	}
}

func TestConfiguredClientClosesCompletedConnections(t *testing.T) {
	const requests = 12
	closed := make(chan struct{}, requests)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "complete response")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	client, _ := testClient(t, Config{})
	endpoint := mustConfiguredEndpoint(t, server.URL)
	for i := 0; i < requests; i++ {
		resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{MaxAttempts: 1, Configured: endpoint})
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil || string(body) != "complete response" {
			t.Fatalf("response = %q, read = %v, close = %v", body, readErr, closeErr)
		}
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for i := 0; i < requests; i++ {
		select {
		case <-closed:
		case <-deadline.C:
			t.Fatalf("only %d of %d completed connections closed", i, requests)
		}
	}
}

func TestConfiguredClientKeepsStreamingBodyReadable(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "second")
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	client, _ := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{MaxAttempts: 1, Configured: mustConfiguredEndpoint(t, server.URL)})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	first := make([]byte, len("first"))
	if _, err := io.ReadFull(resp.Body, first); err != nil || string(first) != "first" {
		t.Fatalf("first = %q, err = %v", first, err)
	}
	release <- struct{}{}
	rest, err := io.ReadAll(resp.Body)
	if err != nil || string(rest) != "second" {
		t.Fatalf("rest = %q, err = %v", rest, err)
	}
}
