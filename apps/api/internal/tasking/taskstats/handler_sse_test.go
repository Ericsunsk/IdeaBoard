package taskstats

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"

	"github.com/gin-gonic/gin"
)

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
	counts       map[string]int64
	pressureRows []taskstore.MonitorExecutionPressureAggregate
	pressureErr  error
}

func (s fakeStatsStore) CountByStatus(_ context.Context, status string) (int64, error) {
	return s.counts[status], nil
}

func (s fakeStatsStore) CountAllTasks(context.Context) (int64, error) {
	return 0, nil
}

func (s fakeStatsStore) ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]taskstore.DailyStatAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]taskstore.ExecutionBottleneckAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]taskstore.ExecutionFailureCategoryAggregate, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]taskstore.StatEventTrendRow, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]taskstore.StatEventLatencySample, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]taskstore.MonitorExecutionPressureAggregate, error) {
	if s.pressureErr != nil {
		return nil, s.pressureErr
	}
	return s.pressureRows, nil
}

func (s fakeStatsStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (taskstore.BackfillDailyStatsResult, error) {
	return taskstore.BackfillDailyStatsResult{}, errors.New("not implemented")
}

func TestGetMonitorExecutionPressureBuildsStableBuckets(t *testing.T) {
	svc := NewService(fakeStatsStore{
		pressureRows: []taskstore.MonitorExecutionPressureAggregate{
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

func TestAdminMonitorDataFromRuntimeNormalizesNilCollections(t *testing.T) {
	data := adminMonitorDataFromRuntime(&tasks.MonitorInfo{
		GlobalQueueLen:  4,
		WaitQueueLen:    1,
		RunningAttempts: 2,
	}, nil)

	if data.RunningAttempts != 2 {
		t.Fatalf("running attempts=%d, want 2", data.RunningAttempts)
	}
	if data.AccountLoads == nil {
		t.Fatal("account loads should be an empty object map, got nil")
	}
	if data.ExecutionPressure == nil {
		t.Fatal("execution pressure should be an empty array, got nil")
	}

	empty := adminMonitorDataFromRuntime(nil, nil)
	if empty.AccountLoads == nil {
		t.Fatal("nil runtime info should still return an empty account loads map")
	}
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
		}, pressureRows: []taskstore.MonitorExecutionPressureAggregate{
			{Key: "normal_running", Total: 1, AvgDurationSeconds: 12, MaxDurationSeconds: 12},
		}}),
		fakeMonitorProvider{info: &tasks.MonitorInfo{
			GlobalQueueLen:  3,
			WaitQueueLen:    2,
			RunningAttempts: 1,
			AccountLoads: map[int64]*tasks.AccountLoad{
				9: {CurrentConcurrent: 1, MaxConcurrent: 4},
			},
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
