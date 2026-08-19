package searchcatalog

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type MetricsHandler struct {
	metrics SearchMetrics
}

type ImageFailureRequest struct {
	Provider Provider `json:"provider" binding:"required"`
}

func NewMetricsHandler(metrics SearchMetrics) *MetricsHandler {
	return &MetricsHandler{metrics: metrics}
}

func (h *MetricsHandler) RecordImageFailure(c *gin.Context) {
	if _, ok := principal.Require(c); !ok {
		return
	}
	if h == nil || h.metrics == nil {
		response.Error(c, http.StatusServiceUnavailable, "搜索观测暂不可用")
		return
	}
	var request ImageFailureRequest
	if err := c.ShouldBindJSON(&request); err != nil || !isProvider(Provider(strings.ToLower(strings.TrimSpace(string(request.Provider))))) {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	h.metrics.RecordImageFailure(Provider(strings.ToLower(strings.TrimSpace(string(request.Provider)))))
	response.Success(c, nil)
}
