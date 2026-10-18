package providers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	providerdomain "ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

func TestMarshalProviderTaskResultReturnsContextualError(t *testing.T) {
	got, err := marshalProviderTaskResult("task-json-fail", map[string]interface{}{
		"task_id":     "task-json-fail",
		"unsupported": func() {},
	})
	if err == nil {
		t.Fatal("expected marshalProviderTaskResult to return an error")
	}
	if got != "" {
		t.Fatalf("result=%q, want empty string", got)
	}
	if !strings.Contains(err.Error(), "序列化任务结果失败") || !strings.Contains(err.Error(), "task-json-fail") {
		t.Fatalf("expected contextual serialization error, got %v", err)
	}

	var unsupportedTypeErr *json.UnsupportedTypeError
	if !errors.As(err, &unsupportedTypeErr) {
		t.Fatalf("expected wrapped json.UnsupportedTypeError, got %T (%v)", err, err)
	}
	stage, category, ok := runtimeports.ExecutionFailureDetails(err)
	if !ok {
		t.Fatal("expected execution failure details")
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryFinalizeError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError)
	}
}

func TestGatewayCallTimeoutStandardBudget(t *testing.T) {
	if got := gatewayCallTimeout(providerdomain.ProviderRuntimeAdapterCutout); got != defaultGatewayCallTimeout {
		t.Fatalf("cutout timeout = %v, want %v", got, defaultGatewayCallTimeout)
	}
	if got := gatewayCallTimeout(providerdomain.ProviderRuntimeAdapterModelImage); got != 300*time.Second {
		t.Fatalf("model image timeout = %v, want %v", got, 300*time.Second)
	}
	if got := gatewayCallTimeout(providerdomain.ProviderRuntimeAdapterUpscale); got != 300*time.Second {
		t.Fatalf("upscale timeout = %v, want %v", got, 300*time.Second)
	}
}
