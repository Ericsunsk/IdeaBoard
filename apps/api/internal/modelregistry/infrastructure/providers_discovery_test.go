package infrastructure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/modelregistry/domain"
)

type discoveryCatalog []domain.CatalogItem

func (items discoveryCatalog) ListCatalog(context.Context) ([]domain.CatalogItem, error) {
	return append([]domain.CatalogItem(nil), items...), nil
}

func discoveryTestRepo(catalog []domain.CatalogItem, cfg DiscoveryConfig, controlPlaneURL string) *ConfiguredProviders {
	return newConfiguredProviders(discoveryCatalog(catalog), cfg, controlPlaneURL,
		func(context.Context) []string { return []string{"127.0.0.1"} })
}

func serveCFProviders(t *testing.T, slugs []string, status int, check func(r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		if !strings.HasPrefix(r.URL.Path, "/client/v4/accounts/aid/ai-gateway/gateways/gid/provider_configs") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[`))
		for i, s := range slugs {
			if i > 0 {
				_, _ = w.Write([]byte(`,`))
			}
			_, _ = w.Write([]byte(`{"provider_slug":"` + s + `","id":"` + s + `-1"}`))
		}
		_, _ = w.Write([]byte(`]}`))
	}))
}

func serveWorkerProviders(t *testing.T, slugs []string, status int, check func(r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		if r.URL.Path != "/v1/providers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			// 401 bodies carry no task_id by contract; the VPS treats every
			// non-200 as a degradation trigger without parsing.
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"unauthorized"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":[`))
		for i, s := range slugs {
			if i > 0 {
				_, _ = w.Write([]byte(`,`))
			}
			_, _ = w.Write([]byte(`"` + s + `"`))
		}
		_, _ = w.Write([]byte(`]}`))
	}))
}

func providersBySlug(t *testing.T, repo *ConfiguredProviders) map[string]bool {
	t.Helper()
	providers, err := repo.ListConfiguredProviders(context.Background())
	if err != nil {
		t.Fatalf("ListConfiguredProviders() error = %v", err)
	}
	bySlug := make(map[string]bool, len(providers))
	for _, p := range providers {
		bySlug[p.Slug] = p.Available
	}
	return bySlug
}

func TestListConfiguredProvidersMarksCFWorkerIntersectionAvailable(t *testing.T) {
	var cfAuth, workerAuth string
	cf := serveCFProviders(t, []string{"openai", "fal", "krea"}, http.StatusOK, func(r *http.Request) {
		cfAuth = r.Header.Get("Authorization")
	})
	defer cf.Close()
	worker := serveWorkerProviders(t, []string{"fal", "google", "krea", "openai", "workers-ai"}, http.StatusOK, func(r *http.Request) {
		workerAuth = r.Header.Get("Authorization")
	})
	defer worker.Close()

	repo := discoveryTestRepo([]domain.CatalogItem{{ID: "m", Provider: "stale-catalog"}}, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "cf-aig-token",
		WorkerBaseURL: worker.URL, WorkerSecret: "worker-secret",
	}, cf.URL)

	bySlug := providersBySlug(t, repo)
	// CF ∩ Worker = available; krea is in both so it is available.
	for _, slug := range []string{"openai", "fal", "krea", "workers-ai"} {
		avail, ok := bySlug[slug]
		if !ok {
			t.Fatalf("providers = %v, want %q listed", bySlug, slug)
		}
		if !avail {
			t.Fatalf("providers = %v, want %q available", bySlug, slug)
		}
	}
	// Worker-only slugs never surface.
	if _, ok := bySlug["google"]; ok {
		t.Fatalf("providers = %v, want Worker-only google hidden", bySlug)
	}
	if cfAuth != "Bearer cf-aig-token" {
		t.Fatalf("CF Authorization = %q, want shared gateway bearer", cfAuth)
	}
	if workerAuth != "Bearer worker-secret" {
		t.Fatalf("Worker Authorization = %q, want internal-secret bearer", workerAuth)
	}
}

