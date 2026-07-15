package usage

import (
	"context"
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Reader interface {
	GetCurrent(context.Context, int64, Range) (Usage, error)
}

type Handler struct {
	service Reader
}

func NewHandler(service Reader) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetCurrent(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "用量统计服务暂不可用", nil)
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	usageRange, err := ParseRange(c.Query("range"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "range 仅支持 7d、30d 或 1y")
		return
	}
	result, err := h.service.GetCurrent(c.Request.Context(), identityPrincipal.UserID(), usageRange)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "加载用量统计失败", err)
		return
	}
	if err := ValidateUsage(result, usageRange); err != nil {
		response.InternalError(c, http.StatusInternalServerError, "加载用量统计失败", err)
		return
	}
	response.SuccessDTO(c, result)
}
