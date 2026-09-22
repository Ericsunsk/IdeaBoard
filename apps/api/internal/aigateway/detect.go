package aigateway

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
	"sort"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

// DetectedModel represents a model discovered from an AI Gateway.
// Cost fields mirror LiteLLM community units (USD per token); the admin UI
// scales them for display. They are reference data only, never billing input.
type DetectedModel struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Provider                string  `json:"provider"`
	Mode                    string  `json:"mode,omitempty"`
	Description             string  `json:"description,omitempty"`
	ContextOrResolution     string  `json:"context_or_resolution,omitempty"`
	InputCostPerToken       float64 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken      float64 `json:"output_cost_per_token,omitempty"`
	CacheReadCostPerToken   float64 `json:"cache_read_input_token_cost,omitempty"`
	CacheCreateCostPerToken float64 `json:"cache_creation_input_token_cost,omitempty"`
	SupportsPromptCaching   bool    `json:"supports_prompt_caching,omitempty"`
}

// ModelDetection is the outcome of community-catalog detection: the
// configured providers' models plus the configured count the admin UI
// needs to explain empty results. Detection is fail-closed: control-plane
// and catalog failures surface as errors, never silent empty results.
type ModelDetection struct {
	Models              []DetectedModel `json:"models"`
	ConfiguredProviders int             `json:"configured_providers"`
}

// configuredProvidersTTL bounds how long a control-plane provider list is
// reused: management tokens rarely rotate provider wiring within minutes.
const configuredProvidersTTL = 10 * time.Minute

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
// unknown and detection must fail closed.
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
	configuredSlugsCache.Unlock()
	return slugs, nil
}

// Provider identity lives in workflow.ResolveProvider (single registry):
// canonical slug, display name, community slice and gateway namespace.
// Control-plane slugs are lower-cased here; variant spellings resolve
// through the registry's aliases at lookup time.

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

// resolveControlPlaneIdentity prefers the stored account/gateway IDs
// (custom egress BaseURLs carry no parseable identity); URL parsing is
// only a fallback. Pure for testability: no network.
func resolveControlPlaneIdentity(gateway ResolvedGateway) (accountID, gatewayID string, err error) {
	accountID = strings.TrimSpace(gateway.AccountID)
	gatewayID = strings.TrimSpace(gateway.GatewayID)
	if accountID == "" || gatewayID == "" {
		accountID, gatewayID = extractGatewayIdentity(gateway.BaseURL)
	}
	if accountID == "" || gatewayID == "" || strings.TrimSpace(gateway.Token) == "" {
		return "", "", errors.New("网关缺少 Account/Gateway 身份，请先在网关设置中补全后再探测")
	}
	return accountID, gatewayID, nil
}

