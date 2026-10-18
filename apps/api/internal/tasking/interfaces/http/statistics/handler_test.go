package http

// Handler aggregation for handler.go (monitor SSE + statistics endpoints).
// Merged from handler_sse_test.go and support_test.go (shared aliases/helpers);
// the source files were orphans with no same-named test.

import (
	"bufio"
	"context"
	"errors"
	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	tasks "ideaboard/api/internal/tasking/domain"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// merged from handler_sse_test.go.
type fakeMonitorProvider struct {
	info *tasks.MonitorInfo
	err  error
}

func (p fakeMonitorProvider) GetMonitorInfo(context.Context) (*tasks.MonitorInfo, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.info, nil
}

type fakeStatsStore struct {
	counts           map[string]int64
	pressureRows     []statisticscontracts.MonitorExecutionPressureAggregate
	pressureErr      error
	goldenSignalRows []statisticscontracts.MonitorGoldenSignalSample
	goldenSignalsErr error
}

func (s fakeStatsStore) CountByStatus(_ context.Context, status string) (int64, error) {
	return s.counts[status], nil
}

func (s fakeStatsStore) CountAllTaskCalls(context.Context) (int64, error) {
	return 0, nil
}

func (s fakeStatsStore) ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListDailyCaptureCreditsByRange(context.Context, time.Time, time.Time, *time.Location) (map[string]int64, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	if s.pressureErr != nil {
		return nil, s.pressureErr
	}
	return s.pressureRows, nil
}

func (s fakeStatsStore) ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	if s.goldenSignalsErr != nil {
		return nil, s.goldenSignalsErr
	}
	return s.goldenSignalRows, nil
}

func (s fakeStatsStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	return statisticscontracts.BackfillDailyStatsResult{}, errors.New("not implemented")
}

func TestGetMonitorExecutionPressureBuildsStableBuckets(t *testing.T) {
	svc := NewService(fakeStatsStore{
		pressureRows: []statisticscontracts.MonitorExecutionPressureAggregate{
			{
				Key:                "slow_failed",
				Total:              2,
				AvgDurationSeconds: 80,
				MaxDurationSeconds: 120,
				Failed:             2,
				Retry:              1,
			},
			{
				Key:                "normal_running",
				Total:              3,
				AvgDurationSeconds: 30,
				MaxDurationSeconds: 50,
			},
		},
	})

	points, err := svc.GetMonitorExecutionPressure(context.Background())
	if err != nil {
		t.Fatalf("GetMonitorExecutionPressure failed: %v", err)
	}
	if len(points) != len(monitorExecutionPressureBuckets) {
		t.Fatalf("points=%d, want %d", len(points), len(monitorExecutionPressureBuckets))
	}

	byKey := make(map[string]MonitorExecutionPressurePoint, len(points))
	for _, point := range points {
		byKey[point.Key] = point
	}
	failed := byKey["slow_failed"]
	if failed.Count != 2 || failed.FailedCount != 2 || failed.RetryCount != 1 {
		t.Fatalf("slow_failed point mismatch: %+v", failed)
	}
	if failed.SharePercent != 40 {
		t.Fatalf("slow_failed share=%v, want 40", failed.SharePercent)
	}
	if failed.DurationPressure != 100 {
		t.Fatalf("slow_failed duration pressure=%v, want 100", failed.DurationPressure)
	}
	if failed.AnomalyPressure != 100 {
		t.Fatalf("slow_failed anomaly pressure=%v, want 100", failed.AnomalyPressure)
	}
	if byKey["normal_pending"].Count != 0 || byKey["normal_pending"].AnomalyPressure != 0 {
		t.Fatalf("empty bucket should be stable zero: %+v", byKey["normal_pending"])
	}
}

func TestGetMonitorGoldenSignalsCalculatesSuccessAndFailureSignals(t *testing.T) {
	svc := NewService(fakeStatsStore{
		goldenSignalRows: []statisticscontracts.MonitorGoldenSignalSample{
			{
				Status:            tasks.StatusCompleted,
				QueueWaitMS:       100,
				ProviderLatencyMS: 900,
				FinalizeLatencyMS: 40,
				ProviderObserved:  true,
				ProviderAttempts:  1,
			},
			{
				Status:            tasks.StatusFailed,
				FailureStage:      tasks.ExecutionStageProvider,
				FailureCategory:   tasks.FailureCategoryTimeout,
				QueueWaitMS:       300,
				ProviderLatencyMS: 1200,
				FinalizeLatencyMS: 0,
				ProviderObserved:  true,
				ProviderError:     true,
				ProviderAttempts:  1,
				ProviderErrors:    1,
			},
		},
	})

	signals, err := svc.GetMonitorGoldenSignals(context.Background())
	if err != nil {
		t.Fatalf("GetMonitorGoldenSignals failed: %v", err)
	}
	if signals.SampleCount != 2 || signals.ProviderAttempts != 2 || signals.ProviderErrorCount != 1 {
		t.Fatalf("signals counts mismatch: %+v", signals)
	}
	if signals.ProviderErrorRate != 0.5 {
		t.Fatalf("provider_error_rate=%v, want 0.5", signals.ProviderErrorRate)
	}
	if signals.QueueDelayMS.Samples != 2 || signals.QueueDelayMS.AvgMS != 200 || signals.QueueDelayMS.MaxMS != 300 {
		t.Fatalf("queue signal mismatch: %+v", signals.QueueDelayMS)
	}
	if signals.ProviderLatencyMS.Samples != 2 || signals.ProviderLatencyMS.MaxMS != 1200 {
		t.Fatalf("provider latency signal mismatch: %+v", signals.ProviderLatencyMS)
	}
	if signals.FinalizeLatencyMS.Samples != 1 || signals.FinalizeLatencyMS.AvgMS != 40 {
		t.Fatalf("finalize latency signal mismatch: %+v", signals.FinalizeLatencyMS)
	}
}

