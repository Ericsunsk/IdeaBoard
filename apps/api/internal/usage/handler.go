package usage

import (
	"context"
	"net/http"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Reader interface {
	GetCurrent(context.Context, int64) (Usage, error)
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
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
	}
	result, err := h.service.GetCurrent(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "加载用量统计失败", err)
		return
	}
	if err := ValidateUsage(result); err != nil {
		response.InternalError(c, http.StatusInternalServerError, "加载用量统计失败", err)
		return
	}
	response.SuccessDTO(c, result)
}
