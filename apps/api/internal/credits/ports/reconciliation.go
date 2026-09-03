package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CreditHealthFindingSummary struct {
	Total    int64 `json:"total"`
	Warning  int64 `json:"warning"`
	Error    int64 `json:"error"`
	Critical int64 `json:"critical"`
	HighRisk int64 `json:"high_risk"`
}

type CreditHealthSnapshot struct {
	ActiveRun             *ReconciliationRun
	LatestSuccessfulRun   *ReconciliationRun
	LatestFailedRun       *ReconciliationRun
	OpenFindings          CreditHealthFindingSummary
	RestrictedWalletCount int64
	SnapshotTime          time.Time
}

type ReconciliationRunDispatcher interface {
	DispatchReconciliationRun(context.Context, pgx.Tx, int64) error
}

type ReconciliationRun struct {
	ID                  int64                   `json:"id"`
	Status              ReconciliationRunStatus `json:"status"`
	Trigger             ReconciliationTrigger   `json:"trigger"`
	RequestedBy         *int64                  `json:"-"`
	RequestedByPublicID *uuid.UUID              `json:"requested_by_public_id,omitempty"`
	RequestedByName     string                  `json:"requested_by_display_name,omitempty"`
	Reason              string                  `json:"reason,omitempty"`
	ScheduledFor        *time.Time              `json:"scheduled_for,omitempty"`
	FindingCount        int64                   `json:"finding_count"`
	CriticalCount       int64                   `json:"critical_count"`
	IsolatedWalletCount int64                   `json:"isolated_wallet_count"`
	AttemptCount        int                     `json:"attempt_count"`
	ErrorMessage        string                  `json:"error_message,omitempty"`
	RequestedAt         time.Time               `json:"requested_at"`
	StartedAt           *time.Time              `json:"started_at,omitempty"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
}

type ReconciliationFinding struct {
	ID                int64                       `json:"id"`
	Fingerprint       string                      `json:"fingerprint"`
	FindingType       ReconciliationFindingType   `json:"finding_type"`
	Severity          ReconciliationSeverity      `json:"severity"`
	Risk              ReconciliationRisk          `json:"risk"`
	WalletUserID      *int64                      `json:"-"`
	WalletPublicID    *uuid.UUID                  `json:"wallet_public_id,omitempty"`
	WalletDisplayName string                      `json:"wallet_display_name,omitempty"`
	SubjectType       string                      `json:"subject_type"`
	SubjectID         string                      `json:"subject_id"`
	Details           map[string]any              `json:"details"`
	Status            ReconciliationFindingStatus `json:"status"`
	Isolated          bool                        `json:"isolated"`
	OccurrenceCount   int64                       `json:"occurrence_count"`
	FirstSeenAt       time.Time                   `json:"first_seen_at"`
	LastSeenAt        time.Time                   `json:"last_seen_at"`
	ResolvedAt        *time.Time                  `json:"resolved_at,omitempty"`
}

type ReconciliationRunStatus string
type ReconciliationTrigger string
type ReconciliationFindingStatus string
type ReconciliationFindingType string
type ReconciliationSeverity string
type ReconciliationRisk string

const (
	ReconciliationTriggerManual                         ReconciliationTrigger       = "manual"
	ReconciliationTriggerScheduled                      ReconciliationTrigger       = "scheduled"
	ReconciliationRunQueued                             ReconciliationRunStatus     = "queued"
	ReconciliationRunRunning                            ReconciliationRunStatus     = "running"
	ReconciliationRunCompleted                          ReconciliationRunStatus     = "completed"
	ReconciliationRunFailed                             ReconciliationRunStatus     = "failed"
	ReconciliationFindingOpen                           ReconciliationFindingStatus = "open"
	ReconciliationFindingResolved                       ReconciliationFindingStatus = "resolved"
	ReconciliationFindingLedgerPostingsInvalid          ReconciliationFindingType   = "ledger_postings_invalid"
	ReconciliationFindingCommandShapeInvalid            ReconciliationFindingType   = "command_shape_invalid"
	ReconciliationFindingWalletProjectionMismatch       ReconciliationFindingType   = "wallet_projection_mismatch"
	ReconciliationFindingStaleHold                      ReconciliationFindingType   = "stale_hold"
	ReconciliationFindingTaskHoldMissingBillingSnapshot ReconciliationFindingType   = "task_hold_missing_billing_snapshot"
	ReconciliationFindingTaskTraceMissing               ReconciliationFindingType   = "task_trace_missing"
	ReconciliationFindingTaskBillingFactMissing         ReconciliationFindingType   = "task_billing_fact_missing"
	ReconciliationFindingActiveTaskNotHeld              ReconciliationFindingType   = "active_task_not_held"
	ReconciliationFindingTerminalTaskSettlementMismatch ReconciliationFindingType   = "terminal_task_settlement_mismatch"
	ReconciliationFindingTaskTerminalTransactionCount   ReconciliationFindingType   = "task_terminal_transaction_count"
	ReconciliationFindingIdempotencyRequestHashConflict ReconciliationFindingType   = "idempotency_request_hash_conflict"
	ReconciliationFindingTaskSettlementReferenceInvalid ReconciliationFindingType   = "task_settlement_reference_invalid"
	ReconciliationFindingDuplicateBusinessReference     ReconciliationFindingType   = "duplicate_business_reference"
	ReconciliationFindingCorrectionExceedsOriginal      ReconciliationFindingType   = "correction_amount_exceeds_original"
	ReconciliationFindingRefundAmountInvalid            ReconciliationFindingType   = "refund_amount_invalid"
	ReconciliationFindingIllegalReferenceChain          ReconciliationFindingType   = "illegal_reference_chain"
	ReconciliationFindingHoldStateInvalid               ReconciliationFindingType   = "hold_state_invalid"
	ReconciliationFindingHoldTerminalTransactionCount   ReconciliationFindingType   = "hold_terminal_transaction_count"
	ReconciliationFindingChargeStateInvalid             ReconciliationFindingType   = "charge_state_invalid"
	ReconciliationSeverityWarning                       ReconciliationSeverity      = "warning"
	ReconciliationSeverityError                         ReconciliationSeverity      = "error"
	ReconciliationSeverityCritical                      ReconciliationSeverity      = "critical"
	ReconciliationRiskLow                               ReconciliationRisk          = "low"
	ReconciliationRiskHigh                              ReconciliationRisk          = "high"
)

type ReconciliationRunPage struct {
	Runs         []ReconciliationRun `json:"runs"`
	NextBeforeID int64               `json:"next_before_id,omitempty"`
}

type ReconciliationFindingPage struct {
	Findings     []ReconciliationFinding `json:"findings"`
	NextBeforeID int64                   `json:"next_before_id,omitempty"`
}

type ListReconciliationInput struct {
	BeforeID int64
	Limit    int
}

type StartManualReconciliationInput struct {
	ActorID int64
	Reason  string
}

type Reconciliation interface {
	StartReconciliation(context.Context, StartManualReconciliationInput) (ReconciliationRun, error)
	ScheduleReconciliation(context.Context, time.Time, bool) (*ReconciliationRun, error)
	ExecuteReconciliation(context.Context, int64) error
	FailReconciliation(context.Context, int64, error) error
	GetReconciliationRun(context.Context, int64) (ReconciliationRun, error)
	ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error)
	ListGlobalReconciliationFindings(context.Context, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error)
}
