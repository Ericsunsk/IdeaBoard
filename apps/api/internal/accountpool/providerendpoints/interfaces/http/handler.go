package http

import (
	"errors"
	"strconv"

	"ideaboard/api/internal/accountpool/providerendpoints/application"
	"ideaboard/api/internal/accountpool/providerendpoints/domain"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type CreateEndpointRequest struct {
	APIType    string `json:"api_type" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	Capability string `json:"capability"`
	Name       string `json:"name" binding:"required"`
	BaseURL    string `json:"base_url" binding:"required"`
	APIKey     string `json:"api_key"`
	Status     int16  `json:"status"`
}

type UpdateEndpointRequest struct {
	APIType    string `json:"api_type" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	Capability string `json:"capability"`
	Name       string `json:"name" binding:"required"`
	BaseURL    string `json:"base_url" binding:"required"`
	APIKey     string `json:"api_key"`
	Status     int16  `json:"status"`
}

type UpdateEndpointStatusRequest struct {
	Status *int16 `json:"status"`
}

type DetectModelsRequest struct {
	BaseURL  string `json:"base_url" binding:"required"`
	APIKey   string `json:"api_key" binding:"required"`
	APIType  string `json:"api_type"`
	Protocol string `json:"protocol"`
}

type providerEndpointListResponse struct {
	List []*domain.Endpoint `json:"list"`
}

type providerEndpointRegistryResponse struct {
	Protocols []domain.ProtocolDefinition `json:"protocols"`
}

type providerEndpointResponse struct {
	ProviderEndpoint *domain.Endpoint `json:"provider_endpoint"`
}

type DetectedModelsResponse struct {
	Models  []string `json:"models"`
	APIType string   `json:"api_type"`
}

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	endpoints, err := h.service.List(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取 ProviderEndpoint 列表失败")
		return
	}
	response.SuccessDTO(c, providerEndpointListResponse{List: endpoints})
}

func (h *Handler) Registry(c *gin.Context) {
	response.SuccessDTO(c, providerEndpointRegistryResponse{Protocols: domain.ProtocolRegistry()})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	endpoint, err := h.service.Create(c.Request.Context(), application.CreateEndpointCommand{
		APIType:    req.APIType,
		Protocol:   req.Protocol,
		Capability: req.Capability,
		Name:       req.Name,
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		Status:     req.Status,
	})
	if err != nil {
		if RespondProviderEndpointConflict(c, err) {
			return
		}
		if respondValidationError(c, err) {
			return
		}
		response.InternalError(c, 500, "创建 ProviderEndpoint 失败", err)
		return
	}

	response.SuccessDTO(c, providerEndpointResponse{ProviderEndpoint: endpoint})
}

func (h *Handler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	var req UpdateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	endpoint, err := h.service.Update(c.Request.Context(), id, application.UpdateEndpointCommand{
		APIType:    req.APIType,
		Protocol:   req.Protocol,
		Capability: req.Capability,
		Name:       req.Name,
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		Status:     req.Status,
	})
	if err != nil {
		if database.IsNotFound(err) || errors.Is(err, domain.ErrNotFound) {
			response.Error(c, 404, "ProviderEndpoint 不存在")
			return
		}
		if RespondProviderEndpointConflict(c, err) {
			return
		}
		if respondValidationError(c, err) {
			return
		}
		response.InternalError(c, 500, "更新 ProviderEndpoint 失败", err)
		return
	}

	response.SuccessDTO(c, providerEndpointResponse{ProviderEndpoint: endpoint})
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	var req UpdateEndpointStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Status == nil {
		response.Error(c, 400, "参数错误")
		return
	}

	endpoint, err := h.service.UpdateStatus(c.Request.Context(), id, *req.Status)
	if err != nil {
		if database.IsNotFound(err) || errors.Is(err, domain.ErrNotFound) {
			response.Error(c, 404, "ProviderEndpoint 不存在")
			return
		}
		if errors.Is(err, domain.ErrInvalidStatus) {
			response.Error(c, 400, "status 只支持 0 或 1")
			return
		}
		response.InternalError(c, 500, "更新 ProviderEndpoint 状态失败", err)
		return
	}

	response.SuccessDTO(c, providerEndpointResponse{ProviderEndpoint: endpoint})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if database.IsNotFound(err) || errors.Is(err, domain.ErrNotFound) {
			response.Error(c, 404, "ProviderEndpoint 不存在")
			return
		}
		response.InternalError(c, 500, "删除 ProviderEndpoint 失败", err)
		return
	}

	response.Success(c, nil)
}

func (h *Handler) DetectModels(c *gin.Context) {
	var req DetectModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	models, apiType, err := h.service.DetectModels(c.Request.Context(), application.DetectModelsCommand{
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		APIType:  req.APIType,
		Protocol: req.Protocol,
	})
	if err != nil {
		var detectionErr *application.ModelDetectionError
		if errors.As(err, &detectionErr) {
			response.Error(c, 400, detectionErr.PublicMessage)
			return
		}
		response.Error(c, 400, err.Error())
		return
	}

	response.SuccessDTO(c, DetectedModelsResponse{Models: models, APIType: apiType})
}

func RespondProviderEndpointConflict(c *gin.Context, err error) bool {
	if errors.Is(err, domain.ErrNameConflict) {
		response.Error(c, 409, "ProviderEndpoint 名称已存在，请使用其他名称")
		return true
	}
	if errors.Is(err, domain.ErrURLConflict) {
		response.Error(c, 409, "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint")
		return true
	}
	return false
}

func respondValidationError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, domain.ErrInvalidStatus):
		response.Error(c, 400, "status 只支持 0 或 1")
		return true
	case errors.Is(err, domain.ErrAPIKeyRequired):
		response.Error(c, 400, domain.ErrAPIKeyRequired.Error())
		return true
	case errors.Is(err, domain.ErrUnsupportedProtocol):
		response.Error(c, 400, domain.ErrUnsupportedProtocol.Error())
		return true
	case errors.Is(err, domain.ErrImageAPIRequiresFunc):
		response.Error(c, 400, domain.ErrImageAPIRequiresFunc.Error())
		return true
	case errors.Is(err, domain.ErrInvalidBaseURL):
		response.Error(c, 400, domain.ErrInvalidBaseURL.Error())
		return true
	case errors.Is(err, domain.ErrUnsupportedAPIType):
		response.Error(c, 400, domain.ErrUnsupportedAPIType.Error())
		return true
	default:
		return false
	}
}
