package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"

	"github.com/gin-gonic/gin"
)

// providerConfigStore is the narrow settings seam the admin handler needs.
// The settings-backed repository satisfies it structurally.
type providerConfigStore interface {
	List(context.Context) ([]domain.ProviderConfig, error)
	Save(context.Context, domain.Provider, *string, bool, int64) error
	RecordTest(context.Context, domain.Provider, domain.TestState, domain.FailureCategory, string, int64) error
	RawAPIKey(context.Context, domain.Provider) (string, error)
}

type Handler struct {
	repository providerConfigStore
	adapters   map[domain.Provider]ports.ProviderTestAdapter
}

type ProviderListResponse struct {
	Providers []domain.ProviderConfig `json:"providers"`
}

type UpdateProviderRequest struct {
	APIKey      *string `json:"api_key"`
	EndpointURL string  `json:"endpoint_url"`
	Model       string  `json:"model"`
	Enabled     bool    `json:"enabled"`
}

type TestProviderResponse struct {
	Provider        domain.Provider        `json:"provider"`
	TestState       domain.TestState       `json:"test_state"`
	FailureCategory domain.FailureCategory `json:"failure_category,omitempty"`
	Message         string                 `json:"message"`
}

func NewHandler(repository providerConfigStore, adapters ...ports.ProviderTestAdapter) *Handler {
	byProvider := make(map[domain.Provider]ports.ProviderTestAdapter, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			byProvider[adapter.Provider()] = adapter
		}
	}
	return &Handler{repository: repository, adapters: byProvider}
}

func (h *Handler) List(c *gin.Context) {
	if h == nil || h.repository == nil {
		response.MachineError(c, http.StatusInternalServerError, "search_provider_unavailable", "搜索供应商配置不可用")
		return
	}
	configs, err := h.repository.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取搜索供应商配置失败", err)
		return
	}
	response.SuccessDTO(c, ProviderListResponse{Providers: configs})
}

func (h *Handler) Update(c *gin.Context) {
	// Authorization rides the outer Control Plane guard (controlProxy +
	// guard); the actor is resolved from the request principal.
	admin, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.repository == nil {
		response.MachineError(c, http.StatusServiceUnavailable, "search_provider_unavailable", "搜索供应商配置不可用")
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		response.MachineError(c, http.StatusNotFound, "search_provider_not_found", "搜索供应商不存在")
		return
	}
	var request UpdateProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.MachineError(c, http.StatusBadRequest, "search_request_invalid", "参数错误")
		return
	}
	saveErr := h.repository.Save(c.Request.Context(), provider, request.APIKey, request.Enabled, admin.UserID())
	if saveErr != nil {
		if errors.Is(saveErr, domain.ErrProviderTestRequired) {
			response.MachineError(c, http.StatusConflict, "search_provider_test_required", "启用前请先通过连接测试")
			return
		}
		if strings.Contains(saveErr.Error(), "api key cannot be empty") {
			response.MachineError(c, http.StatusBadRequest, "search_provider_api_key_invalid", "API Key 不能为空")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "保存搜索供应商配置失败", saveErr)
		return
	}
	response.Success(c, nil)
}

func (h *Handler) Test(c *gin.Context) {
	// Authorization rides the outer Control Plane guard (controlProxy +
	// guard); the actor is resolved from the request principal.
	admin, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.repository == nil {
		response.MachineError(c, http.StatusServiceUnavailable, "search_provider_unavailable", "搜索供应商配置不可用")
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		response.MachineError(c, http.StatusNotFound, "search_provider_not_found", "搜索供应商不存在")
		return
	}
	adapter := h.adapters[provider]
	if adapter == nil {
		response.MachineError(c, http.StatusInternalServerError, "search_provider_unavailable", "搜索供应商测试不可用")
		return
	}
	key, err := h.repository.RawAPIKey(c.Request.Context(), provider)
	if err != nil {
		response.MachineError(c, http.StatusConflict, "search_provider_api_key_required", "请先保存 API Key")
		return
	}
	category, message, testErr := adapter.Test(c.Request.Context(), key)
	state := domain.TestStatePassed
	if testErr != nil {
		state = domain.TestStateFailed
	}
	if err := h.repository.RecordTest(c.Request.Context(), provider, state, category, message, admin.UserID()); err != nil {
		response.InternalError(c, http.StatusInternalServerError, "保存搜索供应商测试状态失败", err)
		return
	}
	if testErr != nil {
		response.MachineError(c, http.StatusBadRequest, "search_provider_test_failed", "搜索供应商连接测试失败")
		return
	}
	response.SuccessDTO(c, TestProviderResponse{Provider: provider, TestState: state, Message: message})
}

func parseProvider(raw string) (domain.Provider, bool) {
	provider := domain.Provider(strings.ToLower(strings.TrimSpace(raw)))
	return provider, domain.IsProvider(provider)
}
