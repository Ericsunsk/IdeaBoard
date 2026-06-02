package main

import "testing"

func TestHealthcheckURLUsesExplicitURL(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "http://127.0.0.1:7000/ready")
	t.Setenv("APP_PORT", "6000")

	if got, want := healthcheckURL(), "http://127.0.0.1:7000/ready"; got != want {
		t.Fatalf("healthcheckURL() = %q, want %q", got, want)
	}
}

func TestHealthcheckURLDefaultsToAppPort(t *testing.T) {
	t.Setenv("APP_PORT", "7000")

	if got, want := healthcheckURL(), "http://127.0.0.1:7000/api/health"; got != want {
		t.Fatalf("healthcheckURL() = %q, want %q", got, want)
	}
}

func TestHealthcheckURLDefaultsToPort6000(t *testing.T) {
	if got, want := healthcheckURL(), "http://127.0.0.1:6000/api/health"; got != want {
		t.Fatalf("healthcheckURL() = %q, want %q", got, want)
	}
}
