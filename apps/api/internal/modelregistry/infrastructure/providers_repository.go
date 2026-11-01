package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/modelregistry/ports"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

// Configured provider source: the Cloudflare control-plane provider_configs
// list (CF_ACCOUNT_ID/CF_GATEWAY_ID/CF_AIG_TOKEN, shared Gateway token)
// annotated with the
// ai-proxy Worker /v1/providers capability list, plus the workers-ai
// first-party constant (always present and available). Any remote failure
// (control plane or Worker unreachable) degrades in place to the catalog
// fallback below: the distinct admin-managed catalog providers plus
// workers-ai, warn-logged, never failing the listing. Reachability beyond
// that is a Preview dry-run concern, not a listing concern. The task
// execution chain never reads this function (it consumes the pinned
// RoutingSnapshot), so discovery degradation cannot block execution.

// DiscoveryConfig carries the provider-discovery inputs: the Cloudflare
// control-plane identity plus the shared Gateway token, and the Worker egress
// for the /v1/providers capability read. The zero value keeps catalog fallback.
type DiscoveryConfig struct {
	// AccountID is the Cloudflare account owning the gateway (CF_ACCOUNT_ID).
	AccountID string
	// GatewayID is the AI Gateway identifier (CF_GATEWAY_ID).
	GatewayID string
	// ReadToken is the shared CF_AIG_TOKEN Gateway bearer credential, reused
	// for control-plane discovery (intentional: VPS already holds
	// WORKER_INTERNAL_SECRET).
	ReadToken string
	// WorkerBaseURL is the ai-proxy Worker service root (WORKER_BASE_URL).
	WorkerBaseURL string
	// WorkerSecret is the WORKER_INTERNAL_SECRET bearer credential.
	WorkerSecret string
}

// ConfiguredProviders owns remote reads, capability merging and catalog fallback.
// It is fully initialized at construction and never changes accepted Task routing.
type ConfiguredProviders struct {
	catalog             ports.CatalogReader
	discovery           DiscoveryConfig
	controlPlaneBaseURL string
	allowHosts          func(context.Context) []string
	outbound            *safehttp.Client
}

func NewConfiguredProviders(catalog ports.CatalogReader, cfg DiscoveryConfig) *ConfiguredProviders {
	return newConfiguredProviders(catalog, cfg, "https://api.cloudflare.com", safehttp.AllowHosts)
}

func newConfiguredProviders(catalog ports.CatalogReader, cfg DiscoveryConfig, controlPlaneURL string, allowHosts func(context.Context) []string) *ConfiguredProviders {
	return &ConfiguredProviders{catalog: catalog, discovery: cfg, controlPlaneBaseURL: controlPlaneURL,
		allowHosts: allowHosts, outbound: safehttp.NewClient(safehttp.Config{})}
}

// discoveryTarget names control-plane provider discovery in logs/metrics.
const discoveryTarget = "ai-gateway-control-plane"

// workerProvidersTarget names the Worker capability read in logs/metrics.
const workerProvidersTarget = "edge-worker-providers"

// discoveryTimeout bounds one provider-discovery read (all attempts).
const discoveryTimeout = 4 * time.Second

// discoveryMaxBytes caps one provider-discovery response body.
const discoveryMaxBytes = 1 << 20

