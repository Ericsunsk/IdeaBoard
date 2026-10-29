package application

import (
	"context"
	"encoding/json"
	aigatewaydomain "ideaboard/api/internal/aigateway/domain"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// configStore is the narrow seam the admin handler needs. ConfigRepository
// satisfies it; tests inject fakes.
type configStore interface {
	GetSummary(ctx context.Context) (GatewaySummary, error)
	Update(ctx context.Context, input UpdateInput, actorID int64) error
	ListCatalog(ctx context.Context) ([]CatalogItem, error)
	SaveCatalog(ctx context.Context, items []CatalogItem, actorID int64) error
	DetectModels(ctx context.Context, provider string) (ModelDetection, error)
	GetModelReference(ctx context.Context, modelID, provider, model string, refresh bool) (ModelReference, error)
}

// Handler serves the Admin AI gateway settings endpoints.
type Handler struct {
	store configStore
	// observed is the read-only 7-day model observation service, attached
	// once at composition time via SetObservedService. Nil keeps
	// ListModelsObserved fail-closed.
	observed *aigatewaydomain.ObservedService
}

// NewHandler builds the admin handler over the config store.
func NewHandler(store configStore) *Handler {
	return &Handler{store: store}
}

type updateGatewayRequest struct {
	BaseURL   string  `json:"base_url"`
	Token     *string `json:"token"`
	AccountID string  `json:"account_id"`
	GatewayID string  `json:"gateway_id"`
}

// Get returns the masked gateway configuration.
func (h *Handler) Get(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	summary, err := h.store.GetSummary(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgGet2, err)
		return
	}
	response.SuccessDTO(c, summary)
}

// Update rotates the gateway token and identity. A missing token keeps the
// stored secret; base URL or account and gateway IDs are required.
// Authorization rides the outer Control Plane guard (controlProxy + guard
// enforces the operator capability); the handler resolves the actor from
// the request principal without re-checking the admin role.
func (h *Handler) Update(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	var req updateGatewayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errorrender.MsgAigatewayApplicationUpdate)
		return
	}
	identity, ok := principal.Require(c)
	if !ok {
		return
	}
	err := h.store.Update(c.Request.Context(), UpdateInput{
		BaseURL:   strings.TrimSpace(req.BaseURL),
		Token:     req.Token,
		AccountID: strings.TrimSpace(req.AccountID),
		GatewayID: strings.TrimSpace(req.GatewayID),
	}, identity.UserID())
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil)
}

type previewGatewayRouteRequest struct {
	ModelID      string `json:"model_id"`
	CustomPath   string `json:"custom_path"`
	CustomParams string `json:"custom_params"`
}

// PreviewGatewayRoute resolves routing facts for a model identifier without
// touching configuration. Pure function of the request; no store access.
// Authorization rides the outer Control Plane guard (controlProxy + guard).
func (h *Handler) PreviewGatewayRoute(c *gin.Context) {
	var req previewGatewayRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errorrender.MsgAigatewayApplicationUpdate)
		return
	}
	if strings.TrimSpace(req.ModelID) == "" {
		response.Error(c, http.StatusBadRequest, errorrender.MsgPreviewGatewayRoute)
		return
	}
	preview := PreviewGatewayRoute(req.ModelID, req.CustomPath, req.CustomParams)
	response.SuccessDTO(c, preview)
}

type testGatewayConnectionRequest struct {
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Path     string `json:"path"`
}

// TestConnection probes gateway reachability server-side (browsers cannot
// read cross-origin status). The token is used for this probe only and never
// persisted. Pure diagnosis: no configuration is read or written.
// Authorization rides the outer Control Plane guard.
func (h *Handler) TestConnection(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	var req testGatewayConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errorrender.MsgAigatewayApplicationUpdate)
		return
	}
	provider, model, path := strings.TrimSpace(req.Provider), strings.TrimSpace(req.Model), strings.TrimSpace(req.Path)
	if provider == "" || path == "" {
		response.Error(c, http.StatusBadRequest, errorrender.MsgTestConnection)
		return
	}
	result := ProbeGatewayConnection(c.Request.Context(), req.BaseURL, req.Token, provider, model, path)
	response.SuccessDTO(c, result)
}

