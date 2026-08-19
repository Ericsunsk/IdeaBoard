package searchcatalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/secrets"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repository *ConfigRepository
	adapters   map[Provider]providerAdapter
}

type ProviderListResponse struct {
	Providers []ProviderConfig `json:"providers"`
}

type UpdateProviderRequest struct {
	APIKey  *string `json:"api_key"`
	Enabled bool    `json:"enabled"`
}

type TestProviderResponse struct {
	Provider        Provider        `json:"provider"`
	TestState       TestState       `json:"test_state"`
	FailureCategory FailureCategory `json:"failure_category,omitempty"`
	Message         string          `json:"message"`
}

func NewHandler(repository *ConfigRepository, adapters ...providerAdapter) *Handler {
	if len(adapters) == 0 {
		adapters = newProviderAdapters()
	}
	byProvider := make(map[Provider]providerAdapter, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			byProvider[adapter.Provider()] = adapter
		}
	}
	return &Handler{repository: repository, adapters: byProvider}
}

func (h *Handler) List(c *gin.Context) {
	if h == nil || h.repository == nil {
		response.Error(c, http.StatusInternalServerError, "搜索供应商配置不可用")
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
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		response.Error(c, http.StatusNotFound, "搜索供应商不存在")
		return
	}
	var request UpdateProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if err := h.repository.Save(c.Request.Context(), provider, request.APIKey, request.Enabled, admin.UserID()); err != nil {
		if errors.Is(err, ErrProviderTestRequired) {
			response.Error(c, http.StatusConflict, "启用前请先通过连接测试")
			return
		}
		if strings.Contains(err.Error(), "api key cannot be empty") {
			response.Error(c, http.StatusBadRequest, "API Key 不能为空")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "保存搜索供应商配置失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *Handler) Test(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		response.Error(c, http.StatusNotFound, "搜索供应商不存在")
		return
	}
	key, err := h.repository.rawAPIKey(c.Request.Context(), provider)
	if err != nil {
		response.Error(c, http.StatusConflict, "请先保存 API Key")
		return
	}
	adapter := h.adapters[provider]
	if adapter == nil {
		response.Error(c, http.StatusInternalServerError, "搜索供应商测试不可用")
		return
	}
	category, message, testErr := adapter.Test(c.Request.Context(), key)
	state := TestStatePassed
	if testErr != nil {
		state = TestStateFailed
	}
	if err := h.repository.RecordTest(c.Request.Context(), provider, state, category, message, admin.UserID()); err != nil {
		response.InternalError(c, http.StatusInternalServerError, "保存搜索供应商测试状态失败", err)
		return
	}
	if testErr != nil {
		response.ErrorDTO(c, http.StatusBadRequest, TestProviderResponse{Provider: provider, TestState: state, FailureCategory: category, Message: message})
		return
	}
	response.SuccessDTO(c, TestProviderResponse{Provider: provider, TestState: state, Message: message})
}

func (r *ConfigRepository) rawAPIKey(ctx context.Context, provider Provider) (string, error) {
	stored, err := r.load(ctx, provider)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(stored.APIKeyCiphertext) == "" {
		return "", ErrProviderUnavailable
	}
	return decryptStoredAPIKey(stored)
}

func decryptStoredAPIKey(stored storedProviderConfig) (string, error) {
	if strings.TrimSpace(stored.APIKeyCiphertext) == "" {
		return "", ErrProviderUnavailable
	}
	key, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.MaterialKind)
	if err != nil {
		return "", fmt.Errorf("decrypt search provider API key: %w", err)
	}
	return key, nil
}

func parseProvider(raw string) (Provider, bool) {
	provider := Provider(strings.ToLower(strings.TrimSpace(raw)))
	return provider, isProvider(provider)
}