// tryFetchConfiguredProviderSlugs queries Cloudflare AI Gateway Control Plane API
// to discover configured providers if the token has management permissions.
func tryFetchConfiguredProviderSlugs(ctx context.Context, gateway ResolvedGateway) ([]string, error) {
	accountID, gatewayID, err := resolveControlPlaneIdentity(gateway)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai-gateway/gateways/%s/provider_configs", accountID, gatewayID)
	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(gateway.Token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := safehttp.DoRequestWithAllowHosts(reqCtx, httpReq, 4*time.Second, "api.cloudflare.com")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("control plane returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
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

// DetectModels resolves the admin-visible model offering as the
// intersection of control-plane configured providers and the LiteLLM
// community catalog. Community keys are stored verbatim (no ID rewriting);
// gateway-executable routing is derived in the workflow routing layer.
// Fail-closed: control-plane and catalog failures surface as errors.
func (r *ConfigRepository) DetectModels(ctx context.Context, providerFilter string) (ModelDetection, error) {
	resolved, err := r.Resolve(ctx)
	if err != nil {
		return ModelDetection{}, err
	}

	filter := strings.ToLower(strings.TrimSpace(providerFilter))
	if filter != "" && filter != "all" && filter != "全部" {
		spec, known := workflow.ResolveProvider(filter)
		if !known {
			return ModelDetection{}, fmt.Errorf("未知 Provider %q", strings.TrimSpace(providerFilter))
		}
		return detectWithCatalog(ctx, []workflow.ProviderEntry{spec}, 1)
	}

	configuredSlugs, err := cachedConfiguredSlugs(ctx, resolved)
	if err != nil {
		return ModelDetection{}, fmt.Errorf("读取网关已配置 Provider 失败: %w", err)
	}
	// Membership is driven by the control plane; the registry only
	// enriches. Unknown slugs resolve to synthetic defaults (community
	// guess + custom- namespace) instead of vanishing silently.
	var targets []workflow.ProviderEntry
	for _, s := range configuredSlugs {
		spec, _ := workflow.ResolveProvider(s)
		targets = append(targets, spec)
	}
	// Workers AI 是无 key 第一方：从不出名单在 control-plane 配置里，
	// 交集会系统性漏掉它；改走可用性判定，存活即纳入。
	if !hasTarget(targets, "workers-ai") {
		if available, err := workersAIAvailable(ctx, resolved); err == nil && available {
			if spec, known := workflow.ResolveProvider("workers-ai"); known {
				targets = append(targets, spec)
			}
		}
	}
	// Empty list means nothing is configured: stay silent, never fetch.
	if len(targets) == 0 {
		return ModelDetection{ConfiguredProviders: 0}, nil
	}
	return detectWithCatalog(ctx, targets, len(targets))
}

func hasTarget(targets []workflow.ProviderEntry, slug string) bool {
	for _, t := range targets {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

// workersAIAvailable checks Workers AI upstream reachability through the
// gateway: Workers AI needs no stored key, so control-plane config can
// never attest it. Classification rides the shared ports signal table;
// transport failures surface as errors so callers can distinguish
// "unreachable" from "unavailable".
func workersAIAvailable(ctx context.Context, gateway ResolvedGateway) (bool, error) {
	probeURL := strings.TrimRight(strings.TrimSpace(gateway.BaseURL), "/") + "/workers-ai/models/search"
	parsed, err := url.Parse(probeURL)
	if err != nil || parsed.Hostname() == "" {
		return false, fmt.Errorf("网关地址无效")
	}

	reqCtx, cancel := context.WithTimeout(ctx, workflow.GatewayProbeTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, probeURL, nil)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(gateway.Token) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(gateway.Token))
		httpReq.Header.Set("cf-aig-authorization", "Bearer "+strings.TrimSpace(gateway.Token))
	}

	resp, err := safehttp.DoRequestWithAllowHosts(reqCtx, httpReq, workflow.GatewayProbeTimeout, parsed.Hostname())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return workflow.ClassifyGatewayResponse(resp.StatusCode, string(body)) == workflow.ReachabilityReachable, nil
}

// detectWithCatalog intersects one community fetch with the target
// providers' slices.
func detectWithCatalog(ctx context.Context, targets []workflow.ProviderEntry, configured int) (ModelDetection, error) {
	entries, err := fetchCommunityCatalog(ctx)
	if err != nil {
		return ModelDetection{}, err
	}
	return ModelDetection{
		Models:              filterCommunityModels(entries, targets),
		ConfiguredProviders: configured,
	}, nil
}

// communityCatalogURL is a var for test override: the LiteLLM community
// catalog is the single model metadata source (public JSON, no auth).
var communityCatalogURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// communityCatalogTTL bounds refetch of the ~3MB community catalog.
const communityCatalogTTL = 24 * time.Hour

type communityPriceEntry struct {
	LitellmProvider         string  `json:"litellm_provider"`
	Mode                    string  `json:"mode"`
	MaxInputTokens          int     `json:"max_input_tokens"`
	DeprecationDate         string  `json:"deprecation_date"`
	InputCostPerToken       float64 `json:"input_cost_per_token"`
	OutputCostPerToken      float64 `json:"output_cost_per_token"`
	CacheReadCostPerToken   float64 `json:"cache_read_input_token_cost"`
	CacheCreateCostPerToken float64 `json:"cache_creation_input_token_cost"`
	SupportsPromptCaching   bool    `json:"supports_prompt_caching"`
}

// communityRecord is one trimmed catalog entry: key verbatim, provider
// lower-cased for matching, deprecation resolved at fetch time.
type communityRecord struct {
	Key                     string
	Provider                string
	Mode                    string
	MaxInputTokens          int
	InputCostPerToken       float64
	OutputCostPerToken      float64
	CacheReadCostPerToken   float64
	CacheCreateCostPerToken float64
	SupportsPromptCaching   bool
}

var communityCache struct {
	sync.RWMutex
	entries   []communityRecord
	fetchedAt time.Time
}

// fetchCommunityCatalog returns the trimmed community catalog, reusing a
// 24h entry. Every failure is explicit (fail-closed): network, non-200,
// empty body, shape drift, or zero usable entries all surface as errors.
func fetchCommunityCatalog(ctx context.Context) ([]communityRecord, error) {
	communityCache.RLock()
	if len(communityCache.entries) > 0 && time.Since(communityCache.fetchedAt) < communityCatalogTTL {
		cached := make([]communityRecord, len(communityCache.entries))
		copy(cached, communityCache.entries)
		communityCache.RUnlock()
		return cached, nil
	}
	communityCache.RUnlock()

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	catalogURL, err := url.Parse(communityCatalogURL)
	if err != nil || catalogURL.Hostname() == "" {
		return nil, fmt.Errorf("社区模型目录地址无效")
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, communityCatalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造社区目录请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; IdeaBoard/1.0)")

	resp, err := safehttp.DoRequestWithAllowHosts(reqCtx, req, 15*time.Second, catalogURL.Hostname())
	if err != nil {
		return nil, fmt.Errorf("获取社区模型目录失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("社区模型目录返回 %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil || len(bodyBytes) == 0 {
		return nil, fmt.Errorf("社区模型目录内容为空")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, fmt.Errorf("社区模型目录格式变化，解析失败")
	}

	now := time.Now()
	entries := make([]communityRecord, 0, 1024)
	for key, rawEntry := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		var entry communityPriceEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			continue
		}
		if entry.DeprecationDate != "" {
			if deprecatedOn, err := time.Parse("2006-01-02", strings.TrimSpace(entry.DeprecationDate)); err == nil && !deprecatedOn.After(now) {
				continue
			}
		}
		entries = append(entries, communityRecord{
			Key:                     key,
			Provider:                strings.ToLower(strings.TrimSpace(entry.LitellmProvider)),
			Mode:                    strings.ToLower(strings.TrimSpace(entry.Mode)),
			MaxInputTokens:          entry.MaxInputTokens,
			InputCostPerToken:       entry.InputCostPerToken,
			OutputCostPerToken:      entry.OutputCostPerToken,
			CacheReadCostPerToken:   entry.CacheReadCostPerToken,
			CacheCreateCostPerToken: entry.CacheCreateCostPerToken,
			SupportsPromptCaching:   entry.SupportsPromptCaching,
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("社区模型目录无有效条目")
	}

	communityCache.Lock()
	communityCache.entries = entries
	communityCache.fetchedAt = time.Now()
	communityCache.Unlock()
	return entries, nil
}

// communityRecordMatches reports whether one catalog entry belongs to one
// provider slice: key prefix ("<p>/...") or provider field attribution.
func communityRecordMatches(e communityRecord, target workflow.ProviderEntry) bool {
	for _, p := range target.CommunityPrefixes {
		if strings.HasPrefix(e.Key, p+"/") {
			return true
		}
	}
	for _, p := range target.CommunityProviders {
		if e.Provider == p {
			return true
		}
	}
	return false
}

// filterCommunityModels intersects the catalog with the target providers.
// IDs and names stay verbatim community keys (Q4): the last key segment is
// the display name, bare keys keep the full key. First target match wins;
// results are deduplicated and sorted by ID.
func filterCommunityModels(entries []communityRecord, targets []workflow.ProviderEntry) []DetectedModel {
	models := make([]DetectedModel, 0, 64)
	seen := make(map[string]bool)
	for _, target := range targets {
		for _, e := range entries {
			if seen[e.Key] || !communityRecordMatches(e, target) {
				continue
			}
			seen[e.Key] = true
			name := e.Key
			if i := strings.LastIndex(e.Key, "/"); i >= 0 {
				name = e.Key[i+1:]
			}
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			models = append(models, DetectedModel{
				ID:                      e.Key,
				Name:                    name,
				Provider:                target.DisplayName,
				Mode:                    e.Mode,
				ContextOrResolution:     formatContextLength(e.MaxInputTokens),
				InputCostPerToken:       e.InputCostPerToken,
				OutputCostPerToken:      e.OutputCostPerToken,
				CacheReadCostPerToken:   e.CacheReadCostPerToken,
				CacheCreateCostPerToken: e.CacheCreateCostPerToken,
				SupportsPromptCaching:   e.SupportsPromptCaching,
			})
		}
	}
	sortModelsByID(models)
	return models
}

func sortModelsByID(models []DetectedModel) {
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
}
func formatContextLength(tokens int) string {
	if tokens <= 0 {
		return ""
	}
	if tokens >= 1_000_000 {
		if tokens%1_000_000 == 0 {
			return fmt.Sprintf("%dM", tokens/1_000_000)
		}
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1000 {
		k := (tokens + 500) / 1000
		if tokens%1024 == 0 {
			k = tokens / 1024
		}
		return fmt.Sprintf("%dK", k)
	}
	return fmt.Sprintf("%d", tokens)
}
