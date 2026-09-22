package aigateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/workflow"
)

// ErrModelMappingMissing reports a feature without a configured model.
// Workers fail closed on it: no silent default model exists.
var ErrModelMappingMissing = errors.New("AI gateway model mapping is not configured for this feature")

// modelMappingKeyPrefix scopes per-feature workflow model selections in settings.
const modelMappingKeyPrefix = "ai_gateway_model_"

// modelMappingKey renders the settings key for one workflow feature slug.
func modelMappingKey(feature string) string { return modelMappingKeyPrefix + feature }

// visionMappingFeature is the admin API bare key for vision search. It
// deliberately stays out of the workflow feature contract: vision is not a
// submittable task, it only needs a model id from the same admin surface.
// The bare string stays "vision" for API compatibility; storage is isolated
// via visionMappingKey below so a future workflow slug can never collide.
const visionMappingFeature = "vision"

// visionMappingKey isolates the vision-search model from workflow mappings.
// It lives outside modelMappingKeyPrefix on purpose: workflow mappings use
// ai_gateway_model_<slug>, vision uses ai_gateway_vision_model, so the two key
// spaces never intersect. Fresh DB: no legacy rows exist.
const visionMappingKey = "ai_gateway_vision_model"

// validateModelMapping checks an admin mapping without touching storage.
// Model IDs stay provider-qualified (provider/model) so dialect dispatch
// keeps working, but providers are never allowlisted here: hardcoding them
// would break zero-code supplier onboarding. Errors are safe to display.
func validateModelMapping(feature, model string) (string, string, error) {
	slug := strings.ToLower(strings.TrimSpace(feature))
	if slug != visionMappingFeature {
		var err error
		slug, err = workflow.ValidateWorkflowFeatureSlug(feature)
		if err != nil {
			return "", "", err
		}
	}
	model = strings.TrimSpace(model)
	if !strings.Contains(model, "/") || strings.ContainsAny(model, " \t\n") {
		return "", "", errors.New("模型必须是 provider/model 格式且不含空白")
	}
	if len(model) > 128 {
		return "", "", errors.New("模型标识过长")
	}
	provider := strings.TrimSpace(strings.SplitN(model, "/", 2)[0])
	if provider == "" {
		return "", "", errors.New("模型缺少 provider 前缀")
	}
	return slug, model, nil
}

// ResolveModel returns the admin-configured model for one feature at
// request time. Unset features fail closed.
func (r *ConfigRepository) ResolveModel(ctx context.Context, feature string) (string, error) {
	if r == nil || r.settings == nil {
		return "", errors.New("AI gateway config repository is unavailable")
	}
	slug, err := workflow.ValidateWorkflowFeatureSlug(feature)
	if err != nil {
		return "", err
	}
	raw, err := r.settings.Get(ctx, modelMappingKey(slug))
	if err != nil {
		return "", fmt.Errorf("load AI gateway model mapping: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", ErrModelMappingMissing
	}
	return strings.TrimSpace(raw), nil
}

// ResolveVisionModel returns the admin-configured vision search model.
// Storage is isolated at visionMappingKey. Unset means fail closed,
// exactly like task mappings.
func (r *ConfigRepository) ResolveVisionModel(ctx context.Context) (string, error) {
	if r == nil || r.settings == nil {
		return "", errors.New("AI gateway config repository is unavailable")
	}
	raw, err := r.settings.Get(ctx, visionMappingKey)
	if err != nil {
		return "", fmt.Errorf("load AI gateway vision model mapping: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", ErrModelMappingMissing
	}
	return strings.TrimSpace(raw), nil
}

// ListModelMappings returns every configured feature mapping for admin display.
// API contract is unchanged (bare "vision" + workflow slugs). Storage: workflow
// mappings come from the prefix scan, vision comes from visionMappingKey.
func (r *ConfigRepository) ListModelMappings(ctx context.Context) (map[string]string, error) {
	if r == nil || r.settings == nil {
		return nil, errors.New("AI gateway config repository is unavailable")
	}
	values, err := r.settings.GetByPrefix(ctx, modelMappingKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("load AI gateway model mappings: %w", err)
	}
	result := make(map[string]string, len(values)+1)
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		// 目录行与映射共享 key 前缀：后缀 "catalog" 只可能是目录本身，
		// workflow slug 由 ValidateWorkflowFeatureSlug 把关，不可能叫这个。
		if key == "catalog" {
			continue
		}
		result[strings.TrimPrefix(key, modelMappingKeyPrefix)] = strings.TrimSpace(value)
	}
	visionRaw, visionErr := r.settings.Get(ctx, visionMappingKey)
	if visionErr != nil && !errors.Is(visionErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("load AI gateway vision model mapping: %w", visionErr)
	}
	if strings.TrimSpace(visionRaw) != "" {
		result[visionMappingFeature] = strings.TrimSpace(visionRaw)
	}
	return result, nil
}

// SaveModelMapping validates and persists one feature mapping atomically
// with its admin audit record. Vision ("vision") writes to the isolated
// visionMappingKey; workflow slugs write under the shared prefix. The two key
// spaces never intersect, so cross-namespace overwrites are impossible.
// It routes through the locked setter so concurrent mapping saves serialize
// against catalog cascades (same advisory lock, no lost updates).
func (r *ConfigRepository) SaveModelMapping(ctx context.Context, feature, model string, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	if actorID <= 0 {
		return errors.New("admin actor is required")
	}
	slug, normalized, err := validateModelMapping(feature, model)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{"operation": "save_model", "feature": slug, "model": normalized})
	if err != nil {
		return fmt.Errorf("encode AI gateway model audit: %w", err)
	}
	storageKey := modelMappingKey(slug)
	if slug == visionMappingFeature {
		storageKey = visionMappingKey
	}
	return r.settings.SetAndDeleteManyWithAdminAudit(ctx, []settings.SystemConfigValue{
		{Key: storageKey, Value: normalized},
	}, nil, settings.SystemConfigAdminAudit{
		ActionType: "update_ai_gateway_model",
		ActorID:    actorID,
		Metadata:   metadata,
	})
}

