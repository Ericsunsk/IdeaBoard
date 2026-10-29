package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"

	aigatewayports "ideaboard/api/internal/aigateway/ports"
)

// Configured provider source: control-plane provider_configs (AI Gateway
// Read token) plus the workers-ai first-party constant. No community
// catalog, no models.dev, no availability probing.

// configuredProvidersTTL bounds how long a control-plane provider list is
// reused: management tokens rarely rotate provider wiring within minutes.
const configuredProvidersTTL = 10 * time.Minute

// maxConfiguredSlugsEntries bounds the token-hash cache: entries are keyed
// by gateway token, so rotation without a bound leaks one entry per token.
const maxConfiguredSlugsEntries = 64

type configuredSlugsEntry struct {
	slugs   []string
	expires time.Time
}

var configuredSlugsCache = struct {
	sync.Mutex
	byToken map[string]configuredSlugsEntry
}{byToken: make(map[string]configuredSlugsEntry)}

// controlPlaneSlugsFetcher is a var for test override of the
// control-plane provider list call.
var controlPlaneSlugsFetcher = tryFetchConfiguredProviderSlugs

// cachedConfiguredSlugs returns the control-plane provider list, reusing a
// short-TTL entry keyed by token hash. An error means the configured set is
// unknown and the caller must fail closed.
func cachedConfiguredSlugs(ctx context.Context, gateway ResolvedGateway) ([]string, error) {
	keyBytes := sha256.Sum256([]byte(gateway.Token))
	key := hex.EncodeToString(keyBytes[:])
	now := time.Now()
	configuredSlugsCache.Lock()
	if entry, ok := configuredSlugsCache.byToken[key]; ok && now.Before(entry.expires) {
		slugs := append([]string(nil), entry.slugs...)
		configuredSlugsCache.Unlock()
		return slugs, nil
	}
	configuredSlugsCache.Unlock()
	slugs, err := controlPlaneSlugsFetcher(ctx, gateway)
	if err != nil {
		return nil, err
	}
	configuredSlugsCache.Lock()
	configuredSlugsCache.byToken[key] = configuredSlugsEntry{
		slugs:   append([]string(nil), slugs...),
		expires: now.Add(configuredProvidersTTL),
	}
	evictConfiguredSlugsLocked(now)
	configuredSlugsCache.Unlock()
	return slugs, nil
}

// evictConfiguredSlugsLocked keeps the cache bounded: expired entries go
// first, then arbitrary entries until back under the cap. Caller holds the
// lock.
func evictConfiguredSlugsLocked(now time.Time) {
	for key, entry := range configuredSlugsCache.byToken {
		if !now.Before(entry.expires) {
			delete(configuredSlugsCache.byToken, key)
		}
	}
	for key := range configuredSlugsCache.byToken {
		if len(configuredSlugsCache.byToken) <= maxConfiguredSlugsEntries {
			break
		}
		delete(configuredSlugsCache.byToken, key)
	}
}

// Provider identity lives in workflow.ResolveProvider (single registry):
// canonical slug, display name and gateway namespace. Control-plane slugs
// are lower-cased here; variant spellings resolve through the registry's
// aliases at lookup time.

// cfProviderConfigsResponse represents the Cloudflare AI Gateway control-plane provider configuration schema.
type cfProviderConfigsResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ProviderSlug string `json:"provider_slug"`
		ID           string `json:"id"`
	} `json:"result"`
}

var gatewayURLRegex = regexp.MustCompile(`gateway\.ai\.cloudflare\.com/v1/([^/]+)/([^/]+)`)

func extractGatewayIdentity(baseURL string) (accountID, gatewayID string) {
	matches := gatewayURLRegex.FindStringSubmatch(baseURL)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return "", ""
}

// resolveControlPlaneIdentity prefers the stored account/gateway IDs;
// URL parsing is only a fallback. Pure for testability: no network.
func resolveControlPlaneIdentity(gateway ResolvedGateway) (accountID, gatewayID string, err error) {
	accountID = strings.TrimSpace(gateway.AccountID)
	gatewayID = strings.TrimSpace(gateway.GatewayID)
	if accountID == "" || gatewayID == "" {
		accountID, gatewayID = extractGatewayIdentity(gateway.BaseURL)
	}
	if accountID == "" || gatewayID == "" || strings.TrimSpace(gateway.Token) == "" {
		return "", "", errors.New(errorrender.MsgResolveControlPlaneIdentity)
	}
	return accountID, gatewayID, nil
}

