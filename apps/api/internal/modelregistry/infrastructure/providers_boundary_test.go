package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// The Worker suite checks this fixture against its real /v1/providers handler.
func TestConfiguredGWLLMMatchesWorkerCapabilities(t *testing.T) {
	capabilities, err := os.ReadFile("../../platform/workerwire/testdata/providers.json")
	if err != nil {
		t.Fatal(err)
	}
	cf := serveCFProviders(t, []string{"gwllm", "unadapted"}, http.StatusOK, nil)
	defer cf.Close()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/providers" || r.Header.Get("Authorization") != "Bearer worker-secret" {
			t.Errorf("unexpected Worker discovery request: %s", r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(capabilities)
	}))
	defer worker.Close()
	repo := discoveryTestRepo(nil, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "cf-token",
		WorkerBaseURL: worker.URL, WorkerSecret: "worker-secret",
	}, cf.URL)
	providers, err := repo.ListConfiguredProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range providers {
		switch provider.Slug {
		case "gwllm":
			if !provider.Available || provider.IDPrefix() != "gwllm/" {
				t.Fatalf("GWLLM = %+v; want selectable with the configured provider identity", provider)
			}
		case "unadapted":
			if provider.Available {
				t.Fatal("an unadapted provider must remain unavailable")
			}
		case "workers-ai":
		default:
			t.Fatalf("unexpected provider: %+v", provider)
		}
	}
	if len(providers) != 3 {
		t.Fatalf("providers = %+v; want GWLLM, unadapted and workers-ai", providers)
	}
}
