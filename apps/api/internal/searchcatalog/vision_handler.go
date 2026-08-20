package searchcatalog

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type VisionHandler struct {
	service     *VisionSearchService
	inputReader VisionInputReader
}

func NewVisionHandler(service *VisionSearchService, readers ...VisionInputReader) *VisionHandler {
	reader := VisionInputReader(newMultipartVisionInputReader(nil))
	if len(readers) > 0 && readers[0] != nil {
		reader = readers[0]
	}
	return &VisionHandler{service: service, inputReader: reader}
}

func (h *VisionHandler) Extract(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.MachineError(c, http.StatusServiceUnavailable, "vision_search_unavailable", "视觉搜索暂不可用")
		return
	}
	if h.inputReader == nil {
		response.MachineError(c, http.StatusServiceUnavailable, "vision_search_unavailable", "视觉搜索暂不可用")
		return
	}
	input, err := h.inputReader.Read(c.Request, identity.UserID())
	if err != nil {
		writeVisionError(c, err)
		return
	}
	extraction, err := h.service.Extract(c.Request.Context(), input)
	if err != nil {
		writeVisionError(c, err)
		return
	}
	response.SuccessDTO(c, extraction)
}

func writeVisionError(c *gin.Context, err error) {
	status, code, message := http.StatusBadGateway, "vision_search_provider_error", "视觉搜索暂时不可用"
	switch {
	case errors.Is(err, ErrVisionInput):
		status, code, message = http.StatusBadRequest, "vision_search_input_invalid", "请添加 1–4 张参考图"
	case errors.Is(err, ErrVisionImageInvalid):
		status, code, message = http.StatusBadRequest, "vision_search_image_invalid", "参考图格式、尺寸或总大小无效"
	case errors.Is(err, ErrVisionImageUnavailable):
		status, code, message = http.StatusBadRequest, "vision_search_image_unavailable", "参考图暂时无法读取"
	case errors.Is(err, ErrVisionNotConfigured):
		status, code, message = http.StatusServiceUnavailable, "vision_search_not_configured", "视觉搜索暂未配置"
	case errors.Is(err, ErrVisionUnavailable):
		status, code, message = http.StatusServiceUnavailable, "vision_search_unavailable", "视觉搜索暂不可用"
	case errors.Is(err, ErrVisionSchema):
		status, code, message = http.StatusBadGateway, "vision_search_schema_error", "视觉搜索返回格式无效"
	default:
		var providerErr *visionProviderError
		if errors.As(err, &providerErr) && providerErr.kind == visionProviderFailureTimeout {
			status, code, message = http.StatusGatewayTimeout, "vision_search_timeout", "视觉搜索超时，请重试"
		}
	}
	response.MachineError(c, status, code, message)
}
