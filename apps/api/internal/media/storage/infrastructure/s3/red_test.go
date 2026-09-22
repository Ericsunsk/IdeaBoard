package s3

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/outboundhttp"

	"github.com/minio/minio-go/v7"
)

func TestClassifyS3ErrorMapsClosedOutcomes(t *testing.T) {
	if got := classifyS3Error(nil); got != outboundhttp.OutcomeSuccess {
		t.Fatalf("nil = %q, want success", got)
	}
	if got := classifyS3Error(context.Canceled); got != outboundhttp.OutcomeCanceled {
		t.Fatalf("canceled = %q, want canceled", got)
	}
	if got := classifyS3Error(context.DeadlineExceeded); got != outboundhttp.OutcomeTimeout {
		t.Fatalf("deadline = %q, want timeout", got)
	}
	if got := classifyS3Error(minio.ErrorResponse{StatusCode: 403}); got != outboundhttp.OutcomeClientError {
		t.Fatalf("403 = %q, want http_4xx", got)
	}
	if got := classifyS3Error(minio.ErrorResponse{StatusCode: 503}); got != outboundhttp.OutcomeServerError {
		t.Fatalf("503 = %q, want http_5xx", got)
	}
	if got := classifyS3Error(errors.New("connection reset")); got != outboundhttp.OutcomeTransportErr {
		t.Fatalf("plain = %q, want transport_error", got)
	}
}

func TestS3DataTargetIsSharedWithDelivery(t *testing.T) {
	if s3DataTarget != s3DeliveryTarget {
		t.Fatalf("data target = %q, want delivery target %q (one alertable target)", s3DataTarget, s3DeliveryTarget)
	}
}
