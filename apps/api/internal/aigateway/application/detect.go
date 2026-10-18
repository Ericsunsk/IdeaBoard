package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
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

	aigatewayports "ideaboard/api/internal/aigateway/ports"
)

// DetectedModel represents a model discovered from an AI Gateway.
// Detection surfaces routing and capability facts only: no prices. Billing
// charges solely on the billing SKU UnitCredits; reference prices (if
// needed) come from the read-only models.dev seam.
// New optional fields stay appended: JSON is name-addressed, but positional
// order is a compatibility contract.
type DetectedModel struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Provider              string `json:"provider"`
	Mode                  string `json:"mode,omitempty"`
	Description           string `json:"description,omitempty"`
	ContextOrResolution   string `json:"context_or_resolution,omitempty"`
	SupportsPromptCaching bool   `json:"supports_prompt_caching,omitempty"`
	// DescriptionEnriched marks auto-filled detection descriptions: the
	// admin refresh flow may overwrite them, operator-written catalog text
	// (enriched=false) is never touched.
	DescriptionEnriched bool `json:"description_enriched,omitempty"`
	// ContextLimit is the raw models.dev limit.context in tokens, present
	// only when models.dev attests the model. It is display metadata only,
	// never billing input.
	ContextLimit int `json:"context_limit,omitempty"`
	// Capabilities names the models.dev capability flags attested true for
	// the model, upstream names verbatim (reasoning, tool_call,
	// temperature, attachment, structured_output, open_weights). False and
	// unknown stay absent: only present capabilities are listed.
	Capabilities []string `json:"capabilities,omitempty"`
	// Modalities mirrors the models.dev modalities object verbatim. Nil
	// means the upstream entry carried no modalities.
	Modalities *ModelReferenceModalities `json:"modalities,omitempty"`
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
		return "", "", errors.New(errorrender.MsgResolveControlPlaneIdentity)
	}
	return accountID, gatewayID, nil
}

// detectTarget names detection egress in outbound logs/metrics.
const (
	detectControlPlaneTarget = "ai-gateway-control-plane"
	detectWorkersAITarget    = "ai-gateway-workers-ai"
	detectCatalogTarget      = "community-catalog"
)