func TestAdminMonitorDataFromRuntimeNormalizesNilCollections(t *testing.T) {
	data := adminMonitorDataFromRuntime(&tasks.MonitorInfo{
		GlobalQueueLen:  4,
		WaitQueueLen:    1,
		RunningAttempts: 2,
	}, nil, MonitorGoldenSignals{SampleCount: 3})

	if data.RunningAttempts != 2 {
		t.Fatalf("running attempts=%d, want 2", data.RunningAttempts)
	}
	if data.GoldenSignals.SampleCount != 3 {
		t.Fatalf("golden signal sample count=%d, want 3", data.GoldenSignals.SampleCount)
	}
	if data.ExecutionPressure == nil {
		t.Fatal("execution pressure should be an empty array, got nil")
	}

	empty := adminMonitorDataFromRuntime(nil, nil, MonitorGoldenSignals{})
	if empty.ExecutionPressure == nil {
		t.Fatal("nil runtime info should still return an empty execution pressure array")
	}
}

func TestGetStatisticsRejectsInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewService(fakeStatsStore{}), nil)

	router := gin.New()
	router.GET("/statistics", handler.GetStatistics)

	req := httptest.NewRequest(http.MethodGet, "/statistics?range=90d", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "range 仅支持 1d") {
		t.Fatalf("body = %s, want range validation message", rec.Body.String())
	}
}

func TestBuildMonitorSnapshotFailsWhenExecutionPressureUnavailable(t *testing.T) {
	handler := NewHandler(
		NewService(fakeStatsStore{
			counts:      map[string]int64{},
			pressureErr: errors.New("pressure unavailable"),
		}),
		fakeMonitorProvider{info: &tasks.MonitorInfo{}},
	)

	_, err := handler.buildMonitorSnapshot(context.Background())
	if err == nil {
		t.Fatal("buildMonitorSnapshot succeeded, want execution pressure error")
	}
	if !strings.Contains(err.Error(), "get monitor execution pressure") {
		t.Fatalf("err=%v, want execution pressure context", err)
	}
}

func TestStreamMonitorSendsInitialSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(
		NewService(fakeStatsStore{counts: map[string]int64{
			tasks.StatusPending:   2,
			tasks.StatusRunning:   1,
			tasks.StatusCompleted: 5,
			tasks.StatusFailed:    1,
		}, pressureRows: []statisticscontracts.MonitorExecutionPressureAggregate{
			{Key: "normal_running", Total: 1, AvgDurationSeconds: 12, MaxDurationSeconds: 12},
		}}),
		fakeMonitorProvider{info: &tasks.MonitorInfo{
			GlobalQueueLen:  3,
			WaitQueueLen:    2,
			RunningAttempts: 1,
		}},
	)

	router := gin.New()
	router.GET("/stream", handler.StreamMonitor)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/stream")
	if err != nil {
		t.Fatalf("request stream failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type=%q, want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	var payload strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read stream failed: %v, payload=%s", readErr, payload.String())
		}
		payload.WriteString(line)
		body := payload.String()
		if strings.Contains(body, "event: snapshot") && strings.Contains(body, `"pending":2`) {
			if !strings.Contains(body, `"global_queue_len":3`) {
				t.Fatalf("body missing monitor data: %s", body)
			}
			if !strings.Contains(body, `"running_attempts":1`) {
				t.Fatalf("body missing running attempts: %s", body)
			}
			if !strings.Contains(body, `"execution_pressure"`) || !strings.Contains(body, `"normal_running"`) {
				t.Fatalf("body missing execution pressure: %s", body)
			}
			if !strings.Contains(body, `"golden_signals"`) {
				t.Fatalf("body missing golden signals: %s", body)
			}
			return
		}
	}
}

func TestStreamMonitorReturnsJSONErrorBeforeStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(
		NewService(fakeStatsStore{counts: map[string]int64{}}),
		fakeMonitorProvider{err: errors.New("monitor unavailable")},
	)

	router := gin.New()
	router.GET("/stream", handler.StreamMonitor)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("unexpected stream content-type=%q", got)
	}
	if !strings.Contains(rec.Body.String(), "获取监控数据失败") {
		t.Fatalf("body missing public error message: %s", rec.Body.String())
	}
}

// merged from support_test.go.
type Service = statisticsapp.Service
type MonitorExecutionPressurePoint = statisticsapp.MonitorExecutionPressurePoint
type MonitorGoldenSignals = statisticsapp.MonitorGoldenSignals

func NewService(store statisticsapp.Store) *Service { return statisticsapp.NewService(store) }

var monitorExecutionPressureBuckets = make([]struct{}, 11)
