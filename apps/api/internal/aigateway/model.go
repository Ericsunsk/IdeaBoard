package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/database"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/outboundhttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/workflow"
)

// ErrVisionModelMissing reports a vision search model without configuration.
// Workers fail closed on it: no silent default model exists.
var ErrVisionModelMissing = errors.New("AI gateway vision model mapping is not configured")

// visionMappingFeature is the admin API bare key for vision search. It
// deliberately stays out of the service contract: vision is not a
// submittable task, it only needs a model id from the same admin surface.
// The bare string stays "vision" for API compatibility; storage is isolated
// via visionMappingKey below so a future service slug can never collide.
const visionMappingFeature = "vision"

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

// gatewayProbeOutbound is the shared standard outbound client for
// connection diagnosis: timeout cap and observability match the gateway
// migration (#766). Probes stay single-shot end to end (MaxAttempts 1):
// any HTTP status counts as reachable, only transport failures are
// unreachable.
var gatewayProbeOutbound = outboundhttp.NewClient(outboundhttp.Config{})

// gatewayProbeTarget names connection diagnosis in outbound logs/metrics.
const gatewayProbeTarget = "ai-gateway-probe"

// ProbeGatewayConnection probes one provider-native route with a bounded
// timeout. The provided token is used only for this probe and never persisted.
// Purpose split (deliberate): this is transport diagnosis, not availability.
// Any HTTP status counts as reachable (even 401/403 — they still prove
// routing, DNS and TLS); only transport failures are unreachable. Model
// availability questions go through workflow.ClassifyGatewayResponse.
func ProbeGatewayConnection(ctx context.Context, baseURL, token, provider, model, path string) ConnectionProbeResult {
	const probeTimeout = 8 * time.Second

	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ConnectionProbeResult{Message: msgProbeGatewayConnection}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ConnectionProbeResult{Message: msgProbeGatewayConnection2}
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	path = strings.Trim(strings.TrimSpace(path), "/")
	if provider == "" || path == "" {
		return ConnectionProbeResult{Message: msgProbeGatewayConnection3}
	}
	if strings.ContainsAny(provider+path, "?&#\r\n") {
		return ConnectionProbeResult{Message: msgProbeGatewayConnection4}
	}
	probeURL := trimmed + "/" + workflow.GatewayProviderPath(provider) + "/" + path

	start := time.Now()

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(token) != "" {
			req.Header.Set(gatewayHeaderAuthorization, "Bearer "+strings.TrimSpace(token))
		}
		req.Header.Set(gatewayHeaderMaxAttempts, "1")
		req.Header.Set(gatewayHeaderSkipCache, "true")
		if model != "" {
			req.Header.Set(gatewayHeaderModel, model)
		}
		resp, err := gatewayProbeOutbound.Do(ctx, req, outboundhttp.RequestOptions{
			Timeout:     probeTimeout,
			MaxAttempts: 1,
			AllowHosts:  outbound.AllowHosts(ctx),
			Target:      gatewayProbeTarget,
		})
		if err != nil {
			var statusErr *outboundhttp.HTTPStatusError
			if errors.As(err, &statusErr) {
				return gatewayProbeStatusResponse(req, statusErr), nil
			}
			return nil, err
		}
		return resp, nil
	}

	resp, err := doRequest()
	if err != nil {
		return ConnectionProbeResult{Message: msgProbeGatewayConnection5 + err.Error()}
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()
	return ConnectionProbeResult{
		Reachable:  true,
		StatusCode: resp.StatusCode,
		LatencyMs:  latency,
		Message:    fmt.Sprintf(msgProbeGatewayConnection6, latency, resp.StatusCode),
	}
}

// gatewayProbeStatusResponse converts an exhausted retryable probe status
// back to a response shape: diagnosis treats any HTTP status as reachable,
// so a single-shot 429/5xx still proves routing, DNS and TLS.
func gatewayProbeStatusResponse(req *http.Request, statusErr *outboundhttp.HTTPStatusError) *http.Response {
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
// prices. Billing charges solely on the billing SKU UnitCredits; reference
// prices (if needed) come from the read-only models.dev seam, never from
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
	// DescriptionEnriched marks auto-filled detection descriptions: refresh
	// enrichment may overwrite them, operator-written text is never touched.
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
	validated := make([]CatalogItem, 0, len(items))
	for i := range items {
		item := items[i]
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
		if strings.TrimSpace(item.CustomParams) != "" {
			var decoded any
			if err := json.Unmarshal([]byte(item.CustomParams), &decoded); err != nil {
				return fmt.Errorf("item %d custom_params is not valid JSON: %w", i, err)
			}
			if _, ok := decoded.(map[string]any); !ok {
				return fmt.Errorf("item %d custom_params must be a JSON object", i)
			}
		}
		validated = append(validated, item)
	}
	copy(items, validated)
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode AI gateway model catalog: %w", err)
	}
	return r.settings.WithAIGatewayModelMappingWrite(ctx, func(tx settings.SystemConfigWriteTx) (settings.SystemConfigAdminAudit, error) {
		// Read and prune under the same lock as mapping writes so a valid
		// mapping update cannot be deleted using a stale pre-lock snapshot.
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
// (#847: the single-value service mappings are removed, so only the isolated
// vision key can go stale here.)
func staleMappingKeysForRemovedModels(ctx context.Context, tx settings.SystemConfigWriteTx, items []CatalogItem) ([]string, error) {
	keep := make(map[string]struct{}, len(items))
	for _, item := range items {
		keep[strings.TrimSpace(item.ID)] = struct{}{}
	}
	rawCatalog, err := tx.Get(ctx, catalogConfigKey)
	oldItems := DefaultCatalog()
	if err != nil && !database.IsNotFound(err) {
		return nil, fmt.Errorf("load current catalog: %w", err)
	}
	if err == nil && strings.TrimSpace(rawCatalog) != "" {
		if err := json.Unmarshal([]byte(rawCatalog), &oldItems); err != nil {
			return nil, nil
		}
	}
	removed := make(map[string]struct{})
	for _, item := range oldItems {
		if id := strings.TrimSpace(item.ID); id != "" {
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
	if err == nil && isRemovedModel(removed, visionModel) {
		return []string{visionMappingKey}, nil
	}
	return nil, nil
}

func isRemovedModel(removed map[string]struct{}, model string) bool {
	_, ok := removed[strings.TrimSpace(model)]
	return ok
}