// fetchHTTPBody performs one bounded GET against the URL's own host through
// the standard outbound client (SSRF allowlist + bounded retries, same as
// the gateway migration). Status-to-error mapping stays with callers so each
// domain keeps its fail-closed wording: an exhausted retryable status (408/
// 429/5xx) returns its status and body preview with a nil error, exactly
// like the single-shot response it replaces. Only transport/timeout
// failures surface as errors.
func fetchHTTPBody(ctx context.Context, rawURL string, headers map[string]string, timeout time.Duration, maxBytes int64, target string, maxAttempts int) (body []byte, statusCode int, err error) {
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
	resp, err := detectOutbound.Do(ctx, httpReq, safehttp.RequestOptions{
		Timeout:     timeout,
		MaxAttempts: maxAttempts,
		// Never authorize the URL's host implicitly. Control-plane,
		// community-catalog, and custom-gateway egress must all be present
		// in the operator-managed outbound policy.
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
	body, statusCode, err := fetchHTTPBody(ctx, apiURL, map[string]string{
		aigatewayports.HeaderAuthorization: "Bearer " + strings.TrimSpace(gateway.Token),
		"Content-Type":                     "application/json",
	}, 4*time.Second, 1<<20, detectControlPlaneTarget, 0)
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

// DetectModels resolves the admin-visible model offering as the union of
// the LiteLLM community catalog and the models.dev catalog, both
// intersected with the control-plane configured providers. Community keys
// are stored verbatim (no ID rewriting); gateway-executable routing is
// derived in the workflow routing layer.
// Fail-closed on the primary source: control-plane and LiteLLM catalog
// failures surface as errors. models.dev is best-effort enrichment: its
// failure degrades to pure LiteLLM; only a LiteLLM failure (both sources
// down included) stays fail-closed.
func (r *ConfigRepository) DetectModels(ctx context.Context, providerFilter string) (ModelDetection, error) {
	resolved, err := r.Resolve(ctx)
	if err != nil {
		return ModelDetection{}, err
	}

	filter := strings.ToLower(strings.TrimSpace(providerFilter))
	configuredSlugs, err := cachedConfiguredSlugs(ctx, resolved)
	if err != nil {
		return ModelDetection{}, fmt.Errorf(errorrender.MsgDetectModels, err)
	}
	if filter != "" && filter != "all" && filter != errorrender.MsgDetectModels2 {
		spec, known := workflow.ResolveProvider(filter)
		if !known {
			return ModelDetection{}, fmt.Errorf(errorrender.MsgDetectModels3, strings.TrimSpace(providerFilter))
		}
		configured := false
		for _, configuredSlug := range configuredSlugs {
			candidate, _ := workflow.ResolveProvider(configuredSlug)
			if candidate.Slug == spec.Slug {
				configured = true
				break
			}
		}
		if !configured && spec.Slug == "workers-ai" {
			configured, _ = workersAIAvailable(ctx, resolved)
		}
		if !configured {
			return ModelDetection{ConfiguredProviders: 0}, nil
		}
		return detectWithCatalog(ctx, []workflow.ProviderEntry{spec}, 1)
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
// never attest it. The probe never fails: transport problems and truncated
// bodies read as unavailable, and only an explicit reachable classification
// adds the target. Classification rides the shared ports signal table.
func workersAIAvailable(ctx context.Context, gateway ResolvedGateway) (bool, error) {
	probeURL := strings.TrimRight(strings.TrimSpace(gateway.BaseURL), "/") + "/workers-ai/models/search"
	parsed, err := url.Parse(probeURL)
	if err != nil || parsed.Hostname() == "" {
		return false, errors.New(errorrender.MsgWorkersAIAvailable)
	}

	headers := map[string]string{}
	if strings.TrimSpace(gateway.Token) != "" {
		headers[aigatewayports.HeaderAuthorization] = "Bearer " + strings.TrimSpace(gateway.Token)
		headers[aigatewayports.GatewayHeaderAuthorization] = "Bearer " + strings.TrimSpace(gateway.Token)
	}

	// Legacy semantics: a truncated probe body never fails the probe; only
	// the status/body classification decides reachability. The probe stays
	// single-shot end to end (no client-side retry): whatever answers IS
	// the probed target.
	body, statusCode, _ := fetchHTTPBody(ctx, probeURL, headers, workflow.GatewayProbeTimeout, 1<<20, detectWorkersAITarget, 1)
	return workflow.ClassifyGatewayResponse(statusCode, string(body)) == workflow.ReachabilityReachable, nil
}

// detectWithCatalog unions the LiteLLM community fetch with the models.dev
// facts over the same target attribution. Either source matching a target
// admits the model (models.dev-only entries are included, LiteLLM-only
// entries are kept); same keys deduplicate with models.dev winning metadata
// conflicts. models.dev failure degrades to pure LiteLLM (facts=nil);
// LiteLLM failure stays fail-closed, which also covers both-sources-down.
func detectWithCatalog(ctx context.Context, targets []workflow.ProviderEntry, configured int) (ModelDetection, error) {
	entries, err := fetchCommunityCatalog(ctx)
	if err != nil {
		return ModelDetection{}, err
	}
	facts, err := loadModelsReferenceFacts(ctx, false)
	if err != nil {
		facts = nil
	}
	return ModelDetection{
		Models:              filterCommunityModels(entries, facts, targets),
		ConfiguredProviders: configured,
	}, nil
}

// communityCatalogURL is a var for test override: the LiteLLM community
// catalog is the primary model metadata source (public JSON, no auth);
// models.dev joins it as best-effort enrichment in detectWithCatalog.
var communityCatalogURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// communityCatalogTTL bounds refetch of the ~3MB community catalog.
const communityCatalogTTL = 24 * time.Hour

type communityPriceEntry struct {
	LitellmProvider       string `json:"litellm_provider"`
	Mode                  string `json:"mode"`
	MaxInputTokens        int    `json:"max_input_tokens"`
	DeprecationDate       string `json:"deprecation_date"`
	SupportsPromptCaching bool   `json:"supports_prompt_caching"`
}

// communityRecord is one trimmed catalog entry: key verbatim, provider
// lower-cased for matching, deprecation resolved at fetch time.
type communityRecord struct {
	Key                   string
	Provider              string
	Mode                  string
	MaxInputTokens        int
	SupportsPromptCaching bool
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

	bodyBytes, statusCode, err := fetchHTTPBody(ctx, communityCatalogURL, map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; IdeaBoard/1.0)",
	}, 15*time.Second, 8<<20, detectCatalogTarget, 0)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgFetchCommunityCatalog, err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf(errorrender.MsgFetchCommunityCatalog2, statusCode)
	}

	if len(bodyBytes) == 0 {
		return nil, errors.New(errorrender.MsgFetchCommunityCatalog3)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, errors.New(errorrender.MsgFetchCommunityCatalog4)
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
			Key:                   key,
			Provider:              strings.ToLower(strings.TrimSpace(entry.LitellmProvider)),
			Mode:                  strings.ToLower(strings.TrimSpace(entry.Mode)),
			MaxInputTokens:        entry.MaxInputTokens,
			SupportsPromptCaching: entry.SupportsPromptCaching,
		})
	}

	if len(entries) == 0 {
		return nil, errors.New(errorrender.MsgFetchCommunityCatalog5)
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

// filterCommunityModels unions the LiteLLM catalog with the models.dev
// facts over one shared target attribution: IDs and names stay verbatim
// community keys (Q4) for community entries; the last key segment is the
// display name, bare keys keep the full key. First target match wins;
// results are deduplicated (case-insensitive across sources) and sorted by
// ID. Same-key entries are enriched in place with models.dev winning
// metadata conflicts; models.dev-only entries matching a target are
// appended with the upstream display name when present.
// Relay partitions (poe/digitalocean/aihubmix, ...) never match: they carry
// no registry slug, so attribution drops them without a special case.
func filterCommunityModels(entries []communityRecord, facts map[string]modelsDevFact, targets []workflow.ProviderEntry) []DetectedModel {
	models := make([]DetectedModel, 0, 64)
	indexByLowerID := make(map[string]int)
	for _, target := range targets {
		for _, e := range entries {
			lower := strings.ToLower(e.Key)
			if _, dup := indexByLowerID[lower]; dup || !communityRecordMatches(e, target) {
				continue
			}
			name := e.Key
			if i := strings.LastIndex(e.Key, "/"); i >= 0 {
				name = e.Key[i+1:]
			}
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			indexByLowerID[lower] = len(models)
			models = append(models, DetectedModel{
				ID:                    e.Key,
				Name:                  name,
				Provider:              target.DisplayName,
				Mode:                  e.Mode,
				ContextOrResolution:   formatContextLength(e.MaxInputTokens),
				SupportsPromptCaching: e.SupportsPromptCaching,
			})
		}
	}
	if len(facts) > 0 {
		ids := make([]string, 0, len(facts))
		for id := range facts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fact := facts[id]
			lower := strings.ToLower(id)
			if idx, dup := indexByLowerID[lower]; dup {
				applyModelsDevEnrichment(&models[idx], fact)
				continue
			}
			for _, target := range targets {
				if !modelsDevRecordMatches(lower, fact, target) {
					continue
				}
				name := strings.TrimSpace(fact.name)
				if name == "" {
					name = lower
					if i := strings.LastIndex(lower, "/"); i >= 0 {
						name = lower[i+1:]
					}
					name = strings.TrimSpace(name)
				}
				if name == "" {
					break
				}
				entry := DetectedModel{ID: lower, Name: name, Provider: target.DisplayName}
				applyModelsDevEnrichment(&entry, fact)
				indexByLowerID[lower] = len(models)
				models = append(models, entry)
				break
			}
		}
	}
	sortModelsByID(models)
	return models
}

// modelsDevRecordMatches reports whether one models.dev fact belongs to one
// provider slice, reusing the community attribution verbatim: the fact ID
// stands in for the community key, the models.dev provider namespace for
// the litellm_provider field.
func modelsDevRecordMatches(id string, fact modelsDevFact, target workflow.ProviderEntry) bool {
	return communityRecordMatches(communityRecord{
		Key:      id,
		Provider: strings.ToLower(strings.TrimSpace(fact.provider)),
	}, target)
}

// applyModelsDevEnrichment overlays models.dev facts onto one detected
// model: modalities, capabilities and limit apply whenever attested; a
// positive limit also reformats the display context (models.dev wins the
// conflict). Description fills only from structured facts (see
// describeModelsDevFact) and flips DescriptionEnriched so refresh flows can
// overwrite auto text while operator-written text stays untouched.
func applyModelsDevEnrichment(model *DetectedModel, fact modelsDevFact) {
	if fact.context > 0 {
		model.ContextLimit = fact.context
		model.ContextOrResolution = formatContextLength(fact.context)
	}
	if fact.modalities != nil {
		model.Modalities = cloneReferenceModalities(fact.modalities)
	}
	if caps := modelsDevCapabilities(fact); len(caps) > 0 {
		model.Capabilities = caps
	}
	if desc := describeModelsDevFact(fact); desc != "" {
		model.Description = desc
		model.DescriptionEnriched = true
	}
}

// modelsDevCapabilities lists the upstream capability flags attested true,
// upstream names verbatim, sorted for determinism. False (unsupported) and
// nil (unknown) stay absent.
func modelsDevCapabilities(fact modelsDevFact) []string {
	flags := []struct {
		name string
		set  *bool
	}{
		{"reasoning", fact.reasoning},
		{"tool_call", fact.toolCall},
		{"temperature", fact.temperature},
		{"attachment", fact.attachment},
		{"structured_output", fact.structuredOutput},
		{"open_weights", fact.openWeights},
	}
	var caps []string
	for _, flag := range flags {
		if flag.set != nil && *flag.set {
			caps = append(caps, flag.name)
		}
	}
	sort.Strings(caps)
	return caps
}

// modelsDevModalityLabels renders common modality tokens for the admin UI;
// unknown tokens pass through verbatim so feed additions stay visible.
var modelsDevModalityLabels = map[string]string{
	"text":  errorrender.MsgModelsDevCapabilities,
	"image": errorrender.MsgModelsDevCapabilities2,
	"audio": errorrender.MsgModelsDevCapabilities3,
	"video": errorrender.MsgModelsDevCapabilities4,
}

// describeModelsDevFact renders a short auto-generated placeholder from
// structured models.dev facts (non-trivial modalities + knowledge cutoff).
// Upstream marketing copy is deliberately never piped (reference-seam
// posture: gateway运营文案优先); empty means nothing worth auto-filling,
// so the entry keeps an empty description with enriched=false.
func describeModelsDevFact(fact modelsDevFact) string {
	var parts []string
	if summary := summarizeModelsDevModalities(fact.modalities); summary != "" {
		parts = append(parts, summary)
	}
	if knowledge := strings.TrimSpace(fact.knowledge); knowledge != "" {
		parts = append(parts, errorrender.MsgDescribeModelsDevFact+knowledge)
	}
	return strings.Join(parts, "；")
}

// summarizeModelsDevModalities names non-text input/output modalities.
// Pure text-to-text is the default and stays silent to avoid noise.
func summarizeModelsDevModalities(modalities *ModelReferenceModalities) string {
	if modalities == nil {
		return ""
	}
	if !hasNonTextModality(modalities.Input) && !hasNonTextModality(modalities.Output) {
		return ""
	}
	var parts []string
	if labels := labelModalities(modalities.Input); len(labels) > 0 {
		parts = append(parts, errorrender.MsgSummarizeModelsDevModalities+strings.Join(labels, "、"))
	}
	if labels := labelModalities(modalities.Output); len(labels) > 0 {
		parts = append(parts, errorrender.MsgSummarizeModelsDevModalities2+strings.Join(labels, "、"))
	}
	return strings.Join(parts, "；")
}

func hasNonTextModality(tokens []string) bool {
	for _, token := range tokens {
		if clean := strings.ToLower(strings.TrimSpace(token)); clean != "" && clean != "text" {
			return true
		}
	}
	return false
}

func labelModalities(tokens []string) []string {
	var labels []string
	for _, token := range tokens {
		clean := strings.TrimSpace(token)
		if clean == "" {
			continue
		}
		if label, ok := modelsDevModalityLabels[strings.ToLower(clean)]; ok {
			labels = append(labels, label)
		} else {
			labels = append(labels, clean)
		}
	}
	return labels
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
	// Binary K throughout (admin display only): exact multiples stay exact,
	// others round to nearest KiB. Previously decimal rounding mixed with a
	// binary special-case, so 1536 displayed as 2K.
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", (tokens+512)/1024)
	}
	return fmt.Sprintf("%d", tokens)
}
