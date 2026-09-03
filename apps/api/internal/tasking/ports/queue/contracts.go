package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

func LeaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	if riverJobID <= 0 {
		return false
	}
	return strings.HasPrefix(leaseToken, fmt.Sprintf("river:%d:", riverJobID))
}

const (
	AssignStatusAssigned   = "assigned"
	AssignStatusNoCapacity = "no_capacity"
	AssignStatusDiscard    = "discard"
)

var (
	ErrNoAccountCapacity = errors.New("no account capacity")
	ErrTaskLeaseConflict = errors.New("task lease conflict")
)

type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *tasks.TaskPayload) error
}

type QueueStatsStore interface {
	QueueLengths(ctx context.Context) (global int64, wait int64, err error)
}

type TaskAssigner interface {
	Assign(ctx context.Context, task *tasks.TaskPayload) (TaskAssignment, error)
	ConfirmTaskLease(ctx context.Context, assignment TaskAssignment) error
}

type TaskAssignment struct {
	TaskID                   string
	AccountID                int64
	ProviderEndpointID       int64
	CredentialID             int64
	ResolvedModel            string
	APIType                  string
	ProviderEndpointProtocol string
	ProviderEndpointName     string
	ProviderEndpointBaseURL  string
	CredentialFingerprint    string
	CredentialKeyVersion     string
	Status                   string
	LeaseToken               string
}

func (a TaskAssignment) HasLeaseIdentity() bool {
	return a.TaskID != "" && a.AccountID > 0 && a.ProviderEndpointID > 0 && a.CredentialID > 0 && a.LeaseToken != ""
}

func (a TaskAssignment) HasRuntimeSnapshot() bool {
	return a.HasLeaseIdentity() && a.CredentialID > 0 && a.APIType != "" && a.ProviderEndpointProtocol != "" && a.ProviderEndpointBaseURL != ""
}

type AccountConcurrencyStore interface {
	GetAccountConcurrent(ctx context.Context, accountID int64) (int, error)
	GetAccountMaxConcurrent(ctx context.Context, accountID int64) (int, error)
}

type AccountLoadStore interface {
	GetAccountLoadInfo(ctx context.Context, accountIDs []int64) (map[int64]*tasks.AccountLoad, error)
}

type TaskCancellationStore interface {
	MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error
	IsCancelled(ctx context.Context, taskID string) (bool, error)
	ClearCancelled(ctx context.Context, taskID string) error
}

type TaskMonitorStore interface {
	MonitorInfo(ctx context.Context, accountIDs []int64) (*tasks.MonitorInfo, error)
}
