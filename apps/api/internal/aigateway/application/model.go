package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	aigatewayports "ideaboard/api/internal/aigateway/ports"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/workflow"
)

// ErrVisionModelMissing reports a vision search model without configuration.
// Workers fail closed on it: no silent default model exists.
var ErrVisionModelMissing = errors.New("AI gateway vision model mapping is not configured")

// visionMappingKey isolates the vision-search model. Fresh DB: no legacy
// rows exist.
const visionMappingKey = "ai_gateway_vision_model"

// ResolveVisionModel returns the admin-configured vision search model.
// Storage is isolated at visionMappingKey. Unset means fail closed.
func (r *ConfigRepository) ResolveVisionModel(ctx context.Context) (string, error) {
	if r == nil || r.settings == nil {
		return "", errors.New("AI gateway config repository is unavailable")
	}
	raw, err := r.settings.Get(ctx, visionMappingKey)
	if err != nil {
		return "", fmt.Errorf("load AI gateway vision model mapping: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", ErrVisionModelMissing
	}
	return strings.TrimSpace(raw), nil
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
// Preview targets the generic image pipeline, whose path is produced entirely
// by workflow.ResolveRoute (dialects.Resolve delegates to it for Generation
// and preview only reads namespace/path/source): preview output and the
// executed route cannot diverge, and this business package stays free of
// runtime imports per the architecture gate.
func PreviewGatewayRoute(modelID, customPath, customParams string) RoutePreview {
	provider, model := workflow.SplitModelIdentifier(modelID, "")
	// Preview targets the generic image pipeline (ModelImage adapter); pipeline
	// operations with dedicated routes resolve their own paths at execution time.
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

// gatewayProbeOutbound is the shared outbound client for connection diagnosis.
var gatewayProbeOutbound = safehttp.NewClient(safehttp.Config{})

// gatewayProbeTarget names connection diagnosis in outbound logs/metrics.
const gatewayProbeTarget = "ai-gateway-probe"

// ProbeGatewayConnection probes gateway reachability with a bounded
// timeout. The provided token is used only for this probe and never persisted.
// Purpose split (deliberate): this is transport diagnosis, not availability.
// Any HTTP status counts as reachable (even 401/403 — they still prove
// routing, DNS and TLS); only transport failures are unreachable.
//
// Target selection: provider+path both empty probes the gateway root URL
// directly (any HTTP status still proves DNS/TLS/routing; CF exposes no
// unified health endpoint, so the root URL is the gateway-liveness probe).
// Exactly one of provider/path empty is rejected as ambiguous (no guessing).
func ProbeGatewayConnection(ctx context.Context, baseURL, token, provider, model, path string) ConnectionProbeResult {
	const probeTimeout = 8 * time.Second

	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection2}
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	path = strings.Trim(strings.TrimSpace(path), "/")
	onlyOneRoutePart := (provider == "") != (path == "")
	if onlyOneRoutePart {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection3}
	}
	if strings.ContainsAny(provider+path, "?&#\r\n") {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection4}
	}
	probeURL := trimmed
	if provider != "" {
		probeURL = trimmed + "/" + workflow.GatewayProviderPath(provider) + "/" + path
	}

	start := time.Now()

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(token) != "" {
			req.Header.Set(aigatewayports.GatewayHeaderAuthorization, "Bearer "+strings.TrimSpace(token))
		}
		req.Header.Set(aigatewayports.GatewayHeaderMaxAttempts, "1")
		req.Header.Set(aigatewayports.GatewayHeaderSkipCache, "true")
		if model != "" {
			req.Header.Set(aigatewayports.GatewayHeaderModel, model)
		}
		resp, err := gatewayProbeOutbound.Do(ctx, req, safehttp.RequestOptions{
			Timeout:     probeTimeout,
			MaxAttempts: 1,
			AllowHosts:  safehttp.AllowHosts(ctx),
			Target:      gatewayProbeTarget,
		})
		if err != nil {
			var statusErr *safehttp.HTTPStatusError
			if errors.As(err, &statusErr) {
				return gatewayProbeStatusResponse(req, statusErr), nil
			}
			return nil, err
		}
		return resp, nil
	}

	resp, err := doRequest()
	if err != nil {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection5 + err.Error()}
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()
	return ConnectionProbeResult{
		Reachable:  true,
		StatusCode: resp.StatusCode,
		LatencyMs:  latency,
		Message:    fmt.Sprintf(errorrender.MsgProbeGatewayConnection6, latency, resp.StatusCode),
	}
}

// gatewayProbeStatusResponse converts an exhausted retryable probe status
// back to a response shape: diagnosis treats any HTTP status as reachable,
// so a single-shot 429/5xx still proves routing, DNS and TLS.
func gatewayProbeStatusResponse(req *http.Request, statusErr *safehttp.HTTPStatusError) *http.Response {
	preview := ""
	status := 0
	if statusErr != nil {
		preview = statusErr.Body
		status = statusErr.StatusCode
	}
	resp := &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(preview)),
		ContentLength: int64(len(preview)),
		Request:       req,
	}
	return resp
}

// catalogConfigKey stores the JSON array of available model items.
const catalogConfigKey = "ai_gateway_model_catalog"

