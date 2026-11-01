package application

import (
	"encoding/json"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/modelregistry/domain"
	modelregistryports "ideaboard/api/internal/modelregistry/ports"
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"
	"ideaboard/api/internal/platform/response"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler serves the Admin AI settings endpoints. The workerBaseURL and
// workerSecret carry the ai-proxy Worker egress (env config) for
// Preview dry-runs; they are set once at composition time via
// SetWorkerConfig and never leave the server (dry-run auth stays
// server-side, browsers only see reachable/status).
type Handler struct {
	store modelregistryports.ConfigRepository
	// observed is the read-only 7-day model observation seam, attached
	// once at composition time via SetObservedService. Nil keeps
	// ListModelsObserved fail-closed.
	observed modelregistryports.ObservedReader
	// workerBaseURL/workerSecret power Preview dry-runs. Empty keeps
	// Preview validation-only (reachable false, never an error).
	workerBaseURL string
	workerSecret  string
}

// NewHandler builds the admin handler over the config seam.
func NewHandler(store modelregistryports.ConfigRepository) *Handler {
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
	err := h.store.Update(c.Request.Context(), domain.UpdateInput{
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

type previewWorkerRouteRequest struct {
	ModelID      string `json:"model_id"`
	CustomParams string `json:"custom_params"`
}

// PreviewWorkerRoute validates one model identifier and dry-runs it
// against the Worker when Worker credentials are configured (no billing,
// no storage writes). Validation-only when unconfigured. The route name
// stays for Admin compatibility; the logic is Worker-only.
// Authorization rides the outer Control Plane guard (controlProxy + guard).
func (h *Handler) PreviewWorkerRoute(c *gin.Context) {
	var req previewWorkerRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errorrender.MsgAigatewayApplicationUpdate)
		return
	}
	if strings.TrimSpace(req.ModelID) == "" {
		response.Error(c, http.StatusBadRequest, errorrender.MsgPreviewWorkerRoute)
		return
	}
	var baseURL, secret string
	if h != nil {
		baseURL, secret = h.workerBaseURL, h.workerSecret
	}
	preview := PreviewWorkerRoute(c.Request.Context(), baseURL, secret, req.ModelID, req.CustomParams)
	response.SuccessDTO(c, preview)
}

type testGatewayConnectionRequest struct {
	BaseURL string `json:"base_url"`
	Secret  string `json:"secret"`
	Model   string `json:"model"`
}

// TestConnection dry-runs one model against the Worker server-side
// (browsers cannot read cross-origin status). The secret is used for this
// probe only and never persisted. Pure diagnosis: no configuration is read
// or written.
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
	if strings.TrimSpace(req.Model) == "" {
		response.Error(c, http.StatusBadRequest, errorrender.MsgPreviewWorkerRoute)
		return
	}
	result := ProbeWorkerConnection(c.Request.Context(), req.BaseURL, req.Secret, req.Model)
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
	Items []domain.CatalogItem `json:"items"`
}

// SaveCatalog updates the full model catalog in system settings.
// Decoding is strict: unknown fields are rejected instead of silently
// ignored, so legacy writers fail loudly.
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

// ListProviders serves the configured providers (control-plane
// provider_configs plus the workers-ai first-party constant) for provider
// quick-pick UIs: canonical slug, display name and the ID input hint.
// Authorization rides the outer Control Plane guard.
func (h *Handler) ListProviders(c *gin.Context) {
	if !platformhttp.RequireDependency(c, h != nil && h.store != nil, errorrender.MsgAigatewayApplicationGet) {
		return
	}
	entries, err := h.store.ListConfiguredProviders(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
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

// SetObservedService attaches the read-only model observation seam. It is
// set once at composition time (same pattern as vision billing); a nil
// service keeps the endpoint fail-closed with 503.
func (h *Handler) SetObservedService(svc modelregistryports.ObservedReader) {
	if h == nil {
		return
	}
	h.observed = svc
}

// SetWorkerConfig attaches the ai-proxy Worker egress (env config) for
// Preview dry-runs. It is set once at composition time; empty keeps Preview
// validation-only (reachable false, never an error).
func (h *Handler) SetWorkerConfig(baseURL, secret string) {
	if h == nil {
		return
	}
	h.workerBaseURL = strings.TrimSpace(baseURL)
	h.workerSecret = secret
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
