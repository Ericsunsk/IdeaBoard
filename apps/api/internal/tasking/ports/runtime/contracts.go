package runtime

import (
	"errors"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

var ErrTaskLeaseStateConflict = errors.New("task lease state conflict")

// ActiveTaskExecution is the live execution claim for one task: the
// execution token fenced it, replacing the retired account lease.
type ActiveTaskExecution struct {
	TaskID         string
	ExecutionToken string
	RiverJobID     int64
	FeatureSlug    string
	ResolvedModel  string
	AcquiredAt     time.Time
}

type TaskConvergenceKind string

const (
	TaskConvergenceCompleted       TaskConvergenceKind = "completed"
	TaskConvergenceFailed          TaskConvergenceKind = "failed"
	TaskConvergenceFinalFailed     TaskConvergenceKind = "final_failed"
	TaskConvergenceRecoveredFailed TaskConvergenceKind = "recovered_failed"
	TaskConvergenceRequeued        TaskConvergenceKind = "requeued"
	TaskConvergenceReset           TaskConvergenceKind = "reset"
)

type TaskConvergenceIntent struct {
	Kind   TaskConvergenceKind
	TaskID string
	// ExecutionToken fences terminal writes. Empty means a system-initiated
	// transition that needs no fence.
	ExecutionToken string
	Result         string
	ErrorMessage   string
	OccurredAt     time.Time
	Outputs        []tasks.TaskOutput
	AttemptFinish  *tasks.TaskAttemptFinish
	// Gateway receipt facts recorded on the closed attempt.
	ResolvedModel  string
	GatewayStep    string
	ReceiptLogID   string
	ReceiptEventID string
	CacheHit       bool
}
