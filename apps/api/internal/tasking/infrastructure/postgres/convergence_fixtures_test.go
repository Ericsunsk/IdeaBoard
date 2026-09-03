package postgres

import (
	"context"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func (r *Repository) convergePendingFailureForTest(ctx context.Context, taskID, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFailed, TaskID: taskID, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeFailedWithLeaseForTest(ctx context.Context, taskID string, accountID, providerEndpointID, credentialID int64, leaseToken, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFailed, TaskID: taskID, Lease: &runtimecontracts.TaskLeaseIdentity{AccountID: accountID, ProviderEndpointID: providerEndpointID, CredentialID: credentialID, LeaseToken: leaseToken}, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeResetForTest(ctx context.Context, taskID string, accountID, providerEndpointID, credentialID int64, leaseToken, errorMessage string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceReset, TaskID: taskID, Lease: &runtimecontracts.TaskLeaseIdentity{AccountID: accountID, ProviderEndpointID: providerEndpointID, CredentialID: credentialID, LeaseToken: leaseToken}, ErrorMessage: errorMessage, OccurredAt: attemptFinishTime(attemptFinish), AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeRequeueForTest(ctx context.Context, taskID string, accountID, providerEndpointID, credentialID int64, leaseToken, errorMessage, result string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceRequeued, TaskID: taskID, Lease: &runtimecontracts.TaskLeaseIdentity{AccountID: accountID, ProviderEndpointID: providerEndpointID, CredentialID: credentialID, LeaseToken: leaseToken}, ErrorMessage: errorMessage, Result: result, OccurredAt: attemptFinishTime(attemptFinish), AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeFinalFailureForTest(ctx context.Context, taskID string, accountID, providerEndpointID, credentialID int64, leaseToken, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFinalFailed, TaskID: taskID, Lease: &runtimecontracts.TaskLeaseIdentity{AccountID: accountID, ProviderEndpointID: providerEndpointID, CredentialID: credentialID, LeaseToken: leaseToken}, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}

func attemptFinishTime(finish *tasks.TaskAttemptFinish) time.Time {
	if finish != nil && !finish.CompletedAt.IsZero() {
		return finish.CompletedAt
	}
	return time.Now()
}