// RoutePreview is a read-only diagnosis of where a model identifier would
// route: provider namespace plus the default image-pipeline path. Pipeline
// operations with dedicated routes (cutout/upscale) resolve their own paths
// at execution time.
type RoutePreview struct {
	ProviderSlug    string `json:"provider_slug"`
	IsFirstParty    bool   `json:"is_first_party"`
	TargetPath      string `json:"target_path"`
	PathSource      string `json:"path_source"`
	PathSourceLabel string `json:"path_source_label"`
}

// PreviewGatewayRoute resolves routing facts without touching configuration.
// It shares workflow.ResolveRoute with execution: preview output and the
// executed route cannot diverge.
func PreviewGatewayRoute(modelID, customPath, customParams string) RoutePreview {
	provider, model := workflow.SplitModelIdentifier(modelID, "")
	resolved := workflow.ResolveRoute(provider, model, customPath, customParams)
	return RoutePreview{
		ProviderSlug:    resolved.Namespace,
		IsFirstParty:    workflow.IsFirstPartyGatewayProvider(provider),
		TargetPath:      resolved.Path,
		PathSource:      string(resolved.Source),
		PathSourceLabel: resolved.Source.Label(),
	}
}

// ConnectionProbeResult is a stateless reachability diagnosis: any HTTP
// status received from the target counts as reachable (even 4xx/5xx, which
// still prove routing, DNS, and TLS work); only transport-level failures
// (DNS, refused, timeout) count as unreachable.
type ConnectionProbeResult struct {
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Message    string `json:"message"`
}

// ProbeGatewayConnection dials baseURL with a bounded timeout. The provided
// token is used only for this probe and never persisted.
// Purpose split (deliberate): this is transport diagnosis, not availability.
// Any HTTP status counts as reachable (even 401/403 — they still prove
// routing, DNS and TLS); only transport failures are unreachable. Model
// availability questions go through workflow.ClassifyGatewayResponse.
func ProbeGatewayConnection(ctx context.Context, baseURL, token string) ConnectionProbeResult {
	const probeTimeout = 8 * time.Second

	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ConnectionProbeResult{Message: "网关地址为空"}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return ConnectionProbeResult{Message: "网关地址必须是 http(s) URL"}
	}

	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	doRequest := func(method string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(probeCtx, method, trimmed, nil)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(token) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		}
		return http.DefaultClient.Do(req)
	}

	resp, err := doRequest(http.MethodHead)
	if err != nil {
		return ConnectionProbeResult{Message: "网关连接失败: " + err.Error()}
	}
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		// 该端点不支持 HEAD，降级为 GET（不读 body）。
		resp.Body.Close()
		resp, err = doRequest(http.MethodGet)
		if err != nil {
			return ConnectionProbeResult{Message: "网关连接失败: " + err.Error()}
		}
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()
	return ConnectionProbeResult{
		Reachable:  true,
		StatusCode: resp.StatusCode,
		LatencyMs:  latency,
		Message:    fmt.Sprintf("网关连接正常 (%dms, HTTP %d)", latency, resp.StatusCode),
	}
}

