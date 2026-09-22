package searchcatalog

import (
	"errors"
	"ideaboard/api/internal/credits/domain"
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type VisionHandler struct {
	service     *VisionSearchService
	inputReader VisionInputReader
	billing     *VisionBillingService
}

func NewVisionHandler(service *VisionSearchService, readers ...VisionInputReader) *VisionHandler {
	reader := VisionInputReader(newMultipartVisionInputReader(nil))
	if len(readers) > 0 && readers[0] != nil {
		reader = readers[0]
	}
	return &VisionHandler{service: service, inputReader: reader}
}

func (h *VisionHandler) SetBillingService(service *VisionBillingService) {
	if h != nil {
		h.billing = service
	}
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
	if h.billing == nil {
		response.MachineError(c, http.StatusServiceUnavailable, "vision_search_unavailable", "视觉搜索暂不可用")
		return
	}
	input, err := h.inputReader.Read(c.Request, identity.UserID())
	if err != nil {
		writeVisionError(c, err)
		return
	}
	result, err := h.billing.Extract(c.Request.Context(), identity.UserID(), c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		var pending *visionBillingPending
		if errors.As(err, &pending) || errors.Is(err, ErrVisionSettlement) {
			c.JSON(http.StatusAccepted, response.Response{Code: 0, Msg: "accepted", RequestID: c.GetString(response.RequestIDHeader), Data: result})
			return
		}
		var operationFailure *visionOperationFailure
		if errors.As(err, &operationFailure) {
			response.MachineError(c, operationFailure.HTTPStatus, operationFailure.Code, operationFailure.Message)
			return
		}
		if errors.Is(err, ErrVisionIdempotencyKeyRequired) {
			response.MachineError(c, http.StatusBadRequest, "vision_search_idempotency_key_required", "请重试此次搜索")
			return
		}
		if errors.Is(err, ErrVisionIdempotencyConflict) {
			response.MachineError(c, http.StatusConflict, "vision_search_idempotency_conflict", "此搜索请求已绑定其他输入")
			return
		}
		if errors.Is(err, ErrVisionPricingUnavailable) {
			response.MachineError(c, http.StatusPaymentRequired, "vision_search_pricing_unavailable", "视觉搜索暂未开放")
			return
		}
		if errors.Is(err, domain.ErrInsufficientCredits) {
			response.MachineError(c, http.StatusPaymentRequired, "insufficient_credits", "Ideas 余额不足")
			return
		}
		writeVisionError(c, err)
		return
	}
	if result.Status != "succeeded" {
		c.JSON(http.StatusAccepted, response.Response{Code: 0, Msg: "accepted", RequestID: c.GetString(response.RequestIDHeader), Data: result})
		return
	}
	response.SuccessDTO(c, result)
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
		if errors.As(err, &providerErr) {
			switch providerErr.kind {
			case visionProviderFailureCredentials:
				status, code, message = http.StatusBadGateway, "vision_search_credentials", "视觉搜索凭证无效"
			case visionProviderFailureQuota:
				status, code, message = http.StatusBadGateway, "vision_search_quota", "视觉搜索权限或配额不可用"
			case visionProviderFailureTimeout:
				status, code, message = http.StatusGatewayTimeout, "vision_search_timeout", "视觉搜索超时，请重试"
			case visionProviderFailureNetwork:
				status, code, message = http.StatusBadGateway, "vision_search_network", "视觉搜索连接失败，请重试"
			}
		}
	}
	response.MachineError(c, status, code, message)
}
