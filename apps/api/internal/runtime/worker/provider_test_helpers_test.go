package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
)

func marshalWorkflowTaskParamsForTest(t *testing.T, params *tasks.WorkflowTaskParams) string {
	t.Helper()
	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal workflow task params: %v", err)
	}
	return string(payload)
}

func roundTripDrawingTaskForTest(t *testing.T, task *tasks.TaskPayload) *tasks.TaskPayload {
	t.Helper()
	payload, err := json.Marshal(taskqueue.DrawingTaskArgs{Task: *task})
	if err != nil {
		t.Fatalf("marshal drawing task args: %v", err)
	}
	var decoded taskqueue.DrawingTaskArgs
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal drawing task args: %v", err)
	}
	return &decoded.Task
}

func (c *ProviderExecutor) callOpenAIForTest(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	return c.callOpenAIWithRequest(ctx, account, newProviderRequest(prompt, width, height, params, resolvedModel), timeout)
}

func (c *ProviderExecutor) callGoogleAIForTest(ctx context.Context, account *accounts.ApiAccount, prompt string, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	return c.callGoogleAIWithRequest(ctx, account, newProviderRequest(prompt, 0, 0, params, resolvedModel), timeout)
}

func (c *ProviderExecutor) callDrawingAPIForTest(ctx context.Context, account *accounts.ApiAccount, task *tasks.TaskPayload) (providerCallOutput, error) {
	return c.callDrawingAPIWithAssignment(ctx, account, task, taskqueue.TaskAssignment{})
}
