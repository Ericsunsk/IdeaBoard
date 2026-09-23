package execution

import (
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

// attemptContext carries one gateway execution's in-memory facts. Attempt
// rows live behind the task execution service; this struct never touches
// storage directly.
type attemptContext struct {
	task           *tasks.TaskPayload
	model          string
	executionToken string
	attemptNo      int32
	startedAt      time.Time
	providerOutput runtimeports.ProviderCallOutput
}

func newAttemptContext(
	task *tasks.TaskPayload,
	model string,
	executionToken string,
	attemptNo int32,
	startedAt time.Time,
) attemptContext {
	return attemptContext{
		task:           task,
		model:          model,
		executionToken: executionToken,
		attemptNo:      attemptNo,
		startedAt:      startedAt,
	}
}

func (c attemptContext) withProviderOutput(
	output runtimeports.ProviderCallOutput,
) attemptContext {
	c.providerOutput = output
	return c
}

func (c attemptContext) elapsedSeconds() float64 {
	return time.Since(c.startedAt).Seconds()
}

// buildExecutionFinish maps one attempt outcome onto the execution-attempt
// finish payload. Receipt facts come from the gateway acknowledgement the
// caller already stored on the provider output.
func (c attemptContext) buildExecutionFinish(
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	completedAt time.Time,
) tasks.ExecutionAttemptFinish {
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	output := c.providerOutput
	return tasks.ExecutionAttemptFinish{
		ExecutionToken:  c.executionToken,
		TaskID:          c.task.TaskID,
		Status:          status,
		ResolvedModel:   output.ResolvedModel,
		GatewayStep:     output.GatewayStep,
		ReceiptLogID:    output.ReceiptLogID,
		ReceiptEventID:  output.ReceiptEventID,
		CacheHit:        output.CacheHit,
		FailureStage:    failureStage,
		FailureCategory: failureCategory,
		ErrorMsg:        runtimeports.TruncateErrorBody(reason),
		FinishedAt:      completedAt,
		CompletedAt:     completedAt,
	}
}
