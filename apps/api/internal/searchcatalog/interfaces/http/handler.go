package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"
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
	if !platformhttp.RequireDependencyCode(c, h != nil && h.repository != nil, string(contract.CodeSearchProviderUnavailable), errorrender.MsgSearchProviderUnavailable) {
		return
	}
	configs, err := h.repository.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgSearchcatalogInterfacesHTTPList, err)
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
	if !platformhttp.RequireDependencyCode(c, h != nil && h.repository != nil, string(contract.CodeSearchProviderUnavailable), errorrender.MsgSearchProviderUnavailable) {
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeSearchProviderNotFound), errorrender.MsgSearchProviderNotFound)
		return
	}
	var request UpdateProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchRequestInvalid), errorrender.MsgSearchRequestInvalid)
		return
	}
	saveErr := h.repository.Save(c.Request.Context(), provider, request.APIKey, request.Enabled, admin.UserID())
	if saveErr != nil {
		if errors.Is(saveErr, domain.ErrProviderTestRequired) {
			errorrender.Render(c, http.StatusConflict, string(contract.CodeSearchProviderTestRequired), errorrender.MsgSearchProviderTestRequired)
			return
		}
		if strings.Contains(saveErr.Error(), "api key cannot be empty") {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchProviderAPIKeyInvalid), errorrender.MsgSearchProviderAPIKeyInvalid)
			return
		}
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgSearchcatalogInterfacesHTTPUpdate, saveErr)
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
	if !platformhttp.RequireDependencyCode(c, h != nil && h.repository != nil, string(contract.CodeSearchProviderUnavailable), errorrender.MsgSearchProviderUnavailable) {
		return
	}
	provider, ok := parseProvider(c.Param("provider"))
	if !ok {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeSearchProviderNotFound), errorrender.MsgSearchProviderNotFound)
		return
	}
	adapter := h.adapters[provider]
	if !platformhttp.RequireDependencyCode(c, adapter != nil, string(contract.CodeSearchProviderUnavailable), errorrender.MsgSearchProviderUnavailable2) {
		return
	}
	key, err := h.repository.RawAPIKey(c.Request.Context(), provider)
	if err != nil {
		errorrender.Render(c, http.StatusConflict, string(contract.CodeSearchProviderAPIKeyRequired), errorrender.MsgSearchProviderAPIKeyRequired)
		return
	}
	category, message, testErr := adapter.Test(c.Request.Context(), key)
	state := domain.TestStatePassed
	if testErr != nil {
		state = domain.TestStateFailed
	}
	if err := h.repository.RecordTest(c.Request.Context(), provider, state, category, message, admin.UserID()); err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgSearchcatalogInterfacesHTTPTest, err)
		return
	}
	if testErr != nil {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchProviderTestFailed), errorrender.MsgSearchProviderTestFailed)
		return
	}
	response.SuccessDTO(c, TestProviderResponse{Provider: provider, TestState: state, Message: message})
}

func parseProvider(raw string) (domain.Provider, bool) {
	provider := domain.Provider(strings.ToLower(strings.TrimSpace(raw)))
	return provider, domain.IsProvider(provider)
}
