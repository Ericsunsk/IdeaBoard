package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveHTTPLabelsRouteAndStatus(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.ObserveHTTP("GET", "/api/user/tasks", 200, 25*time.Millisecond)
	set.ObserveHTTP("POST", "/api/user/tasks/submit", 500, 120*time.Millisecond)
	set.ObserveHTTP("GET", "", 404, time.Millisecond)

	if got := testutil.ToFloat64(set.httpRequestsTotal.WithLabelValues("GET", "/api/user/tasks", "200")); got != 1 {
		t.Fatalf("route counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.httpRequestsTotal.WithLabelValues("POST", "/api/user/tasks/submit", "500")); got != 1 {
		t.Fatalf("error counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.httpRequestsTotal.WithLabelValues("GET", UnmatchedRoute, "404")); got != 1 {
		t.Fatalf("unmatched counter = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(set.httpRequestTime); got != 3 {
		t.Fatalf("histogram series = %d, want 3", got)
	}
}

func TestNilSetObserversNeverCrash(t *testing.T) {
	var set *Set
	set.ObserveHTTP("GET", "/x", 200, time.Millisecond)
	set.ObserveGatewayRequest("render", nil)
	set.ObserveGatewayRequest("render", errors.New("boom"))
	set.IncGatewayRetry("render")
	set.IncModelFallback("render", nil)
	set.IncModelFallback("render", errors.New("boom"))
	set.SetRiverQueue("drawing_default", 3, 1.5)
	set.RefreshRiverQueue(context.Background(), "drawing_default", func(context.Context) (int64, float64, error) {
		return 1, 0.5, nil
	},
	)

}

func TestGatewayCountersSplitByFeatureAndOutcome(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.ObserveGatewayRequest("render", nil)
	set.ObserveGatewayRequest("render", errors.New("transport failed"))
	set.ObserveGatewayRequest("", errors.New("transport failed"))
	set.IncGatewayRetry("render")
	set.IncGatewayRetry("")

	if got := testutil.ToFloat64(set.gatewayRequests.WithLabelValues("render", "success")); got != 1 {
		t.Fatalf("gateway success = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.gatewayRequests.WithLabelValues("render", "error")); got != 1 {
		t.Fatalf("gateway error = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.gatewayRequests.WithLabelValues(UnknownService, "error")); got != 1 {
		t.Fatalf("gateway unknown error = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.gatewayRetries.WithLabelValues("render")); got != 1 {
		t.Fatalf("gateway retries = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.gatewayRetries.WithLabelValues(UnknownService)); got != 1 {
		t.Fatalf("gateway unknown retries = %v, want 1", got)
	}
}

func TestModelFallbackCounterSplitsByServiceAndOutcome(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.IncModelFallback("render", nil)
	set.IncModelFallback("render", errors.New("candidate failed"))
	set.IncModelFallback("", errors.New("candidate failed"))

	if got := testutil.ToFloat64(set.modelFallbacks.WithLabelValues("render", "success")); got != 1 {
		t.Fatalf("fallback success = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.modelFallbacks.WithLabelValues("render", "error")); got != 1 {
		t.Fatalf("fallback error = %v, want 1", got)
	}
	if got := testutil.ToFloat64(set.modelFallbacks.WithLabelValues(UnknownService, "error")); got != 1 {
		t.Fatalf("fallback unknown error = %v, want 1", got)
	}
}

func TestRefreshRiverQueuePublishesGauges(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.RefreshRiverQueue(context.Background(), "drawing_default", func(context.Context) (int64, float64, error) {
		return 7, 42.5, nil
	})
	if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("drawing_default")); got != 7 {
		t.Fatalf("queue depth = %v, want 7", got)
	}
	if got := testutil.ToFloat64(set.riverQueueLag.WithLabelValues("drawing_default")); got != 42.5 {
		t.Fatalf("queue lag = %v, want 42.5", got)
	}
}

func TestRefreshRiverQueueKeepsLastGaugesOnError(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.SetRiverQueue("drawing_default", 4, 9)
	set.RefreshRiverQueue(context.Background(), "drawing_default", func(context.Context) (int64, float64, error) {
		return 0, 0, errors.New("db down")
	})
	if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("drawing_default")); got != 4 {
		t.Fatalf("queue depth after error = %v, want 4", got)
	}
}

func TestMultiRefresherTicksUntilStopped(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	calls := 0
	refresher := NewMultiRefresher(set, map[string]RiverQueueStats{
		"drawing_default": func(context.Context) (int64, float64, error) {
			calls++
			return int64(calls), float64(calls), nil
		},
	}, 10*time.Millisecond)
	if refresher.Name() == "" {
		t.Fatal("refresher name must not be empty")
	}
	if err := refresher.Start(context.Background()); err != nil {
		t.Fatalf("Start = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("drawing_default")); got >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresher did not tick")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := refresher.Stop(stopCtx); err != nil {
		t.Fatalf("Stop = %v", err)
	}
}

func TestNewSetReusesAlreadyRegisteredCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	first := NewSet(reg)
	second := NewSet(reg)
	second.ObserveHTTP("GET", "/api/health", 200, time.Millisecond)
	if got := testutil.ToFloat64(first.httpRequestsTotal.WithLabelValues("GET", "/api/health", "200")); got != 1 {
		t.Fatalf("reused counter = %v, want 1", got)
	}
}

func TestMultiRefresherPublishesEveryQueue(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	stats := map[string]RiverQueueStats{
		"drawing_default": func(context.Context) (int64, float64, error) { return 3, 1.5, nil },
		"task_retention":  func(context.Context) (int64, float64, error) { return 0, 0, nil },
		"":                func(context.Context) (int64, float64, error) { return 99, 99, nil },
		"dropped":         nil,
	}
	refresher := NewMultiRefresher(set, stats, 0)
	names := refresher.Queues()
	if len(names) != 2 {
		t.Fatalf("polled queues = %v, want 2 (empty name and nil stats dropped)", names)
	}
	if err := refresher.Start(context.Background()); err != nil {
		t.Fatalf("Start = %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := refresher.Stop(stopCtx); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("drawing_default")); got != 3 {
		t.Fatalf("drawing depth = %v, want 3", got)
	}
	if got := testutil.ToFloat64(set.riverQueueLag.WithLabelValues("drawing_default")); got != 1.5 {
		t.Fatalf("drawing lag = %v, want 1.5", got)
	}
	if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("task_retention")); got != 0 {
		t.Fatalf("retention depth = %v, want 0", got)
	}
}

func TestMultiRefresherKeepsOldGaugesOnQueueError(t *testing.T) {
	set := NewSet(prometheus.NewRegistry())
	set.SetRiverQueue("vision_billing", 4, 2)
	refresher := NewMultiRefresher(set, map[string]RiverQueueStats{
		"vision_billing": func(context.Context) (int64, float64, error) {
			return 0, 0, errors.New("db down")
		},
	}, 0)
	if err := refresher.Start(context.Background()); err != nil {
		t.Fatalf("Start = %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := refresher.Stop(stopCtx); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	if got := testutil.ToFloat64(set.riverQueueDepth.WithLabelValues("vision_billing")); got != 4 {
		t.Fatalf("queue depth after error = %v, want 4", got)
	}
}

func TestNilMultiRefresherNeverCrashes(t *testing.T) {
	var refresher *MultiRefresher
	if err := refresher.Start(context.Background()); err != nil {
		t.Fatalf("nil Start = %v", err)
	}
	if err := refresher.Stop(context.Background()); err != nil {
		t.Fatalf("nil Stop = %v", err)
	}
	if got := refresher.Queues(); len(got) != 0 {
		t.Fatalf("nil Queues() = %v, want empty", got)
	}
	disabled := NewMultiRefresher(nil, nil, 0)
	if err := disabled.Start(context.Background()); err != nil {
		t.Fatalf("disabled Start = %v", err)
	}
}
