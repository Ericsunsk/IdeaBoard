package worker

import (
	"context"
	"errors"
	"fmt"
	tasks "ideaboard/api/internal/tasking/domain"
	"testing"
)

func TestProviderHTTPStatusRecognizesWrappedError(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", newUpstreamHTTPError(422, "invalid_image_request"))

	status, ok := providerHTTPStatus(err)
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
			err := fmt.Errorf("wrapped: %w", newUpstreamHTTPError(tt.status, "upstream message"))
			if got := shouldFailoverProviderError(err); got != tt.want {
				t.Fatalf("shouldFailoverProviderError(%d)=%v, want=%v", tt.status, got, tt.want)
			}
		})
	}
}

func TestShouldFailoverProviderErrorRecognizesWrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", fmt.Errorf("HTTP 请求失败: %w", context.DeadlineExceeded))
	if !shouldFailoverProviderError(err) {
		t.Fatalf("expected deadline exceeded to be retryable")
	}
}

func TestShouldFailoverProviderErrorDoesNotRetryContextCanceled(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: test): %w", context.Canceled)
	if shouldFailoverProviderError(err) {
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
			err := newExecutionFailureError(tasks.ExecutionStageFinalizing, tt.category, errors.New("save failed"))
			if shouldFailoverProviderError(err) {
				t.Fatalf("expected %s failure to be non-retryable", tt.category)
			}
		})
	}
}

func TestExecutionFailureDetailsRecognizesWrappedError(t *testing.T) {
	err := fmt.Errorf("outer: %w",
		newExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, errors.New("compose failed")),
	)

	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details")
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryFinalizeError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError)
	}
}

func TestClassifyProviderFailureUsesExecutionFailureDetails(t *testing.T) {
	err := newExecutionFailureError(
		tasks.ExecutionStageFinalizing,
		tasks.FailureCategoryStorageError,
		errors.New("object storage failed"),
	)

	stage, category := classifyProviderFailure(err, providerCallTimeline{})
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryStorageError {
		t.Fatalf("classification=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryStorageError)
	}
}

func TestUserFacingProviderErrorForHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "422 invalid params",
			err:  fmt.Errorf("wrapped: %w", newUpstreamHTTPError(422, "invalid_image_request")),
			want: "请求参数不合法，请检查输入后重试",
		},
		{
			name: "402 payment required",
			err:  fmt.Errorf("wrapped: %w", newUpstreamHTTPError(402, "insufficient_quota")),
			want: "上游服务额度不足或未开通，请联系管理员",
		},
		{
			name: "generic unknown",
			err:  errors.New("boom"),
			want: "上游图像服务调用失败",
		},
		{
			name: "storage persistence failure",
			err: newExecutionFailureError(
				tasks.ExecutionStageFinalizing,
				tasks.FailureCategoryStorageError,
				errors.New("持久化上游图片失败: 保存文件失败"),
			),
			want: "图片结果保存失败，请检查存储配置",
		},
		{
			name: "finalize failure",
			err: newExecutionFailureError(
				tasks.ExecutionStageFinalizing,
				tasks.FailureCategoryFinalizeError,
				errors.New("local redraw 回贴失败"),
			),
			want: "图片结果保存失败，请检查存储配置",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := userFacingProviderError(tt.err); got != tt.want {
				t.Fatalf("userFacingProviderError()=%q, want=%q", got, tt.want)
			}
		})
	}
}
