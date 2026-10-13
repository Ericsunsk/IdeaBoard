package task

import (
	"time"

	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/google/uuid"
)

type AdminMonitorListItem struct {
	TaskID          string    `json:"task_id"`
	UserPublicID    uuid.UUID `json:"user_public_id"`
	UserDisplayName string    `json:"user_display_name"`
	Prompt          string    `json:"prompt"`
	Service         string    `json:"service"`
	// RequestedResolution is the submit-time snapshot's requested
	// resolution (#852): the monitor list records the requested
	// Service + resolution without leaking pool internals beyond the
	// selection facts below. Empty when the task never pinned a
	// snapshot (pre-pool row).
	RequestedResolution string `json:"requested_resolution,omitempty"`
	// PoolVersion is the per-Service pool version the task's snapshot
	// pinned (ADR-0012 rule 9). Zero means "never pinned".
	PoolVersion uint64 `json:"pool_version,omitempty"`
	// SelectedModel is the snapshot's deterministic selection; it is
	// the first send's model, not necessarily the final one.
	SelectedModel string `json:"selected_model,omitempty"`
	// FinalResolvedModel attributes the list row to the resolved
	// (final) model derived from attempt facts. Empty when attempts
	// carry no model data yet.
	FinalResolvedModel string                         `json:"final_resolved_model,omitempty"`
	Credits            int64                          `json:"credits"`
	Priority           int                            `json:"priority"`
	Status             string                         `json:"status"`
	ErrorMsg           string                         `json:"error_msg"`
	Result             string                         `json:"result"`
	CreatedAt          time.Time                      `json:"created_at"`
	CompletedAt        *time.Time                     `json:"completed_at"`
	LatestExecution    *AdminMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
}

type AdminMonitorDetail struct {
	TaskID            string                         `json:"task_id"`
	CorrelationID     string                         `json:"correlation_id"`
	UserPublicID      uuid.UUID                      `json:"user_public_id"`
	UserDisplayName   string                         `json:"user_display_name"`
	Prompt            string                         `json:"prompt"`
	UserPrompt        string                         `json:"user_prompt"`
	Params            string                         `json:"params"`
	Service           string                         `json:"service"`
	Priority          int                            `json:"priority"`
	Status            string                         `json:"status"`
	RetryCount        int                            `json:"retry_count"`
	ErrorMsg          string                         `json:"error_msg"`
	Result            string                         `json:"result"`
	Outputs           []tasks.TaskOutput             `json:"outputs,omitempty"`
	ExecutionMetrics  *tasks.TaskExecutionMetrics    `json:"execution_metrics,omitempty"`
	CreatedAt         time.Time                      `json:"created_at"`
	UpdatedAt         time.Time                      `json:"updated_at"`
	CompletedAt       *time.Time                     `json:"completed_at"`
	CancelledAt       *time.Time                     `json:"cancelled_at"`
	CancelReason      string                         `json:"cancel_reason"`
	Billing           *tasks.AdminBillingSnapshot    `json:"billing,omitempty"`
	LatestExecution   *AdminMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	ExecutionAttempts []tasks.AdminTaskAttempt       `json:"execution_attempts"`
	// Routing is the read-only projection over the immutable
	// submit-time RoutingSnapshot (#852, ADR-0012 rule 9): requested
	// service/resolution, eligible models, selected + retry candidate,
	// and pool version. Nil when the task never pinned a snapshot.
	// Read-only: rendering it never changes task or billing state.
	Routing *tasks.AdminRoutingSnapshot `json:"routing,omitempty"`
	// FinalResolvedModel attributes execution to the resolved (final)
	// model derived from attempt facts (#852). Empty when attempts
	// carry no model data. Selected (first-send) vs final (terminal)
	// stays distinguishable via Routing.SelectedModel.
	FinalResolvedModel string `json:"final_resolved_model,omitempty"`
}

type AdminMonitorExecutionSnapshot = tasks.AdminTaskExecutionSnapshot
