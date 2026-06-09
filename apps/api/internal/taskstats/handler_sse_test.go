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

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/taskstore"

	"github.com/gin-gonic/gin"
)

type fakeMonitorProvider struct {
	info *queuetypes.MonitorInfo
	err  error
}

func (p fakeMonitorProvider) GetMonitorInfo(context.Context) (*queuetypes.MonitorInfo, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.info, nil
}

type fakeStatsStore struct {
	counts map[string]int64
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

func (s fakeStatsStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]taskstore.StatEventLatencySample, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) ListStatisticsByCreatedAtRange(context.Context, time.Time, time.Time) ([]taskstore.TaskStatisticLite, error) {
	return nil, errors.New("not implemented")
}

func (s fakeStatsStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (taskstore.BackfillDailyStatsResult, error) {
	return taskstore.BackfillDailyStatsResult{}, errors.New("not implemented")
}

func TestStreamMonitorSendsInitialSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(
		NewService(fakeStatsStore{counts: map[string]int64{
			tasks.StatusPending:   2,
			tasks.StatusRunning:   1,
			tasks.StatusCompleted: 5,
			tasks.StatusFailed:    1,
		}}),
		fakeMonitorProvider{info: &queuetypes.MonitorInfo{
			GlobalQueueLen: 3,
			WaitQueueLen:   2,
			RunningTasks:   1,
			AccountLoads: map[int64]*queuetypes.AccountLoad{
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
