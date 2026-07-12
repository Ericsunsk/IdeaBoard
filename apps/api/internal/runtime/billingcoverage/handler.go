package billingcoverage

import (
	"context"
	"errors"
	"net/http"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type CoverageService interface {
	List(context.Context) (Coverage, error)
}

type Handler struct {
	service CoverageService
}

func NewHandler(service CoverageService) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *gin.Context) {
	if handler == nil || handler.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "计费覆盖服务暂不可用", errors.New("billing coverage service unavailable"))
		return
	}
	coverage, err := handler.service.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取计费覆盖失败", err)
		return
	}
	response.SuccessDTO(c, coverage)
}
