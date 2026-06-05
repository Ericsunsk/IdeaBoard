package taskstats

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/runtime/queuetypes"

	"github.com/gin-gonic/gin"
)

const (
	monitorStreamInterval          = 10 * time.Second
	monitorStreamMaxConnPeriod     = 30 * time.Minute
	monitorSnapshotEvent           = "snapshot"
	monitorSnapshotErrorEvent      = "snapshot_error"
	monitorStreamCloseEvent        = "close"
	monitorStreamCloseMaxConnCause = "max_connection_duration_reached"
)

type MonitorInfoProvider interface {
	GetMonitorInfo(ctx context.Context) (*queuetypes.MonitorInfo, error)
}

type Handler struct {
	service *Service
	monitor MonitorInfoProvider
}

type adminMonitorSnapshot struct {
	Monitor *queuetypes.MonitorInfo `json:"monitor"`
	Stats   map[string]int64        `json:"stats"`
}

func NewHandler(service *Service, monitor MonitorInfoProvider) *Handler {
	return &Handler{
		service: service,
		monitor: monitor,
	}
}

func (h *Handler) GetMonitor(c *gin.Context) {
	snapshot, err := h.buildMonitorSnapshot(c.Request.Context())
	if err != nil {
		response.InternalError(c, 500, "获取监控数据失败", err)
		return
	}
	response.Success(c, snapshot)
}

func (h *Handler) StreamMonitor(c *gin.Context) {
	ctx := c.Request.Context()
	snapshot, err := h.buildMonitorSnapshot(ctx)
	if err != nil {
		response.InternalError(c, 500, "获取监控数据失败", err)
		return
	}

	if err := sse.Prepare(c); err != nil {
		slog.Warn("clear admin monitor SSE write deadline failed", "err", err)
	}
	if !writeMonitorSnapshot(c, snapshot) {
		return
	}

	ticker := time.NewTicker(monitorStreamInterval)
	defer ticker.Stop()
	maxConnTimer := time.NewTimer(monitorStreamMaxConnPeriod)
	defer maxConnTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-maxConnTimer.C:
			_ = sse.WriteEvent(c, monitorStreamCloseEvent, gin.H{"reason": monitorStreamCloseMaxConnCause})
			return
		case <-ticker.C:
			snapshot, err := h.buildMonitorSnapshot(ctx)
			if err != nil {
				slog.Warn("admin monitor SSE snapshot failed", "err", err)
				if err := sse.WriteEvent(c, monitorSnapshotErrorEvent, response.Response{
					Code: http.StatusInternalServerError,
					Msg:  "获取监控数据失败",
				}); err != nil {
					return
				}
				continue
			}
			if !writeMonitorSnapshot(c, snapshot) {
				return
			}
		}
	}
}

func (h *Handler) buildMonitorSnapshot(ctx context.Context) (*adminMonitorSnapshot, error) {
	info, err := h.monitor.GetMonitorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get monitor info: %w", err)
	}
	stats, err := h.service.GetTaskStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get task stats: %w", err)
	}
	return &adminMonitorSnapshot{Monitor: info, Stats: stats}, nil
}

func writeMonitorSnapshot(c *gin.Context, snapshot *adminMonitorSnapshot) bool {
	return sse.WriteEvent(c, monitorSnapshotEvent, response.Response{
		Code: 0,
		Msg:  "success",
		Data: snapshot,
	}) == nil
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
