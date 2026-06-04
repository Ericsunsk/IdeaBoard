package taskstats

import (
	"context"
	"io"
	"strings"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/runtime/queuetypes"

	"github.com/gin-gonic/gin"
)

type MonitorInfoProvider interface {
	GetMonitorInfo(ctx context.Context) (*queuetypes.MonitorInfo, error)
}

type Handler struct {
	service *Service
	monitor MonitorInfoProvider
}

func NewHandler(service *Service, monitor MonitorInfoProvider) *Handler {
	return &Handler{
		service: service,
		monitor: monitor,
	}
}

func (h *Handler) GetMonitor(c *gin.Context) {
	ctx := c.Request.Context()
	info, err := h.monitor.GetMonitorInfo(ctx)
	if err != nil {
		response.InternalError(c, 500, "获取监控数据失败", err)
		return
	}
	stats, err := h.service.GetTaskStats(ctx)
	if err != nil {
		response.InternalError(c, 500, "获取监控统计失败", err)
		return
	}
	response.Success(c, gin.H{"monitor": info, "stats": stats})
}

func (h *Handler) GetStatistics(c *gin.Context) {
	stats, err := h.service.GetStatistics(c.Request.Context(), c.Query("range"))
	if err != nil {
		response.InternalError(c, 500, "获取统计数据失败", err)
		return
	}
	response.Success(c, stats)
}

func (h *Handler) BackfillStatistics(c *gin.Context) {
	var req struct {
		Range string `json:"range"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		if err != io.EOF {
			response.Error(c, 400, "请求参数错误")
			return
		}
	}
	req.Range = strings.TrimSpace(req.Range)
	if req.Range == "" {
		req.Range = string(StatisticsRange7D)
	}

	result, err := h.service.BackfillStatistics(c.Request.Context(), req.Range)
	if err != nil {
		response.InternalError(c, 500, "补录统计失败", err)
		return
	}
	response.Success(c, result)
}
