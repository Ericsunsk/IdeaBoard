package safehttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
