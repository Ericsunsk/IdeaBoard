package aigateway

import (
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// SetObservedService attaches the read-only model observation service. It is
// set once at composition time (same pattern as vision billing); a nil
// service keeps the endpoint fail-closed with 503.
func (h *Handler) SetObservedService(svc *ObservedService) {
	if h == nil {
		return
	}
	h.observed = svc
}

// ListModelsObserved serves the admin read-only 7-day first-party model
// observations: per-model success rate, top failure stage/category, cache hit
// rate, plus on-sale SKU UnitCredits margin rows. Gateway actual cost has no
// receipt/log source, so every margin row carries an explicit unavailable
// marker instead of a fabricated number. Models absent from the payload have
// neither attempts nor mappings in the window; callers render 暂无数据.
func (h *Handler) ListModelsObserved(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		return
	}
	if h == nil || h.observed == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "模型观测服务暂不可用", nil)
		return
	}
	rows, err := h.observed.ListModelsObserved(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取模型观测数据失败", err)
		return
	}
	response.SuccessDTO(c, rows)
}