// ListCatalog returns every model item in the AI gateway catalog.
func (h *Handler) ListCatalog(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	catalog, err := h.store.ListCatalog(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgListCatalog, err)
		return
	}
	response.SuccessDTO(c, catalog)
}

type saveCatalogRequest struct {
	Items []CatalogItem `json:"items"`
}

// SaveCatalog updates the full model catalog in system settings.
// Decoding is strict: unknown fields (including the removed per-token
// price fields) are rejected instead of silently ignored, so legacy
// writers fail loudly and migrate to the models.dev reference seam.
// Authorization rides the outer Control Plane guard; the actor is resolved
// from the request principal.
func (h *Handler) SaveCatalog(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	var req saveCatalogRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			response.Error(c, http.StatusBadRequest, errorrender.MsgSaveCatalog)
			return
		}
		response.Error(c, http.StatusBadRequest, errorrender.MsgAigatewayApplicationUpdate)
		return
	}
	identity, ok := principal.Require(c)
	if !ok {
		return
	}
	if err := h.store.SaveCatalog(c.Request.Context(), req.Items, identity.UserID()); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil)
}

// ProviderOption is one registry row surfaced for provider quick-pick UIs:
// canonical slug, display name and the ID input hint (native key shape).
type ProviderOption struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	IDPrefix    string `json:"id_prefix"`
}

// ListProviders serves the registry without touching configuration. Pure
// metadata read; authorization rides the outer Control Plane guard.
func (h *Handler) ListProviders(c *gin.Context) {
	entries := workflow.ListProviders()
	options := make([]ProviderOption, 0, len(entries))
	for _, entry := range entries {
		options = append(options, ProviderOption{
			Slug:        entry.Slug,
			DisplayName: entry.DisplayName,
			IDPrefix:    entry.IDPrefix(),
		})
	}
	response.SuccessDTO(c, options)
}

// DetectModels probes the gateway for available models.
// Authorization rides the outer Control Plane guard.
func (h *Handler) DetectModels(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	provider := strings.TrimSpace(c.Query("provider"))
	detection, err := h.store.DetectModels(c.Request.Context(), provider)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.SuccessDTO(c, detection)
}

// GetModelReference serves the read-only models.dev reference lookup for
// one model. Degraded outcomes (allowlist gap, upstream failure, miss)
// return HTTP 200 with Found=false and an explicit error_code inside the
// payload, never an HTTP error: reference display must not block catalog
// flows. Only invalid input (missing model identity) is a 400.
// Authorization rides the outer Control Plane guard.
func (h *Handler) GetModelReference(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	reference, err := h.store.GetModelReference(
		c.Request.Context(),
		strings.TrimSpace(c.Query("model_id")),
		strings.TrimSpace(c.Query("provider")),
		strings.TrimSpace(c.Query("model")),
		parseRefreshParam(c.Query("refresh")),
	)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.SuccessDTO(c, reference)
}

// parseRefreshParam accepts truthy refresh values for cache bypass.
func parseRefreshParam(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// merged from internal/aigateway/application/observed_handler.go (structural consolidation; symbols unchanged).
// SetObservedService attaches the read-only model observation service. It is
// set once at composition time (same pattern as vision billing); a nil
// service keeps the endpoint fail-closed with 503.
func (h *Handler) SetObservedService(svc *aigatewaydomain.ObservedService) {
	if h == nil {
		return
	}
	h.observed = svc
}

// ListModelsObserved serves the admin read-only 7-day first-party model
// observations: per-model success rate, top failure stage/category, cache hit
// rate, plus on-sale SKU UnitCredits margin rows. Gateway actual cost has no
// receipt/log source, so every margin row carries an explicit unavailable
// marker instead of a fabricated number. Models absent from the payload have
// neither attempts nor mappings in the window; callers render 暂无数据.
// Authorization rides the outer Control Plane guard (controlProxy + guard).
func (h *Handler) ListModelsObserved(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.observed != nil, errorrender.MsgListModelsObserved) {
		return
	}
	rows, err := h.observed.ListModelsObserved(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgListModelsObserved2, err)
		return
	}
	response.SuccessDTO(c, rows)
}
