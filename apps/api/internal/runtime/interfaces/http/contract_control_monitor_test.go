package httpapi

// Control monitor/statistics/backfill liveness tests (#809-close). The
// workspace-backed statistics store wires ContractControlDeps.Monitor off
// its honest 501, so these routes must answer 200 with the legacy admin
// shape. The nil-Monitor fail-closed path stays covered by
// TestControlUnwiredDepsFailClosed in contract_control_auth_test.go.

import (
	"context"
	"net/http"
	"testing"
	"time"

	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
)

type fakeMonitorStatsStore struct{}

func (fakeMonitorStatsStore) CountByStatus(context.Context, string) (int64, error) {
	return 0, nil
}

func (fakeMonitorStatsStore) CountAllTaskCalls(context.Context) (int64, error) {
	return 0, nil
}

func (fakeMonitorStatsStore) ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListDailyCaptureCreditsByRange(context.Context, time.Time, time.Time, *time.Location) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (fakeMonitorStatsStore) ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	return statisticscontracts.BackfillDailyStatsResult{}, nil
}

type fakeMonitorInfoProvider struct{}

func (fakeMonitorInfoProvider) GetMonitorInfo(context.Context) (*tasksdomain.MonitorInfo, error) {
	return &tasksdomain.MonitorInfo{}, nil
}

func newMonitorHarness(t *testing.T) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(deps *ContractControlDeps) {
		deps.Monitor = statisticshttp.NewHandler(
			statisticsapp.NewService(fakeMonitorStatsStore{}),
			fakeMonitorInfoProvider{},
		)
	})
}

func controlMonitorData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("success envelope requires data object: %v", payload)
	}
	return data
}

func TestControlMonitorReadsLive(t *testing.T) {
	h := newMonitorHarness(t)
	admin := func(c controlCall) controlCall {
		c.userID = controlOperatorID
		c.role = "admin"
		return c
	}
	if status, _, payload := h.serve(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/platform/monitor"})); status != http.StatusOK {
		t.Fatalf("wired monitor = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["monitor"] == nil || data["stats"] == nil {
		t.Fatalf("monitor data = %v, want monitor + stats keys", data)
	}
	if status, _, payload := h.serve(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/platform/statistics?range=7d"})); status != http.StatusOK {
		t.Fatalf("wired statistics = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["summary"] == nil {
		t.Fatalf("statistics data = %v, want summary key", data)
	}
}

func TestControlStatisticsBackfillLive(t *testing.T) {
	h := newMonitorHarness(t)
	admin := func(c controlCall) controlCall {
		c.userID = controlOperatorID
		c.role = "admin"
		return c
	}
	call := admin(controlCall{
		method: http.MethodPost, path: "/api/control/v1/platform/statistics/backfill",
		body: `{"range":"7d"}`, headers: controlMutationHeaders("monitor-backfill-1", "rebuild stats after deploy"),
	})
	if status, _, payload := h.serve(call); status != http.StatusOK {
		t.Fatalf("wired backfill = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["range"] == nil {
		t.Fatalf("backfill data = %v, want range key", data)
	}
}
