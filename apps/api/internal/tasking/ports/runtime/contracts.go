package runtime

import (
	"errors"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

var ErrTaskLeaseStateConflict = errors.New("task lease state conflict")

type ActiveTaskLease struct {
	TaskID                   string
	LeaseToken               string
	RiverJobID               int64
	AccountID                int64
	ProviderEndpointID       int64
	CredentialID             int64
	FeatureSlug              string
	ResolvedModel            string
	APIType                  string
	ProviderEndpointProtocol string
	ProviderEndpointName     string
	ProviderEndpointBaseURL  string
	CredentialFingerprint    string
	CredentialKeyVersion     string
	AcquiredAt               time.Time
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

type TaskLeaseIdentity struct {
	AccountID          int64
	ProviderEndpointID int64
	CredentialID       int64
	LeaseToken         string
}

type TaskConvergenceIntent struct {
	Kind          TaskConvergenceKind
	TaskID        string
	Lease         *TaskLeaseIdentity
	Result        string
	ErrorMessage  string
	OccurredAt    time.Time
	Outputs       []tasks.TaskOutput
	AttemptFinish *tasks.TaskAttemptFinish
}
