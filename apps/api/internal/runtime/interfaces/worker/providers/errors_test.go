package providers

import (
	"context"
	"errors"
	"fmt"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	"testing"
)

func TestProviderHTTPStatusRecognizesWrappedError(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", runtimeports.NewUpstreamHTTPError(422, "invalid_image_request", 0))

	status, ok := runtimeports.ProviderHTTPStatus(err)
	if !ok {
		t.Fatalf("expected wrapped upstream http status to be recognized")
	}
	if status != 422 {
		t.Fatalf("unexpected status: got=%d want=422", status)
	}
}

func TestShouldFailoverProviderErrorByHTTPStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "400 non-retryable", status: 400, want: false},
		{name: "401 non-retryable", status: 401, want: false},
		{name: "402 non-retryable", status: 402, want: false},
		{name: "403 non-retryable", status: 403, want: false},
		{name: "404 non-retryable", status: 404, want: false},
		{name: "422 non-retryable", status: 422, want: false},
		{name: "408 retryable", status: 408, want: true},
		{name: "429 retryable", status: 429, want: true},
		{name: "500 retryable", status: 500, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", runtimeports.NewUpstreamHTTPError(tt.status, "upstream message", 0))
			if got := runtimeports.ShouldFailoverProviderError(err); got != tt.want {
				t.Fatalf("runtimeports.ShouldFailoverProviderError(%d)=%v, want=%v", tt.status, got, tt.want)
			}
		})
	}
}

func TestShouldFailoverProviderErrorRecognizesWrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", fmt.Errorf("HTTP 请求失败: %w", context.DeadlineExceeded))
	if !runtimeports.ShouldFailoverProviderError(err) {
		t.Fatalf("expected deadline exceeded to be retryable")
	}
}

func TestShouldFailoverProviderErrorDoesNotRetryContextCanceled(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", context.Canceled)
	if runtimeports.ShouldFailoverProviderError(err) {
		t.Fatalf("expected context canceled to be non-retryable")
	}
}

func TestShouldFailoverProviderErrorDoesNotRetryFinalizeOrStorageFailure(t *testing.T) {
	tests := []struct {
		name     string
		category string
	}{
		{name: "finalize", category: tasks.FailureCategoryFinalizeError},
		{name: "storage", category: tasks.FailureCategoryStorageError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runtimeports.NewExecutionFailureError(tasks.ExecutionStageFinalizing, tt.category, errors.New("save failed"))
			if runtimeports.ShouldFailoverProviderError(err) {
				t.Fatalf("expected %s failure to be non-retryable", tt.category)
			}
		})
	}
}

func TestExecutionFailureDetailsRecognizesWrappedError(t *testing.T) {
	err := fmt.Errorf("outer: %w",
		runtimeports.NewExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, errors.New("compose failed")),
	)

	stage, category, ok := runtimeports.ExecutionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details")
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryFinalizeError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError)
	}
}