// CatalogItem describes one model available in the AI gateway catalog.
// The catalog carries routing and display metadata only: it holds no
// prices. Billing charges solely on the billing SKU UnitCredits, never from
// stored catalog data.
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
	// DescriptionEnriched distinguishes replaceable import text (true)
	// from operator-written text (false, never overwritten).
	DescriptionEnriched   bool `json:"description_enriched,omitempty"`
	SupportsPromptCaching bool `json:"supports_prompt_caching,omitempty"`
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
		if database.IsNotFound(err) {
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

// normalizeCatalogItems is the catalog write validation seam, independent of storage.
func normalizeCatalogItems(items []CatalogItem) ([]CatalogItem, error) {
	if len(items) > maxCatalogItems {
		return nil, fmt.Errorf("model catalog exceeds %d items", maxCatalogItems)
	}
	seenModels := make(map[string]struct{}, len(items))
	validated := make([]CatalogItem, 0, len(items))
	for i := range items {
		item := items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
		if item.ID == "" {
			return nil, fmt.Errorf("item %d is missing ID", i)
		}
		if len(item.ID) > maxCatalogIDLength {
			return nil, fmt.Errorf("item %d ID exceeds %d characters", i, maxCatalogIDLength)
		}
		if item.Provider == "" || strings.ContainsAny(item.Provider, "/@ \t\n") {
			return nil, fmt.Errorf("item %d requires a provider slug", i)
		}
		key := item.Provider + "\x00" + strings.ToLower(item.ID)
		if _, dup := seenModels[key]; dup {
			return nil, fmt.Errorf("duplicate model %q", workflow.FormatPoolModel(item.Provider, item.ID))
		}
		seenModels[key] = struct{}{}
		if item.Name == "" {
			item.Name = item.ID
		}
		if err := validateCatalogItemBounds(i, item); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.CustomParams) != "" {
			var decoded any
			if err := json.Unmarshal([]byte(item.CustomParams), &decoded); err != nil {
				return nil, fmt.Errorf("item %d custom_params is not valid JSON: %w", i, err)
			}
			if _, ok := decoded.(map[string]any); !ok {
				return nil, fmt.Errorf("item %d custom_params must be a JSON object", i)
			}
		}
		validated = append(validated, item)
	}
	return validated, nil
}

func (r *ConfigRepository) SaveCatalog(ctx context.Context, items []CatalogItem, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	if actorID <= 0 {
		return errors.New("admin actor is required")
	}
	validated, err := normalizeCatalogItems(items)
	if err != nil {
		return err
	}
	copy(items, validated)
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode AI gateway model catalog: %w", err)
	}
	return r.settings.WithAIGatewayCatalogWrite(ctx, func(tx settings.SystemConfigWriteTx) (settings.SystemConfigAdminAudit, error) {
		// Read and prune under the same catalog-write lock so a valid mapping
		// update cannot be deleted using a stale pre-lock snapshot.
		removedMappingKeys, err := staleMappingKeysForRemovedModels(ctx, tx, items)
		if err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("read AI gateway catalog mappings: %w", err)
		}
		metadata, err := json.Marshal(map[string]any{"operation": "save_catalog", "count": len(items), "removed_mappings": removedMappingKeys})
		if err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("encode AI gateway catalog audit: %w", err)
		}
		if err := tx.SetMany(ctx, []settings.SystemConfigValue{{Key: catalogConfigKey, Value: string(encoded)}}); err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("save AI gateway model catalog: %w", err)
		}
		if err := tx.DeleteMany(ctx, removedMappingKeys); err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("delete stale AI gateway vision mapping: %w", err)
		}
		return settings.SystemConfigAdminAudit{
			ActionType: "update_ai_gateway_model",
			ActorID:    actorID,
			Metadata:   metadata,
		}, nil
	})
}

// staleMappingKeysForRemovedModels returns storage keys that point at models
// absent from the incoming catalog. Corrupt catalog JSON skips pruning so an
// admin can replace it; database errors abort the transaction.
func staleMappingKeysForRemovedModels(ctx context.Context, tx settings.SystemConfigWriteTx, items []CatalogItem) ([]string, error) {
	keep := make(map[string]struct{}, len(items))
	for _, item := range items {
		keep[strings.ToLower(workflow.FormatPoolModel(item.Provider, item.ID))] = struct{}{}
	}
	rawCatalog, err := tx.Get(ctx, catalogConfigKey)
	oldItems := DefaultCatalog()
	if err != nil && !database.IsNotFound(err) {
		return nil, fmt.Errorf("load current catalog: %w", err)
	}
	if err == nil && strings.TrimSpace(rawCatalog) != "" {
		if err := json.Unmarshal([]byte(rawCatalog), &oldItems); err != nil {
			oldItems = nil
		}
	}
	removed := make(map[string]struct{})
	for _, item := range oldItems {
		if id := strings.ToLower(workflow.FormatPoolModel(item.Provider, item.ID)); strings.TrimSpace(item.ID) != "" {
			if _, ok := keep[id]; !ok {
				removed[id] = struct{}{}
			}
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	visionModel, err := tx.Get(ctx, visionMappingKey)
	if err != nil && !database.IsNotFound(err) {
		return nil, fmt.Errorf("load current vision mapping: %w", err)
	}
	if err == nil {
		if _, ok := removed[strings.ToLower(strings.TrimSpace(visionModel))]; ok {
			return []string{visionMappingKey}, nil
		}
	}
	return nil, nil
}
