package ports

import (
	"context"
)

// TaskExecutionRequest carries the execution payload dispatched to an external AI provider.
type TaskExecutionRequest struct {
	TaskID          string
	WorkflowID      string
	ProviderAccount string
	Model           string
	Parameters      map[string]any
}

// TaskExecutionResponse represents the canonical raw output envelope from an execution attempt.
type TaskExecutionResponse struct {
	TaskID     string
	Status     string
	OutputData []byte
	Error      string
}

// TaskExecutor defines the runtime contract for executing a provider task attempt.
type TaskExecutor interface {
	Execute(ctx context.Context, req TaskExecutionRequest) (TaskExecutionResponse, error)
}
