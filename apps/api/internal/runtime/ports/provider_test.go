package ports_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

func TestWorkerCallerFunc(t *testing.T) {
	called := false
	var fn ports.WorkerCallerFunc = func(ctx context.Context, input ports.WorkerCallInput) (ports.ProviderCallOutput, error) {
		called = true
		if input.Model != "openai/gpt-image-1" {
			t.Errorf("input = %+v, want model openai/gpt-image-1", input)
		}
		return ports.ProviderCallOutput{Result: "ok"}, nil
	}

	out, err := fn.CallWorkerAPI(t.Context(), ports.WorkerCallInput{
		Task: &tasks.TaskPayload{TaskID: "t-1"}, Model: "openai/gpt-image-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called || out.Result != "ok" {
		t.Fatalf("expected called=true, result=ok, got called=%v, result=%s", called, out.Result)
	}
}

func TestUpstreamHTTPError(t *testing.T) {
	err := ports.NewUpstreamHTTPError(http.StatusTooManyRequests, "rate limit", time.Minute)
	var httpErr *ports.UpstreamHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected UpstreamHTTPError, got %T", err)
	}
	if httpErr.HTTPStatus() != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", httpErr.HTTPStatus())
	}
	if httpErr.RetryAfterDuration() != time.Minute {
		t.Fatalf("expected 1m, got %v", httpErr.RetryAfterDuration())
	}
	if httpErr.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestClassifyProviderFailure(t *testing.T) {
	upErr := ports.NewUpstreamHTTPError(http.StatusBadRequest, "invalid param", 0)
	stage, category := ports.ClassifyProviderFailure(upErr, ports.ProviderCallTimeline{})
	if category != tasks.FailureCategoryInputInvalid {
		t.Fatalf("expected category input_invalid, got %s", category)
	}
	if stage != tasks.ExecutionStageProvider {
		t.Fatalf("expected stage provider, got %s", stage)
	}
}
