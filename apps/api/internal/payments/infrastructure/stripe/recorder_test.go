package stripe

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"ideaboard/api/internal/platform/outboundhttp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stripe/stripe-go/v86"
)

func TestClassifyStripeErrorMapsClosedOutcomes(t *testing.T) {
	if got := classifyStripeError(nil); got != outboundhttp.OutcomeSuccess {
		t.Fatalf("nil = %q, want success", got)
	}
	if got := classifyStripeError(context.Canceled); got != outboundhttp.OutcomeCanceled {
		t.Fatalf("canceled = %q, want canceled", got)
	}
	if got := classifyStripeError(context.DeadlineExceeded); got != outboundhttp.OutcomeTimeout {
		t.Fatalf("deadline = %q, want timeout", got)
	}
	if got := classifyStripeError(&stripe.Error{HTTPStatusCode: http.StatusBadRequest}); got != outboundhttp.OutcomeClientError {
		t.Fatalf("400 = %q, want http_4xx", got)
	}
	if got := classifyStripeError(&stripe.Error{HTTPStatusCode: http.StatusServiceUnavailable}); got != outboundhttp.OutcomeServerError {
		t.Fatalf("503 = %q, want http_5xx", got)
	}
	if got := classifyStripeError(&stripe.Error{}); got != outboundhttp.OutcomeTransportErr {
		t.Fatalf("no status = %q, want transport_error", got)
	}
	if got := classifyStripeError(errors.New("boom")); got != outboundhttp.OutcomeTransportErr {
		t.Fatalf("plain = %q, want transport_error", got)
	}
}

func TestRecorderObservesTargetStripe(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := outboundhttp.NewMetrics(reg)
	rec := NewRecorderWithMetrics(metrics)
	rec.Observe(nil, 5*time.Millisecond)
	rec.Observe(&stripe.Error{HTTPStatusCode: http.StatusUnauthorized}, time.Millisecond)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	counts := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "outbound_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["target"] == Target {
				counts[labels["outcome"]] += metric.GetCounter().GetValue()
			}
		}
	}
	if counts[outboundhttp.OutcomeSuccess] != 1 {
		t.Fatalf("stripe success = %v, want 1", counts[outboundhttp.OutcomeSuccess])
	}
	if counts[outboundhttp.OutcomeClientError] != 1 {
		t.Fatalf("stripe client error = %v, want 1", counts[outboundhttp.OutcomeClientError])
	}
}

func TestNilRecorderNeverCrashes(t *testing.T) {
	var rec *Recorder
	rec.Observe(nil, time.Millisecond)
	var adapter *Adapter
	adapter.observe(nil, time.Now())
}
