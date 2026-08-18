package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type SocialProviderAdminService interface {
	GetSocialProviderSettings(context.Context, string) (*SocialProviderSettingsSummary, error)
	UpdateSocialProviderSettings(context.Context, string, SocialProviderSettingsUpdate) (*SocialProviderSettingsSummary, error)
	GetSocialProviderCapabilities(context.Context) (*SocialProviderCapabilities, error)
}

type SocialProviderAdminHandler struct {
	service SocialProviderAdminService
}

func NewSocialProviderAdminHandler(service SocialProviderAdminService) *SocialProviderAdminHandler {
	return &SocialProviderAdminHandler{service: service}
}

func (h *SocialProviderAdminHandler) Get(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if !h.requireService(c) {
		return
	}
	provider, ok := h.requireSupportedSocialProvider(c)
	if !ok {
		return
	}
	summary, err := h.service.GetSocialProviderSettings(c.Request.Context(), provider)
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, summary)
}

func (h *SocialProviderAdminHandler) Update(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if !h.requireService(c) {
		return
	}
	provider, ok := h.requireSupportedSocialProvider(c)
	if !ok {
		return
	}
	var request struct {
		Enabled      bool   `json:"enabled"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	summary, err := h.service.UpdateSocialProviderSettings(c.Request.Context(), provider, SocialProviderSettingsUpdate{
		Enabled: request.Enabled, ClientID: request.ClientID, ClientSecret: request.ClientSecret,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, summary)
}

func (h *SocialProviderAdminHandler) requireService(c *gin.Context) bool {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusInternalServerError, "OAuth Provider 配置服务未初始化", errors.New("social provider admin service is not initialized"))
		return false
	}
	return true
}

func (h *SocialProviderAdminHandler) respondError(c *gin.Context, err error) {
	var providerErr *SocialProviderSettingsError
	if errors.As(err, &providerErr) {
		switch providerErr.Status {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
			response.Error(c, providerErr.Status, providerErr.Message)
		default:
			response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
		}
		return
	}
	response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
}

func (h *SocialProviderAdminHandler) requireSupportedSocialProvider(c *gin.Context) (string, bool) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if provider == "" || len(provider) > 64 {
		response.Error(c, http.StatusNotFound, "不支持的 OAuth Provider")
		return "", false
	}
	for _, character := range provider {
		if (character < 'a' || character > 'z') && character != '-' && character != '_' && (character < '0' || character > '9') {
			response.Error(c, http.StatusNotFound, "不支持的 OAuth Provider")
			return "", false
		}
	}
	capabilities, err := h.service.GetSocialProviderCapabilities(c.Request.Context())
	if err != nil {
		h.respondError(c, err)
		return "", false
	}
	if capabilities == nil {
		response.Error(c, http.StatusNotFound, "不支持的 OAuth Provider")
		return "", false
	}
	if len(capabilities.Providers) > 0 {
		for _, candidate := range capabilities.Providers {
			if candidate.Provider == provider {
				return provider, true
			}
		}
	} else if hasCapabilityKey(capabilities.Social, provider) {
		return provider, true
	}
	response.Error(c, http.StatusNotFound, "不支持的 OAuth Provider")
	return "", false
}

func hasCapabilityKey(capabilities map[string]bool, provider string) bool {
	_, exists := capabilities[provider]
	return exists
}