func TestListConfiguredProvidersMarksCFOnlyAsUnadapted(t *testing.T) {

	cf := serveCFProviders(t, []string{"openai", "krea"}, http.StatusOK, nil)
	defer cf.Close()
	worker := serveWorkerProviders(t, []string{"openai", "workers-ai"}, http.StatusOK, nil)
	defer worker.Close()

	repo := discoveryTestRepo(nil, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "tok",
		WorkerBaseURL: worker.URL, WorkerSecret: "s",
	}, cf.URL)

	bySlug := providersBySlug(t, repo)
	if avail, ok := bySlug["openai"]; !ok || !avail {
		t.Fatalf("providers = %v, want openai available", bySlug)
	}
	// CF has krea but the Worker has no adapter: shown, marked unadapted.
	if avail, ok := bySlug["krea"]; !ok {
		t.Fatalf("providers = %v, want CF-only krea still listed", bySlug)
	} else if avail {
		t.Fatalf("providers = %v, want krea unavailable (no adapter)", bySlug)
	}
	if avail, ok := bySlug["workers-ai"]; !ok || !avail {
		t.Fatalf("providers = %v, want workers-ai constant available", bySlug)
	}
}

func TestListConfiguredProvidersDegradesWhenCFIsDown(t *testing.T) {

	cf := serveCFProviders(t, nil, http.StatusInternalServerError, nil)
	defer cf.Close()
	worker := serveWorkerProviders(t, []string{"openai", "workers-ai"}, http.StatusOK, nil)
	defer worker.Close()

	repo := discoveryTestRepo([]domain.CatalogItem{
		{ID: "m1", Name: "M1", Provider: "openai"},
		{ID: "m2", Name: "M2", Provider: "OpenAI"},
		{ID: "m3", Name: "M3", Provider: "custom-co"},
	}, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "tok",
		WorkerBaseURL: worker.URL, WorkerSecret: "s",
	}, cf.URL)

	bySlug := providersBySlug(t, repo)
	// Degraded to the catalog fallback: distinct catalog slugs plus
	// workers-ai, all available; the CF-only krea never appears.
	for _, slug := range []string{"openai", "custom-co", "workers-ai"} {
		avail, ok := bySlug[slug]
		if !ok {
			t.Fatalf("providers = %v, want fallback %q", bySlug, slug)
		}
		if !avail {
			t.Fatalf("providers = %v, want fallback %q available", bySlug, slug)
		}
	}
	if len(bySlug) != 3 {
		t.Fatalf("providers = %v, want exactly the catalog fallback set", bySlug)
	}
}

func TestListConfiguredProvidersDegradesWhenWorkerIsDown(t *testing.T) {

	cf := serveCFProviders(t, []string{"openai", "fal"}, http.StatusOK, nil)
	defer cf.Close()
	// Worker unreachable (connection refused): non-200/transport failure
	// must degrade, never fail the listing.
	worker := serveWorkerProviders(t, nil, http.StatusUnauthorized, nil)
	worker.Close()

	repo := discoveryTestRepo([]domain.CatalogItem{
		{ID: "m1", Name: "M1", Provider: "openai"},
	}, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "tok",
		WorkerBaseURL: worker.URL, WorkerSecret: "s",
	}, cf.URL)

	bySlug := providersBySlug(t, repo)
	if avail, ok := bySlug["openai"]; !ok || !avail {
		t.Fatalf("providers = %v, want catalog fallback openai", bySlug)
	}
	if avail, ok := bySlug["workers-ai"]; !ok || !avail {
		t.Fatalf("providers = %v, want workers-ai constant", bySlug)
	}
	// CF-only fal must not leak through the fallback: fallback membership
	// is catalog-driven.
	if _, ok := bySlug["fal"]; ok {
		t.Fatalf("providers = %v, want no CF leakage in fallback", bySlug)
	}
}

func TestListConfiguredProvidersFallbackWithoutDiscovery(t *testing.T) {
	repo := discoveryTestRepo([]domain.CatalogItem{
		{ID: "m1", Name: "M1", Provider: "openai"},
	}, DiscoveryConfig{}, "")
	// Unconfigured discovery falls back without network.
	bySlug := providersBySlug(t, repo)
	if avail, ok := bySlug["openai"]; !ok || !avail {
		t.Fatalf("providers = %v, want catalog fallback openai", bySlug)
	}
	if avail, ok := bySlug["workers-ai"]; !ok || !avail {
		t.Fatalf("providers = %v, want workers-ai constant", bySlug)
	}
}

