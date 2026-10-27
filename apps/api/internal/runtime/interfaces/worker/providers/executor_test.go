package providers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
	if runtimeports.GatewayDefaultTimeout != 300*time.Second {
		t.Fatalf("GatewayDefaultTimeout = %v, want 300s single budget (ADR-0010)", runtimeports.GatewayDefaultTimeout)
	}
}
