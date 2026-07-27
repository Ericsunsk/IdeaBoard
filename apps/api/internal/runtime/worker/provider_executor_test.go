package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
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

func TestResolveAccountModelForExecutionRequiresExactAssignedModel(t *testing.T) {
	account := &accounts.ApiAccount{Models: []accounts.AccountModel{
		{Name: "first-model", Enabled: true},
		{Name: "assigned-model", Enabled: true},
	}}
	params := newProviderTaskParams(&tasks.WorkflowTaskParams{})

	model, ok := resolveAccountModelForExecution(account, params, "assigned-model")
	if !ok || model.Name != "assigned-model" {
		t.Fatalf("resolved model = %+v ok=%v, want exact assigned-model", model, ok)
	}
	if _, ok := resolveAccountModelForExecution(account, params, "missing-model"); ok {
		t.Fatal("assigned missing model must not fall back to another account model")
	}
	if _, ok := resolveAccountModelForExecution(account, params, ""); ok {
		t.Fatal("empty assigned model must not fall back to the account's first model")
	}
}

func TestResolveAccountTimeout(t *testing.T) {
	tests := []struct {
		name    string
		account *accounts.ApiAccount
		want    time.Duration
	}{
		{
			name:    "nil account fallback",
			account: nil,
			want:    120 * time.Second,
		},
		{
			name: "negative timeout fallback",
			account: &accounts.ApiAccount{
				Timeout: -10,
			},
			want: 120 * time.Second,
		},
		{
			name: "zero timeout fallback",
			account: &accounts.ApiAccount{
				Timeout: 0,
			},
			want: 120 * time.Second,
		},
		{
			name: "positive timeout keep",
			account: &accounts.ApiAccount{
				Timeout: 75,
			},
			want: 75 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAccountTimeout(tt.account); got != tt.want {
				t.Fatalf("resolveAccountTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