// ListConfiguredProviders resolves the admin-visible provider offering.
//
// When control-plane discovery and the Worker capability read both succeed,
// membership is driven by the control plane: every CF slug is listed, marked
// available only when the Worker also advertises it (CF ∩ Worker =
// available; CF without Worker = shown but unadapted); Worker-only slugs
// are never shown. workers-ai is always admitted as a first-party constant.
//
// Either remote failing (or discovery unconfigured) degrades in place to
// the catalog fallback: the distinct catalog provider set plus workers-ai,
// all available. Only a catalog load failure surfaces as an error.
func (r *ConfiguredProviders) ListConfiguredProviders(ctx context.Context) ([]workflow.ProviderEntry, error) {
	if r == nil {
		return nil, fmt.Errorf(errorrender.MsgListConfiguredProviders, fmt.Errorf("configured-provider discovery is unavailable"))
	}
	cfSlugs, err := r.fetchCFProviderSlugs(ctx, r.discovery)
	reason := "control-plane-unavailable"
	if err == nil {
		var workerSlugs []string
		workerSlugs, err = r.fetchWorkerProviders(ctx, r.discovery)
		if err == nil {
			return mergeDiscoveredProviders(cfSlugs, workerSlugs), nil
		}
		reason = "worker-unavailable"
	}
	slog.WarnContext(ctx, "modelregistry provider discovery degraded to catalog fallback",
		"reason", reason, "err", err)
	return r.listCatalogFallback(ctx)
}

// ensureWorkersAI appends the workers-ai first-party constant unless the
// caller already listed it. Shared by the discovered merge and the catalog
// fallback so the constant can never drop out of either path.
func ensureWorkersAI(providers []workflow.ProviderEntry, seen map[string]struct{}) []workflow.ProviderEntry {
	if _, dup := seen["workers-ai"]; !dup {
		if entry, known := workflow.ResolveProvider("workers-ai"); known {
			providers = append(providers, entry)
		}
	}
	return providers
}

// mergeDiscoveredProviders annotates CF slugs with Worker capability:
// CF ∩ Worker = available; CF without Worker = shown but unadapted;
// Worker-only slugs are dropped; workers-ai is always present.
func mergeDiscoveredProviders(cfSlugs, workerSlugs []string) []workflow.ProviderEntry {
	workerSet := make(map[string]struct{}, len(workerSlugs))
	for _, s := range workerSlugs {
		if clean := strings.ToLower(strings.TrimSpace(s)); clean != "" {
			workerSet[clean] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(cfSlugs)+1)
	var providers []workflow.ProviderEntry
	for _, s := range cfSlugs {
		entry, known := workflow.ResolveProvider(s)
		if !known || entry.Slug == "" {
			continue
		}
		if _, dup := seen[entry.Slug]; dup {
			continue
		}
		seen[entry.Slug] = struct{}{}
		if _, ok := workerSet[entry.Slug]; !ok && entry.Slug != "workers-ai" {
			entry.Available = false
		}
		providers = append(providers, entry)
	}
	return ensureWorkersAI(providers, seen)
}

// listCatalogFallback resolves the pre-discovery offering: the distinct
// catalog provider set plus the workers-ai first-party constant. Membership
// is driven by the catalog; the registry only enriches. Unknown slugs
// resolve to synthetic defaults instead of vanishing silently.
func (r *ConfiguredProviders) listCatalogFallback(ctx context.Context) ([]workflow.ProviderEntry, error) {
	items, err := r.catalog.ListCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgListConfiguredProviders, err)
	}
	seen := make(map[string]struct{}, len(items)+1)
	var providers []workflow.ProviderEntry
	for _, item := range items {
		spec, _ := workflow.ResolveProvider(item.Provider)
		if spec.Slug == "" {
			continue
		}
		if _, dup := seen[spec.Slug]; dup {
			continue
		}
		seen[spec.Slug] = struct{}{}
		providers = append(providers, spec)
	}
	return ensureWorkersAI(providers, seen), nil
}

// cfProviderConfigsResponse is the Cloudflare AI Gateway control-plane
// provider configuration schema (GET
// /client/v4/accounts/{aid}/ai-gateway/gateways/{gid}/provider_configs):
// gateway-scoped list; there is no account-level provider list (only a
// gateway list). Responses carry masked key previews, never key material.
type cfProviderConfigsResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ProviderSlug string `json:"provider_slug"`
		ID           string `json:"id"`
	} `json:"result"`
}

// workerProvidersResponse is the ai-proxy GET /v1/providers schema:
// WORKER_INTERNAL_SECRET bearer auth (same as generate); success carries
// {providers: string[]} derived from the Worker provider registry;
// 401 bodies carry no task_id and surface here as non-200 failures.
type workerProvidersResponse struct {
	Providers []string `json:"providers"`
}