func TestListConfiguredProvidersDegradesOnMalformedWorkerCapabilities(t *testing.T) {

	cf := serveCFProviders(t, []string{"fal"}, http.StatusOK, nil)
	defer cf.Close()

	for _, payload := range []string{`{}`, `null`, `{"providers":null}`, `{"providers":"fal"}`, `{"providers":[1]}`, `{"providers":[null]}`, `{"providers":[""]}`, `{"providers":["  "]}`, `{"providers":["fal/model"]}`, `{"providers":["fal ai"]}`} {
		t.Run(payload, func(t *testing.T) {
			worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) }))
			defer worker.Close()
			repo := discoveryTestRepo([]domain.CatalogItem{{ID: "catalog-model", Provider: "openai"}}, DiscoveryConfig{AccountID: "aid", GatewayID: "gid", ReadToken: "tok", WorkerBaseURL: worker.URL, WorkerSecret: "s"}, cf.URL)
			actual := providersBySlug(t, repo)
			if len(actual) != 2 || !actual["openai"] || !actual["workers-ai"] {
				t.Fatalf("providers = %v, want catalog fallback without CF-only fal", actual)
			}
		})
	}
}

func TestListConfiguredProvidersAcceptsEmptyAndFutureCapabilities(t *testing.T) {

	cf := serveCFProviders(t, []string{"custom-future", "fal", "workers-ai"}, http.StatusOK, nil)
	defer cf.Close()

	for _, tc := range []struct {
		name, payload   string
		futureAvailable bool
	}{
		{name: "valid-empty", payload: `{"providers":[]}`},
		{name: "future-adapter", payload: `{"providers":["custom-future"]}`, futureAvailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.payload)) }))
			defer worker.Close()
			repo := discoveryTestRepo([]domain.CatalogItem{{ID: "catalog-model", Provider: "openai"}}, DiscoveryConfig{AccountID: "aid", GatewayID: "gid", ReadToken: "tok", WorkerBaseURL: worker.URL, WorkerSecret: "s"}, cf.URL)
			actual := providersBySlug(t, repo)
			if !actual["workers-ai"] {
				t.Fatalf("providers = %v; workers-ai must stay available", actual)
			}
			if available, found := actual["custom-future"]; !found || available != tc.futureAvailable {
				t.Fatalf("providers = %v, want future available %v", actual, tc.futureAvailable)
			}
			if available, found := actual["fal"]; !found || available {
				t.Fatalf("providers = %v, want fal listed but unadapted", actual)
			}
			if _, found := actual["openai"]; found {
				t.Fatalf("providers = %v, valid capabilities must not trigger catalog fallback", actual)
			}
		})
	}
}

func TestConfiguredProvidersControlPlanePolicyRemainsStrict(t *testing.T) {
	requests := 0
	cf := serveCFProviders(t, []string{"fal"}, http.StatusOK, func(*http.Request) { requests++ })
	defer cf.Close()
	repo := newConfiguredProviders(discoveryCatalog{{ID: "catalog", Provider: "openai"}}, DiscoveryConfig{
		AccountID: "aid", GatewayID: "gid", ReadToken: "tok",
	}, cf.URL, func(context.Context) []string { return nil })
	actual := providersBySlug(t, repo)
	if requests != 0 || len(actual) != 2 || !actual["openai"] || !actual["workers-ai"] {
		t.Fatalf("requests = %d, providers = %v; want blocked CF request and catalog fallback", requests, actual)
	}
}

func TestConfiguredProvidersMalformedControlPlaneFallsBack(t *testing.T) {
	for _, payload := range []string{`{`, `null`, `{}`, `{"success":false,"result":[]}`} {
		t.Run(payload, func(t *testing.T) {
			cf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) }))
			defer cf.Close()
			repo := discoveryTestRepo([]domain.CatalogItem{{ID: "catalog", Provider: "openai"}}, DiscoveryConfig{
				AccountID: "aid", GatewayID: "gid", ReadToken: "tok",
			}, cf.URL)
			actual := providersBySlug(t, repo)
			if len(actual) != 2 || !actual["openai"] || !actual["workers-ai"] {
				t.Fatalf("providers = %v; want catalog fallback", actual)
			}
		})
	}
}

type failingDiscoveryCatalog struct{}

func (failingDiscoveryCatalog) ListCatalog(context.Context) ([]domain.CatalogItem, error) {
	return nil, errors.New("catalog unavailable")
}

func TestConfiguredProvidersCatalogFailureSurfaces(t *testing.T) {
	_, err := NewConfiguredProviders(failingDiscoveryCatalog{}, DiscoveryConfig{}).ListConfiguredProviders(context.Background())
	if err == nil || !strings.Contains(err.Error(), "catalog unavailable") {
		t.Fatalf("error = %v; want catalog failure", err)
	}
}
