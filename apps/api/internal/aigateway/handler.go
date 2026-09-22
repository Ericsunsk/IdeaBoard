package aigateway

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

// configStore is the narrow seam the admin handler needs. ConfigRepository
// satisfies it; tests inject fakes.
type configStore interface {
	GetSummary(ctx context.Context) (GatewaySummary, error)
	Update(ctx context.Context, input UpdateInput, actorID int64) error
	ListModelMappings(ctx context.Context) (map[string]string, error)
	SaveModelMapping(ctx context.Context, feature, model string, actorID int64) error
	ListCatalog(ctx context.Context) ([]CatalogItem, error)
	SaveCatalog(ctx context.Context, items []CatalogItem, actorID int64) error
	DetectModels(ctx context.Context, provider string) (ModelDetection, error)
}

// Handler serves the Admin AI gateway settings endpoints.
type Handler struct {
	store configStore
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
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	summary, err := h.store.GetSummary(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取网关配置失败", err)
		return
	}
	response.SuccessDTO(c, summary)
}

// Update rotates the gateway token and identity. A missing token keeps the
// stored secret; base URL or account and gateway IDs are required.
func (h *Handler) Update(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	var req updateGatewayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	identity, ok := principal.RequireAdmin(c)
	if !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
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

type modelMappingResponse struct {
	Feature string `json:"feature"`
	Model   string `json:"model"`
}

// ListMappings returns every configured per-feature model mapping.
func (h *Handler) ListMappings(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	mappings, err := h.store.ListModelMappings(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取模型映射失败", err)
		return
	}
	items := make([]modelMappingResponse, 0, len(mappings))
	for feature, model := range mappings {
		items = append(items, modelMappingResponse{Feature: feature, Model: model})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Feature < items[j].Feature })
	response.SuccessDTO(c, items)
}

type saveModelMappingRequest struct {
	Model string `json:"model"`
}

// SaveMapping validates and persists one feature's model mapping.
// Pricing lives in Billing and never reads this mapping: changing models
// cannot move prices by construction.
func (h *Handler) SaveMapping(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	feature := strings.TrimSpace(c.Param("feature"))
	var req saveModelMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	identity, ok := principal.RequireAdmin(c)
	if !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		return
	}
	if err := h.store.SaveModelMapping(c.Request.Context(), feature, req.Model, identity.UserID()); err != nil {
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
func (h *Handler) PreviewGatewayRoute(c *gin.Context) {
	var req previewGatewayRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if _, ok := principal.RequireAdmin(c); !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		return
	}
	if strings.TrimSpace(req.ModelID) == "" {
		response.Error(c, http.StatusBadRequest, "模型标识不能为空")
		return
	}
	preview := PreviewGatewayRoute(req.ModelID, req.CustomPath, req.CustomParams)
	response.SuccessDTO(c, preview)
}

type testGatewayConnectionRequest struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// TestConnection probes gateway reachability server-side (browsers cannot
// read cross-origin status). The token is used for this probe only and never
// persisted. Pure diagnosis: no configuration is read or written.
func (h *Handler) TestConnection(c *gin.Context) {
	var req testGatewayConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if _, ok := principal.RequireAdmin(c); !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		return
	}
	result := ProbeGatewayConnection(c.Request.Context(), req.BaseURL, req.Token)
	response.SuccessDTO(c, result)
}

// ListCatalog returns every model item in the AI gateway catalog.
func (h *Handler) ListCatalog(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	catalog, err := h.store.ListCatalog(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取模型库失败", err)
		return
	}
	response.SuccessDTO(c, catalog)
}

type saveCatalogRequest struct {
	Items []CatalogItem `json:"items"`
}

// SaveCatalog updates the full model catalog in system settings.
func (h *Handler) SaveCatalog(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	var req saveCatalogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	identity, ok := principal.RequireAdmin(c)
	if !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
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
// metadata read; admin-gated like every other gateway surface.
func (h *Handler) ListProviders(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		return
	}
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
func (h *Handler) DetectModels(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "网关配置服务暂不可用", nil)
		return
	}
	if _, ok := principal.RequireAdmin(c); !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
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
