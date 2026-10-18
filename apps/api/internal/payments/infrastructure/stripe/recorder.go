package stripe

import (
	"context"
	"errors"
	"net"
	"time"

	"ideaboard/api/internal/platform/safehttp"

	"github.com/stripe/stripe-go/v86"
)

// Target names Stripe egress in outbound RED logs/metrics. It stays fixed
// (never an account id or URL) so payment failures are alertable per target.
const Target = "stripe"

// Recorder wraps Stripe SDK calls in the shared outbound RED set without
// touching the SDK itself: one counter + latency observation per SDK call
// with the closed outbound outcome set.
type Recorder struct {
	metrics *safehttp.Metrics
}

// NewRecorder builds a Stripe RED recorder. Nil metrics falls back to the
// process-wide DefaultMetrics at observe time; tests inject an isolated
// instance via NewRecorderWithMetrics.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// NewRecorderWithMetrics builds a recorder writing to m (nil-safe).
func NewRecorderWithMetrics(m *safehttp.Metrics) *Recorder {
	return &Recorder{metrics: m}
}

func (r *Recorder) metricsOrDefault() *safehttp.Metrics {
	if r != nil && r.metrics != nil {
		return r.metrics
	}
	return safehttp.DefaultMetrics
}

// Observe records one finished SDK call. err == nil is success; Stripe
// errors map by HTTP status; context cancel/timeout keep their outcome.
func (r *Recorder) Observe(err error, latency time.Duration) {
	r.metricsOrDefault().Observe(Target, classifyStripeError(err), latency)
}

func classifyStripeError(err error) string {
	if err == nil {
		return safehttp.OutcomeSuccess
	}
	if errors.Is(err, context.Canceled) {
		return safehttp.OutcomeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return safehttp.OutcomeTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return safehttp.OutcomeTimeout
	}
	var stripeErr *stripe.Error
	if errors.As(err, &stripeErr) {
		if stripeErr.HTTPStatusCode == 0 {
			return safehttp.OutcomeTransportErr
		}
		return safehttp.OutcomeForStatus(stripeErr.HTTPStatusCode)
	}
	return safehttp.OutcomeTransportErr
}
