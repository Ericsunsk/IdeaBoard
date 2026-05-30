package safehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestValidateURLRejectsLocalhostByDefault(t *testing.T) {
	if _, err := ValidateURL("http://localhost:6000/uploads/test.png"); err == nil {
		t.Fatal("expected localhost URL to be rejected by default")
	}
}

func TestValidateURLWithAllowHostsAllowsTrustedLocalhost(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://localhost:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err != nil {
		t.Fatalf("expected trusted localhost URL to pass validation, got %v", err)
	}
}

func TestValidateURLWithAllowHostsAllowsEquivalentLoopbackIP(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://127.0.0.1:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err != nil {
		t.Fatalf("expected equivalent loopback IP to pass validation, got %v", err)
	}
}

func TestValidateURLWithAllowHostsStillRejectsOtherPrivateHosts(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://10.0.0.8:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err == nil {
		t.Fatal("expected unmatched private host to be rejected")
	}
}

func TestGetWithAllowHostsUsesRestrictedDialerForTrustedLocalhost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	resp, err := GetWithAllowHosts(context.Background(), server.URL, time.Second, server.URL)
	if err != nil {
		t.Fatalf("expected trusted localhost fetch to succeed, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("expected response body to be readable, got %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("got body %q, want %q", string(body), "ok")
	}
}

func TestGetRejectsLocalhostWithoutAllowHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	if _, err := Get(context.Background(), server.URL, time.Second); err == nil {
		t.Fatal("expected localhost fetch without allow hosts to fail")
	}
}

func TestGetWithAllowHostsUsesCallerContextForPolicyResolve(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := GetWithAllowHosts(ctx, "http://example.com/image.png", time.Second)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDoRequestWithAllowHostsUsesCallerContextForPolicyResolve(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/image.png", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := DoRequestWithAllowHosts(ctx, req, time.Second)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRedirectValidationRejectsPrivateRedirectTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.8/private", http.StatusFound)
	}))
	defer server.Close()

	resp, err := GetWithAllowHosts(context.Background(), server.URL, time.Second, server.URL)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected private redirect target to be rejected")
	}
}

func TestProxyDialAllowanceAllowsConfiguredLoopbackProxy(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	policy := &resolvedPolicy{
		allowedHosts: map[string]bool{},
		allowedIPs:   map[string]bool{},
	}

	addProxyDialAllowance(context.Background(), policy, proxyURL)

	addrs := []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}
	if _, err := filterAllowedIPs("127.0.0.1", addrs, policy.allowedHosts, policy.allowedIPs); err != nil {
		t.Fatalf("expected configured loopback proxy to be allowed, got %v", err)
	}
}
