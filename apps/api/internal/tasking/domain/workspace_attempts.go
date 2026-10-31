package domain

import "time"

// Workspace execution-attempt facts (#848, ADR-0012 rule 7).

// Attempt retry-layer values, matching the ck_task_execution_attempts_retry_layer CHECK exactly.
const (
	AttemptRetryLayerGateway       = RetryLayerGateway
	AttemptRetryLayerModelFallback = RetryLayerModelFallback
	AttemptRetryLayerRiverTask     = RetryLayerRiverTask
)

// Attempt statuses mirror the task_execution_attempts CHECK.
const (
	AttemptStatusCompleted = "completed"
	AttemptStatusFailed    = "failed"
	AttemptStatusRequeued  = "requeued"
)

// AttemptStart opens one gateway send's attempt fact. AttemptNo is
// server-assigned, never caller-supplied; ExecutionToken is unique per
// row.
type AttemptStart struct {
	TaskID            string
	RiverJobID        int64
	ExecutionToken    string
	Service           string
	RequestedModel    string
	ResolvedModel     string
	Provider          string
	RetryLayer        string
	GatewayRetryCount int
	StartedAt         time.Time
}

// AttemptFinish closes one running attempt fact. A zero-row match means
// the fact was never recorded (degraded recorder) or a lost race: the
// caller logs and continues, never fails the task.
type AttemptFinish struct {
	ExecutionToken  string
	TaskID          string
	Status          string
	ResolvedModel   string
	FailureStage    string
	FailureCategory string
	ErrorMsg        string
	FinishedAt      time.Time
}
