package domain

import "time"

// Retry-layer values for execution-attempt facts (ADR-0012 rule 7).
// They match the ck_task_execution_attempts_retry_layer CHECK exactly;
// workspacetasks aliases these names for the workspace worker path.
const (
	RetryLayerGateway       = "gateway"
	RetryLayerModelFallback = "model_fallback"
	RetryLayerRiverTask     = "river_task"
)

// ExecutionAttemptStart opens one gateway execution attempt. The claim and
// the attempt row commit atomically, so a fenced token always has its row:
// the attempt sequence is server-assigned, never caller-supplied. The
// execution token must be unique per attempt; finalization requires the same
// token, which is how stale workers lose the race.
type ExecutionAttemptStart struct {
	TaskID         string
	RiverJobID     int64
	ExecutionToken string
	Service        string
	RequestedModel string
	Provider       string
	StartedAt      time.Time
	// RetryLayer distinguishes the gateway, model_fallback, and
	// river_task layers (ADR-0012 rule 7): the initial schedule
	// records gateway, a whole-task River redelivery records
	// river_task, and the single bounded in-execution send to the
	// snapshot's retry candidate records model_fallback.
	RetryLayer string
	// GatewayRetryCount records same-model gateway-internal retries
	// for this attempt. The application-to-gateway path is
	// single-shot, so it is 0 until the gateway surfaces
	// Cloudflare-internal tries.
	GatewayRetryCount int
}

// ExecutionAttemptFinish closes one running attempt. A zero-token match
// reports zero affected rows instead of an error: the caller already lost
// the race to another worker and must treat the task as converged.
type ExecutionAttemptFinish struct {
	ExecutionToken  string
	TaskID          string
	Status          string
	ResolvedModel   string
	GatewayStep     string
	ReceiptLogID    string
	ReceiptEventID  string
	CacheHit        bool
	FailureStage    string
	FailureCategory string
	ErrorMsg        string
	FinishedAt      time.Time
	CompletedAt     time.Time
	// RetryLayer and GatewayRetryCount echo the layer facts from the
	// matching ExecutionAttemptStart so terminal convergence carries
	// the layer that decided the outcome.
	RetryLayer        string
	GatewayRetryCount int
}
