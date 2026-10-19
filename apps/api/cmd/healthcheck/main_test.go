package main

import "testing"

func TestHealthcheckURLUsesExplicitURL(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "http://127.0.0.1:7000/ready")

	if got, want := healthcheckURL(), "http://127.0.0.1:7000/ready"; got != want {
		t.Fatalf("healthcheckURL() = %q, want %q", got, want)
	}
}

func TestHealthcheckURLDefaultsToPort6000(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "")

	if got, want := healthcheckURL(), "http://127.0.0.1:6000/api/health"; got != want {
		t.Fatalf("healthcheckURL() = %q, want %q", got, want)
	}
}

func TestProbeKindDefaultsToHTTP(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "")
	t.Setenv("API_MODE", "")

	if got := probeKind(); got != probeHTTP {
		t.Fatalf("probeKind() = %q, want http", got)
	}
}

func TestProbeKindWorkerModeUsesDatabase(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "")
	t.Setenv("API_MODE", "worker-only")

	if got := probeKind(); got != probeWorkerDB {
		t.Fatalf("probeKind() = %q, want worker-db", got)
	}
}

func TestProbeKindExplicitURLWinsOverWorkerMode(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "http://127.0.0.1:6000/api/health")
	t.Setenv("API_MODE", "worker-only")

	if got := probeKind(); got != probeHTTP {
		t.Fatalf("probeKind() = %q, want http (explicit URL wins)", got)
	}
}
