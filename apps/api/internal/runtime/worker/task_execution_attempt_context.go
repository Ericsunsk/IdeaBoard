package worker

import (
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

type taskExecutionAttemptContext struct {
	task           *tasks.TaskPayload
	assignment     taskqueueports.TaskAssignment
	accountID      int64
	startedAt      time.Time
	attemptRef     tasks.TaskAttemptRef
	providerOutput providerCallOutput
}

func newTaskExecutionAttemptContext(
	task *tasks.TaskPayload,
	assignment taskqueueports.TaskAssignment,
	accountID int64,
	startedAt time.Time,
	attemptRef tasks.TaskAttemptRef,
) taskExecutionAttemptContext {
	return taskExecutionAttemptContext{
		task:       task,
		assignment: assignment,
		accountID:  accountID,
		startedAt:  startedAt,
		attemptRef: attemptRef,
	}
}

func (c taskExecutionAttemptContext) withProviderOutput(
	output providerCallOutput,
) taskExecutionAttemptContext {
	c.providerOutput = output
	return c
}

func (c taskExecutionAttemptContext) elapsedSeconds() float64 {
	return time.Since(c.startedAt).Seconds()
}

func (c taskExecutionAttemptContext) buildAttemptFinish(
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	completedAt time.Time,
) (tasks.TaskAttemptFinish, bool) {
	return buildTaskAttemptFinish(
		c.attemptRef,
		status,
		reason,
		failureStage,
		failureCategory,
		c.startedAt,
		c.providerOutput.Timeline,
		completedAt,
	)
}