// fetchCFProviderSlugs reads the gateway-scoped provider_configs list with
// the read-only token through the strict allowlist path (AllowHosts, never
// the configured endpoint: api.cloudflare.com is public, not operator
// configuration). Any transport failure, non-2xx status or malformed
// payload is an error; the caller degrades to the catalog fallback.
func (r *ConfiguredProviders) fetchCFProviderSlugs(ctx context.Context, cfg DiscoveryConfig) ([]string, error) {
	accountID := strings.TrimSpace(cfg.AccountID)
	gatewayID := strings.TrimSpace(cfg.GatewayID)
	token := strings.TrimSpace(cfg.ReadToken)
	if accountID == "" || gatewayID == "" || token == "" {
		return nil, fmt.Errorf("control-plane discovery is not configured")
	}
	rawURL := strings.TrimRight(strings.TrimSpace(r.controlPlaneBaseURL), "/") +
		"/client/v4/accounts/" + accountID + "/ai-gateway/gateways/" + gatewayID +
		"/provider_configs?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.outbound.Do(ctx, req, safehttp.RequestOptions{
		Timeout:     discoveryTimeout,
		MaxAttempts: 1,
		// Never authorize the control-plane host implicitly: its egress
		// must be present in the operator-managed outbound policy.
		AllowHosts: r.allowHosts(ctx),
		Target:     discoveryTarget,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, discoveryMaxBytes))
		return nil, fmt.Errorf("control plane returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, discoveryMaxBytes))
	if err != nil {
		return nil, err
	}
	var decoded cfProviderConfigsResponse
	if err := json.Unmarshal(body, &decoded); err != nil || !decoded.Success {
		return nil, fmt.Errorf("invalid control plane response")
	}
	slugs := make([]string, 0, len(decoded.Result))
	for _, item := range decoded.Result {
		if s := strings.ToLower(strings.TrimSpace(item.ProviderSlug)); s != "" {
			slugs = append(slugs, s)
		}
	}
	return slugs, nil
}

// fetchWorkerProviders reads the ai-proxy GET /v1/providers capability list
// through the configured-endpoint path (the Worker base URL is operator
// configuration, loopback included, no allowlist). Any transport failure,
// non-2xx status (including 401, whose body carries no task_id) or
// malformed payload is an error; the caller degrades to the catalog
// fallback.
func (r *ConfiguredProviders) fetchWorkerProviders(ctx context.Context, cfg DiscoveryConfig) ([]string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.WorkerBaseURL), "/")
	secret := strings.TrimSpace(cfg.WorkerSecret)
	if baseURL == "" || secret == "" {
		return nil, fmt.Errorf("worker discovery is not configured")
	}
	endpoint, err := safehttp.ParseConfiguredEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.BaseURL()+"/v1/providers", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := r.outbound.Do(ctx, req, safehttp.RequestOptions{
		Timeout:     discoveryTimeout,
		MaxAttempts: 1,
		Configured:  endpoint,
		Target:      workerProvidersTarget,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, discoveryMaxBytes))
		return nil, fmt.Errorf("worker providers returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, discoveryMaxBytes))
	if err != nil {
		return nil, err
	}
	var decoded workerProvidersResponse
	if err := json.Unmarshal(body, &decoded); err != nil || decoded.Providers == nil {
		return nil, fmt.Errorf("invalid worker providers response")
	}
	for i, value := range decoded.Providers {
		slug := strings.ToLower(strings.TrimSpace(value))
		if slug == "" {
			return nil, fmt.Errorf("invalid worker provider slug")
		}
		// Validate registry slug syntax, not membership: a future adapter may
		// introduce a provider this API version does not know yet.
		for _, char := range slug {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
				return nil, fmt.Errorf("invalid worker provider slug")
			}
		}
		decoded.Providers[i] = slug
	}
	return decoded.Providers, nil
}
