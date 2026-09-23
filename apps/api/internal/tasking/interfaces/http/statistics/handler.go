package statisticshttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/platform/validation"
	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/gin-gonic/gin"
)

const (
	monitorStreamInterval          = 10 * time.Second
	monitorStreamMaxConnPeriod     = 30 * time.Minute
	monitorSnapshotEvent           = "snapshot"
	monitorSnapshotErrorEvent      = "snapshot_error"
	monitorStreamCloseEvent        = "close"
	monitorStreamCloseMaxConnCause = "max_connection_duration_reached"
	monitorStreamMaxConcurrent     = 20
)

type MonitorInfoProvider interface {
	GetMonitorInfo(ctx context.Context) (*tasks.MonitorInfo, error)
}

type Handler struct {
	service            *statisticsapp.Service
	monitor            MonitorInfoProvider
	monitorStreamCount atomic.Int32
}

type adminMonitorSnapshot struct {
	Monitor *adminMonitorData `json:"monitor"`
	Stats   map[string]int64  `json:"stats"`
}

type adminMonitorData struct {
	GlobalQueueLen    int64                                         `json:"global_queue_len"`
	WaitQueueLen      int64                                         `json:"wait_queue_len"`
	RunningAttempts   int64                                         `json:"running_attempts"`
	ExecutionPressure []statisticsapp.MonitorExecutionPressurePoint `json:"execution_pressure"`
	GoldenSignals     statisticsapp.MonitorGoldenSignals            `json:"golden_signals"`
}

func NewHandler(service *statisticsapp.Service, monitor MonitorInfoProvider) *Handler {
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
	if h == nil {
		return
	}
	if count := h.monitorStreamCount.Add(1); count > monitorStreamMaxConcurrent {
		h.monitorStreamCount.Add(-1)
		response.MachineError(c, http.StatusTooManyRequests, "task_monitor_concurrency_limited", "监控连接数已达上限，请稍后重试")
		return
	}
	defer h.monitorStreamCount.Add(-1)

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
	pressure, err := h.service.GetMonitorExecutionPressure(ctx)
	if err != nil {
		return nil, fmt.Errorf("get monitor execution pressure: %w", err)
	}
	goldenSignals, err := h.service.GetMonitorGoldenSignals(ctx)
	if err != nil {
		return nil, fmt.Errorf("get monitor golden signals: %w", err)
	}
	return &adminMonitorSnapshot{Monitor: adminMonitorDataFromRuntime(info, pressure, goldenSignals), Stats: stats}, nil
}

func adminMonitorDataFromRuntime(info *tasks.MonitorInfo, pressure []statisticsapp.MonitorExecutionPressurePoint, goldenSignals statisticsapp.MonitorGoldenSignals) *adminMonitorData {
	if pressure == nil {
		pressure = []statisticsapp.MonitorExecutionPressurePoint{}
	}
	if info == nil {
		return &adminMonitorData{
			ExecutionPressure: pressure,
			GoldenSignals:     goldenSignals,
		}
	}
	return &adminMonitorData{
		GlobalQueueLen:    info.GlobalQueueLen,
		WaitQueueLen:      info.WaitQueueLen,
		RunningAttempts:   info.RunningAttempts,
		ExecutionPressure: pressure,
		GoldenSignals:     goldenSignals,
	}
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
		if validation.Is(err) {
			response.MachineError(c, http.StatusBadRequest, "task_statistics_request_invalid", validationMessage(err))
			return
		}
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
		if !errors.Is(err, io.EOF) {
			response.MachineError(c, http.StatusBadRequest, "task_statistics_request_invalid", "请求参数错误")
			return
		}
	}
	req.Range = strings.TrimSpace(req.Range)
	if req.Range == "" {
		req.Range = string(statisticsapp.StatisticsRange7D)
	}

	result, err := h.service.BackfillStatistics(c.Request.Context(), req.Range)
	if err != nil {
		if validation.Is(err) {
			response.MachineError(c, http.StatusBadRequest, "task_statistics_request_invalid", validationMessage(err))
			return
		}
		response.InternalError(c, 500, "补录统计失败", err)
		return
	}
	response.Success(c, result)
}

func validationMessage(err error) string {
	if validation.Is(err) && err.Error() != "" {
		return err.Error()
	}
	return "参数错误"
}
