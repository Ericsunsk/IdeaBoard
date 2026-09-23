package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/idempotency"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/searchcatalog/application"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

type VisionHandler struct {
	service     *application.VisionSearchService
	inputReader VisionInputReader
	billing     *application.VisionBillingService
}

func NewVisionHandler(service *application.VisionSearchService, readers ...VisionInputReader) *VisionHandler {
	reader := VisionInputReader(newMultipartVisionInputReader(nil))
	if len(readers) > 0 && readers[0] != nil {
		reader = readers[0]
	}
	return &VisionHandler{service: service, inputReader: reader}
}

func (h *VisionHandler) SetBillingService(service *application.VisionBillingService) {
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
		var pending *application.VisionBillingPending
		if errors.As(err, &pending) || errors.Is(err, application.ErrVisionSettlement) {
			response.AcceptedDTO(c, result)
			return
		}
		var operationFailure *application.VisionOperationFailure
		if errors.As(err, &operationFailure) {
			response.MachineError(c, operationFailure.HTTPStatus, operationFailure.Code, operationFailure.Message)
			return
		}
		if errors.Is(err, application.ErrVisionIdempotencyKeyRequired) {
			if strings.TrimSpace(c.GetHeader(idempotency.Header)) == "" {
				response.MachineError(c, http.StatusBadRequest, string(idempotency.CodeRequired), idempotency.MissingError().Error())
			} else {
				response.MachineError(c, http.StatusBadRequest, string(idempotency.CodeRequired), idempotency.InvalidError().Error())
			}
			return
		}
		if errors.Is(err, application.ErrVisionIdempotencyConflict) {
			response.MachineError(c, http.StatusConflict, string(idempotency.CodeConflict), idempotency.ConflictError().Error())
			return
		}
		if errors.Is(err, application.ErrVisionPricingUnavailable) {
			response.MachineError(c, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
			return
		}
		if errors.Is(err, domain.ErrWalletFrozen) || errors.Is(err, domain.ErrWalletRestricted) {
			response.MachineError(c, http.StatusConflict, "wallet_restricted", "账户受限，暂时无法使用 Idea")
			return
		}
		if errors.Is(err, domain.ErrInsufficientCredits) {
			response.MachineError(c, http.StatusPaymentRequired, "insufficient_credits", "Idea不足")
			return
		}
		writeVisionError(c, err)
		return
	}
	if result.Status != "succeeded" {
		response.AcceptedDTO(c, result)
		return
	}
	response.SuccessDTO(c, result)
}

func writeVisionError(c *gin.Context, err error) {
	status, code, message := http.StatusBadGateway, "vision_search_provider_error", "视觉搜索暂时不可用"
	switch {
	case errors.Is(err, searchdomain.ErrVisionInput):
		status, code, message = http.StatusBadRequest, "vision_search_input_invalid", "请添加 1–4 张参考图"
	case errors.Is(err, searchdomain.ErrVisionImageInvalid):
		status, code, message = http.StatusBadRequest, "vision_search_image_invalid", "参考图格式、尺寸或总大小无效"
	case errors.Is(err, searchdomain.ErrVisionImageUnavailable):
		status, code, message = http.StatusBadRequest, "vision_search_image_unavailable", "参考图暂时无法读取"
	case errors.Is(err, searchdomain.ErrVisionNotConfigured):
		status, code, message = http.StatusServiceUnavailable, "vision_search_not_configured", "视觉搜索暂未配置"
	case errors.Is(err, searchdomain.ErrVisionUnavailable):
		status, code, message = http.StatusServiceUnavailable, "vision_search_unavailable", "视觉搜索暂不可用"
	case errors.Is(err, searchdomain.ErrVisionSchema):
		status, code, message = http.StatusBadGateway, "vision_search_schema_error", "视觉搜索返回格式无效"
	default:
		var providerErr *searchdomain.VisionProviderError
		if errors.As(err, &providerErr) {
			switch providerErr.Kind {
			case searchdomain.VisionProviderFailureCredentials:
				status, code, message = http.StatusBadGateway, "vision_search_credentials", "视觉搜索凭证无效"
			case searchdomain.VisionProviderFailureQuota:
				status, code, message = http.StatusBadGateway, "vision_search_quota", "视觉搜索权限或配额不可用"
			case searchdomain.VisionProviderFailureTimeout:
				status, code, message = http.StatusGatewayTimeout, "vision_search_timeout", "视觉搜索超时，请重试"
			case searchdomain.VisionProviderFailureNetwork:
				status, code, message = http.StatusBadGateway, "vision_search_network", "视觉搜索连接失败，请重试"
			}
		}
	}
	response.MachineError(c, status, code, message)
}