// catalogConfigKey stores the JSON array of available model items.
const catalogConfigKey = "ai_gateway_model_catalog"

// CatalogItem describes one model available in the AI gateway catalog.
// Cost fields are reference data mirrored from detection (USD per token),
// never billing input.
type CatalogItem struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	Mode                string   `json:"mode,omitempty"`
	Description         string   `json:"description"`
	ContextOrResolution string   `json:"context_or_resolution"`
	Capabilities        []string `json:"capabilities"`
	Hosting             string   `json:"hosting"`
	TermsURL            string   `json:"terms_url"`
	DocsURL             string   `json:"docs_url"`
	CustomPath          string   `json:"custom_path,omitempty"`
	CustomParams        string   `json:"custom_params,omitempty"`
	// DescriptionEnriched marks auto-filled detection descriptions: refresh
	// enrichment may overwrite them, operator-written text is never touched.
	DescriptionEnriched     bool    `json:"description_enriched,omitempty"`
	InputCostPerToken       float64 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken      float64 `json:"output_cost_per_token,omitempty"`
	CacheReadCostPerToken   float64 `json:"cache_read_input_token_cost,omitempty"`
	CacheCreateCostPerToken float64 `json:"cache_creation_input_token_cost,omitempty"`
	SupportsPromptCaching   bool    `json:"supports_prompt_caching,omitempty"`
}

// DefaultCatalog returns no built-in models: the catalog is fully
// operator-managed through the admin model library. An empty slice keeps
// ListCatalog/SaveCatalog semantics (empty configured catalog) unchanged.
func DefaultCatalog() []CatalogItem {
	return []CatalogItem{}
}

// ListCatalog returns the configured model catalog items, falling back to
// the empty default catalog when nothing is configured. Stored JSON that no
// longer parses is a data-integrity error, never silently replaced: replacing
// it with defaults would let the next save permanently erase operator data.
func (r *ConfigRepository) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	if r == nil || r.settings == nil {
		return nil, errors.New("AI gateway config repository is unavailable")
	}
	raw, err := r.settings.Get(ctx, catalogConfigKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultCatalog(), nil
		}
		return nil, fmt.Errorf("load AI gateway model catalog: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return DefaultCatalog(), nil
	}
	var items []CatalogItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("stored AI gateway model catalog is corrupt: %w", err)
	}
	return items, nil
}

// SaveCatalog validates and persists the model catalog atomically with an audit action.
const (
	maxCatalogItems        = 500
	maxCatalogIDLength     = 128
	maxCatalogTextField    = 2000
	maxCatalogCapabilities = 32
)

func validateCatalogItemBounds(index int, item CatalogItem) error {
	for field, value := range map[string]string{
		"name": item.Name, "provider": item.Provider, "mode": item.Mode,
		"description": item.Description, "context_or_resolution": item.ContextOrResolution,
		"hosting": item.Hosting, "terms_url": item.TermsURL, "docs_url": item.DocsURL,
		"custom_path": item.CustomPath, "custom_params": item.CustomParams,
	} {
		if len(value) > maxCatalogTextField {
			return fmt.Errorf("item %d %s exceeds %d characters", index, field, maxCatalogTextField)
		}
	}
	if len(item.Capabilities) > maxCatalogCapabilities {
		return fmt.Errorf("item %d exceeds %d capabilities", index, maxCatalogCapabilities)
	}
	return nil
}

