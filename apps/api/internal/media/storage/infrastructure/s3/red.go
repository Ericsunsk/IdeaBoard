package s3

import (
	"context"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/platform/outboundhttp"

	"github.com/minio/minio-go/v7"
)

// s3DataTarget names S3-compatible object egress in outbound RED. It reuses
// the delivery-read target so storage Put/Delete/Get share one alertable
// target instead of scattering per-operation names.
const s3DataTarget = s3DeliveryTarget

// observeS3Data records one finished minio data-plane call (Put/Delete) in
// the shared outbound RED set. The SDK itself is untouched: callers time
// the call and report the outcome here.
func observeS3Data(err error, latency time.Duration) {
	outboundhttp.DefaultMetrics.Observe(s3DataTarget, classifyS3Error(err), latency)
}

func classifyS3Error(err error) string {
	if err == nil {
		return outboundhttp.OutcomeSuccess
	}
	if errors.Is(err, context.Canceled) {
		return outboundhttp.OutcomeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return outboundhttp.OutcomeTimeout
	}
	var response minio.ErrorResponse
	if errors.As(err, &response) {
		if response.StatusCode == 0 {
			// Transport-level minio failures carry no HTTP status.
			return outboundhttp.OutcomeTransportErr
		}
		return outboundhttp.OutcomeForStatus(response.StatusCode)
	}
	var responsePtr *minio.ErrorResponse
	if errors.As(err, &responsePtr) && responsePtr != nil {
		if responsePtr.StatusCode == 0 {
			return outboundhttp.OutcomeTransportErr
		}
		return outboundhttp.OutcomeForStatus(responsePtr.StatusCode)
	}
	// StatObject miss probes (NoSuchKey) surface as plain errors without an
	// ErrorResponse on some backends; they are control flow, not failures.
	if isS3NotFoundError(err) {
		return outboundhttp.OutcomeSuccess
	}
	return outboundhttp.OutcomeTransportErr
}

func isS3NotFoundError(err error) bool {
	if err == nil {
		return false
	}
	code := strings.ToLower(s3ErrorCode(err))
	switch code {
	case "nosuchkey", "notfound", "nosuchobject", "nosuchbucket":
		return true
	default:
		return false
	}
}
