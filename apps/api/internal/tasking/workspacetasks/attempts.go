package workspacetasks

// Workspace execution-attempt facts (#848, ADR-0012 rule 7).
//
// The worker records one durable attempt row per gateway send so the
// three retry layers stay distinguishable in PostgreSQL, logs, and
// Admin views:
//
//	gateway        -> the execution's send to the snapshot's current
//	                  model (initial schedule; same-model gateway
//	                  request retries stay inside the gateway client
//	                  and never surface as new rows).
//	model_fallback -> the single bounded in-execution send to the
//	                  snapshot's retry candidate. The row is inserted
//	                  BEFORE the send: a later redelivery observes it
//	                  and never spends the fallback budget twice.
//	river_task     -> a send owned by a whole-task River redelivery
//	                  (task.retry_count > 0 at claim time).
//
// GatewayRetryCount records same-model gateway-internal retries for
// that send. The application-to-gateway path is single-shot, so it is
// 0 until the gateway surfaces Cloudflare-internal tries (#852).
//
// Attempt writes are observability, never settlement: a failing
// recorder warns and the execution continues, except the fallback
// send itself, which requires its persisted model_fallback row (no
// row, no fallback) so at-most-once survives redelivery. Telemetry
// failure never changes task or billing state.

import (
	"context"
	"errors"
	"sync"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

// Attempt retry-layer values, canonical in tasking/domain to match the
// ck_task_execution_attempts_retry_layer CHECK exactly.
const (
	AttemptRetryLayerGateway       = tasks.RetryLayerGateway
	AttemptRetryLayerModelFallback = tasks.RetryLayerModelFallback
	AttemptRetryLayerRiverTask     = tasks.RetryLayerRiverTask
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

// AttemptRecorder is the durable attempt-fact seam. The postgres store
// implements it over task_execution_attempts; the in-memory sibling
// below is the test backend. Repository methods take context.Context
// first and use the dbWithContext pattern in the postgres sibling.
type AttemptRecorder interface {
	StartAttempt(ctx context.Context, start AttemptStart) (int32, error)
	FinishAttempt(ctx context.Context, finish AttemptFinish) (int64, error)
	// HasModelFallbackAttempt reports whether any model_fallback row
	// exists for the task, in any status. True consumes the single
	// bounded fallback: later executions run the retry candidate
	// directly and never fall back again.
	HasModelFallbackAttempt(ctx context.Context, taskID string) (bool, error)
}

// recordedAttempt is one in-memory attempt fact.
type recordedAttempt struct {
	start     AttemptStart
	finish    *AttemptFinish
	attemptNo int32
}

// InMemoryAttemptRecorder is a mutex-guarded AttemptRecorder for tests.
// Attempt numbers are server-assigned per task (max+1), mirroring the
// postgres INSERT ... SELECT COALESCE(MAX(attempt_no),0)+1.
type InMemoryAttemptRecorder struct {
	mu       sync.Mutex
	attempts map[string][]*recordedAttempt
}

// NewInMemoryAttemptRecorderForTests builds an empty recorder.
func NewInMemoryAttemptRecorderForTests() *InMemoryAttemptRecorder {
	return &InMemoryAttemptRecorder{attempts: make(map[string][]*recordedAttempt)}
}

// StartAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) StartAttempt(_ context.Context, start AttemptStart) (int32, error) {
	if r == nil {
		return 0, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.attempts[start.TaskID]
	attemptNo := int32(len(rows) + 1)
	r.attempts[start.TaskID] = append(rows, &recordedAttempt{start: start, attemptNo: attemptNo})
	return attemptNo, nil
}

// FinishAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) FinishAttempt(_ context.Context, finish AttemptFinish) (int64, error) {
	if r == nil {
		return 0, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.attempts[finish.TaskID] {
		if row.start.ExecutionToken == finish.ExecutionToken && row.finish == nil {
			closed := finish
			row.finish = &closed
			return 1, nil
		}
	}
	return 0, nil
}

// HasModelFallbackAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) HasModelFallbackAttempt(_ context.Context, taskID string) (bool, error) {
	if r == nil {
		return false, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.attempts[taskID] {
		if row.start.RetryLayer == AttemptRetryLayerModelFallback {
			return true, nil
		}
	}
	return false, nil
}

// StartedAttempts returns a copy of every recorded start for assertions.
func (r *InMemoryAttemptRecorder) StartedAttempts(taskID string) []AttemptStart {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AttemptStart, 0, len(r.attempts[taskID]))
	for _, row := range r.attempts[taskID] {
		out = append(out, row.start)
	}
	return out
}

// FinishedAttempts returns a copy of every recorded finish for assertions.
func (r *InMemoryAttemptRecorder) FinishedAttempts(taskID string) []AttemptFinish {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []AttemptFinish
	for _, row := range r.attempts[taskID] {
		if row.finish != nil {
			out = append(out, *row.finish)
		}
	}
	return out
}
