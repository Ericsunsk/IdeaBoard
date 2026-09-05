package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
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
	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatal("expected execution failure details")
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryFinalizeError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError)
	}
}

func TestResolveRuntimeAccountTimeout(t *testing.T) {
	tests := []struct {
		name    string
		account *accounts.RuntimeAccountAccess
		want    time.Duration
	}{
		{
			name:    "nil account fallback",
			account: nil,
			want:    120 * time.Second,
		},
		{
			name: "negative timeout fallback",
			account: &accounts.RuntimeAccountAccess{
				Timeout: -10,
			},
			want: 120 * time.Second,
		},
		{
			name: "zero timeout fallback",
			account: &accounts.RuntimeAccountAccess{
				Timeout: 0,
			},
			want: 120 * time.Second,
		},
		{
			name: "positive timeout keep",
			account: &accounts.RuntimeAccountAccess{
				Timeout: 75,
			},
			want: 75 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRuntimeAccountTimeout(tt.account); got != tt.want {
				t.Fatalf("resolveRuntimeAccountTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveProviderTimeoutGrantsFluxFillColdStartBudget(t *testing.T) {
	account := &accounts.RuntimeAccountAccess{Timeout: 120}

	if got := resolveProviderTimeout(account, providerRuntimeAdapterFluxFill); got != minimumFluxFillTimeout {
		t.Fatalf("Flux Fill timeout = %v, want %v", got, minimumFluxFillTimeout)
	}
	if got := resolveProviderTimeout(account, providerRuntimeAdapterCutout); got != 120*time.Second {
		t.Fatalf("cutout timeout = %v, want %v", got, 120*time.Second)
	}
}