func (r *ConfigRepository) SaveCatalog(ctx context.Context, items []CatalogItem, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	if actorID <= 0 {
		return errors.New("admin actor is required")
	}
	if len(items) > maxCatalogItems {
		return fmt.Errorf("model catalog exceeds %d items", maxCatalogItems)
	}
	seenIDs := make(map[string]struct{}, len(items))
	for i, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Provider = strings.TrimSpace(item.Provider)
		if item.ID == "" {
			return fmt.Errorf("item %d is missing ID", i)
		}
		if len(item.ID) > maxCatalogIDLength {
			return fmt.Errorf("item %d ID exceeds %d characters", i, maxCatalogIDLength)
		}
		loweredID := strings.ToLower(item.ID)
		if _, dup := seenIDs[loweredID]; dup {
			return fmt.Errorf("duplicate model ID %q", item.ID)
		}
		seenIDs[loweredID] = struct{}{}
		if item.Name == "" {
			item.Name = item.ID
		}
		if item.Provider == "" {
			if strings.Contains(item.ID, "/") {
				item.Provider = strings.SplitN(item.ID, "/", 2)[0]
			} else {
				item.Provider = "Custom"
			}
		}
		if err := validateCatalogItemBounds(i, item); err != nil {
			return err
		}
		for field, value := range map[string]float64{
			"input_cost_per_token": item.InputCostPerToken, "output_cost_per_token": item.OutputCostPerToken,
			"cache_read_input_token_cost": item.CacheReadCostPerToken, "cache_creation_input_token_cost": item.CacheCreateCostPerToken,
		} {
			if value < 0 {
				return fmt.Errorf("item %d %s must not be negative", i, field)
			}
		}
		if strings.TrimSpace(item.CustomParams) != "" {
			var decoded any
			if err := json.Unmarshal([]byte(item.CustomParams), &decoded); err != nil {
				return fmt.Errorf("item %d custom_params is not valid JSON: %w", i, err)
			}
			if _, ok := decoded.(map[string]any); !ok {
				return fmt.Errorf("item %d custom_params must be a JSON object", i)
			}
		}
		items[i] = item
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode AI gateway model catalog: %w", err)
	}
	// 级联：新目录删掉的模型 ID，其映射（功能映射 + vision）一并移除，
	// 否则定价页残留指向不存在模型的映射。基线/映射读失败时跳过级联
	// 只存目录（保留 corrupt 数据的覆盖恢复路径），不改变保存语义。
	removedMappingKeys := staleMappingKeysForRemovedModels(ctx, r, items)
	metadata, err := json.Marshal(map[string]any{"operation": "save_catalog", "count": len(items), "removed_mappings": removedMappingKeys})
	if err != nil {
		return fmt.Errorf("encode AI gateway catalog audit: %w", err)
	}
	return r.settings.SetAndDeleteManyWithAdminAudit(ctx, []settings.SystemConfigValue{
		{Key: catalogConfigKey, Value: string(encoded)},
	}, removedMappingKeys, settings.SystemConfigAdminAudit{
		ActionType: "update_ai_gateway_model",
		ActorID:    actorID,
		Metadata:   metadata,
	})
}

// staleMappingKeysForRemovedModels returns the storage keys of mappings
// pointing at model IDs absent from the incoming catalog. Read failures
// yield nil (skip cascade, still save) so a corrupt baseline never blocks
// catalog recovery.
func staleMappingKeysForRemovedModels(ctx context.Context, r *ConfigRepository, items []CatalogItem) []string {
	keep := make(map[string]bool, len(items))
	for _, item := range items {
		keep[strings.TrimSpace(item.ID)] = true
	}
	oldItems, err := r.ListCatalog(ctx)
	if err != nil {
		return nil
	}
	removed := make(map[string]bool)
	for _, item := range oldItems {
		if id := strings.TrimSpace(item.ID); id != "" && !keep[id] {
			removed[id] = true
		}
	}
	if len(removed) == 0 {
		return nil
	}
	mappings, err := r.ListModelMappings(ctx)
	if err != nil {
		return nil
	}
	var keys []string
	for feature, model := range mappings {
		if !removed[strings.TrimSpace(model)] {
			continue
		}
		if feature == visionMappingFeature {
			keys = append(keys, visionMappingKey)
			continue
		}
		keys = append(keys, modelMappingKey(feature))
	}
	return keys
}