// configuredProvidersTarget names provider-config egress in outbound logs/metrics.
const configuredProvidersTarget = "ai-gateway-control-plane"

// configuredProvidersOutbound is the shared standard outbound client for
// control-plane provider-config reads.
var configuredProvidersOutbound interface {
	Do(context.Context, *http.Request, safehttp.RequestOptions) (*http.Response, error)
} = safehttp.NewClient(safehttp.Config{})

// fetchConfiguredProvidersBody performs one bounded GET against the URL's own
// host through the standard outbound client (SSRF allowlist + bounded
// retries). Status-to-error mapping stays with the caller: an exhausted
// retryable status (408/429/5xx) returns its status and body preview with a
// nil error. Only transport/timeout failures surface as errors.
func fetchConfiguredProvidersBody(ctx context.Context, rawURL string, headers map[string]string, timeout time.Duration, maxBytes int64, target string, maxAttempts int) (body []byte, statusCode int, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, 0, errors.New(errorrender.MsgFetchHTTPBody)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	for key, value := range headers {
		httpReq.Header.Set(key, value)
	}
	resp, err := configuredProvidersOutbound.Do(ctx, httpReq, safehttp.RequestOptions{
		Timeout:     timeout,
		MaxAttempts: maxAttempts,
		// Never authorize the URL's host implicitly. Control-plane egress
		// must be present in the operator-managed outbound policy.
		AllowHosts: safehttp.AllowHosts(ctx),
		Target:     target,
	})
	if err != nil {
		var statusErr *safehttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return []byte(statusErr.Body), statusErr.StatusCode, nil
		}
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

// tryFetchConfiguredProviderSlugs queries Cloudflare AI Gateway Control Plane API
// to discover configured providers if the token has management permissions.
func tryFetchConfiguredProviderSlugs(ctx context.Context, gateway ResolvedGateway) ([]string, error) {
	accountID, gatewayID, err := resolveControlPlaneIdentity(gateway)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai-gateway/gateways/%s/provider_configs", accountID, gatewayID)
	body, statusCode, err := fetchConfiguredProvidersBody(ctx, apiURL, map[string]string{
		aigatewayports.HeaderAuthorization: "Bearer " + strings.TrimSpace(gateway.Token),
		"Content-Type":                     "application/json",
	}, 4*time.Second, 1<<20, configuredProvidersTarget, 0)
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("control plane returned %d", statusCode)
	}

	var cfResp cfProviderConfigsResponse
	if err := json.Unmarshal(body, &cfResp); err != nil || !cfResp.Success {
		return nil, fmt.Errorf("invalid control plane response")
	}

	slugs := make([]string, 0, len(cfResp.Result))
	for _, item := range cfResp.Result {
		// 归一化只做大小写/空白；变体拼写由 registry 别名在查询时解析。
		if s := strings.ToLower(strings.TrimSpace(item.ProviderSlug)); s != "" {
			slugs = append(slugs, s)
		}
	}
	return slugs, nil
}

// ListConfiguredProviders resolves the admin-visible provider offering as
// the control-plane configured set plus the workers-ai first-party
// constant. Membership is driven by the control plane; the registry only
// enriches. Unknown slugs resolve to synthetic defaults instead of
// vanishing silently. Workers AI carries no key so it never appears in the
// control-plane list; it is always admitted as a first-party constant with
// no availability probing.
func (r *ConfigRepository) ListConfiguredProviders(ctx context.Context) ([]workflow.ProviderEntry, error) {
	resolved, err := r.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	configuredSlugs, err := cachedConfiguredSlugs(ctx, resolved)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgListConfiguredProviders, err)
	}
	seen := make(map[string]struct{}, len(configuredSlugs)+1)
	var providers []workflow.ProviderEntry
	for _, s := range configuredSlugs {
		spec, _ := workflow.ResolveProvider(s)
		if _, dup := seen[spec.Slug]; dup {
			continue
		}
		seen[spec.Slug] = struct{}{}
		providers = append(providers, spec)
	}
	if _, dup := seen["workers-ai"]; !dup {
		if spec, known := workflow.ResolveProvider("workers-ai"); known {
			providers = append(providers, spec)
		}
	}
	return providers, nil
}
