package taskqueue

import (
	"time"

	"ideaboard/api/internal/tasking/tasks"
)

type taskAssignmentTaskState struct {
	Status                  string
	UserID                  int64
	AccountID               int64
	ProviderEndpointID      int64
	CredentialID            int64
	LeaseToken              string
	Priority                int
	ResolvedModel           string
	APIType                 string
	ProviderEndpointName    string
	ProviderEndpointBaseURL string
	CredentialFingerprint   string
	CredentialKeyVersion    string
	CreatedAt               time.Time
}

type taskAssignmentSelectedAccount struct {
	AccountID               int64
	ProviderEndpointID      int64
	CredentialID            int64
	ResolvedModel           string
	APIType                 string
	ProviderEndpointName    string
	ProviderEndpointBaseURL string
	CredentialFingerprint   string
	CredentialKeyVersion    string
}

type taskAssignmentDecision struct {
	done       bool
	assignment TaskAssignment
}

type taskAssignmentPolicy struct{}

func (taskAssignmentPolicy) discard(taskID string) TaskAssignment {
	return TaskAssignment{TaskID: taskID, Status: AssignStatusDiscard}
}

func (taskAssignmentPolicy) noCapacity(taskID string) TaskAssignment {
	return TaskAssignment{TaskID: taskID, Status: AssignStatusNoCapacity}
}

func (taskAssignmentPolicy) assigned(taskID string, selectedAccount taskAssignmentSelectedAccount, leaseToken string) TaskAssignment {
	return TaskAssignment{
		TaskID:                  taskID,
		AccountID:               selectedAccount.AccountID,
		ProviderEndpointID:      selectedAccount.ProviderEndpointID,
		CredentialID:            selectedAccount.CredentialID,
		ResolvedModel:           selectedAccount.ResolvedModel,
		APIType:                 selectedAccount.APIType,
		ProviderEndpointName:    selectedAccount.ProviderEndpointName,
		ProviderEndpointBaseURL: selectedAccount.ProviderEndpointBaseURL,
		CredentialFingerprint:   selectedAccount.CredentialFingerprint,
		CredentialKeyVersion:    selectedAccount.CredentialKeyVersion,
		Status:                  AssignStatusAssigned,
		LeaseToken:              leaseToken,
	}
}

func (p taskAssignmentPolicy) decideLockedTask(taskID string, riverJobID int64, state taskAssignmentTaskState) taskAssignmentDecision {
	if tasks.IsTerminalStatus(state.Status) {
		return taskAssignmentDecision{done: true, assignment: p.discard(taskID)}
	}
	if state.Status == tasks.StatusRunning && state.AccountID > 0 && state.ProviderEndpointID > 0 && LeaseBelongsToRiverJob(state.LeaseToken, riverJobID) {
		return taskAssignmentDecision{
			done: true,
			assignment: p.assigned(taskID, taskAssignmentSelectedAccount{
				AccountID:               state.AccountID,
				ProviderEndpointID:      state.ProviderEndpointID,
				CredentialID:            state.CredentialID,
				ResolvedModel:           state.ResolvedModel,
				APIType:                 state.APIType,
				ProviderEndpointName:    state.ProviderEndpointName,
				ProviderEndpointBaseURL: state.ProviderEndpointBaseURL,
				CredentialFingerprint:   state.CredentialFingerprint,
				CredentialKeyVersion:    state.CredentialKeyVersion,
			}, state.LeaseToken),
		}
	}
	if state.Status == tasks.StatusRunning {
		return taskAssignmentDecision{done: true, assignment: p.discard(taskID)}
	}
	return taskAssignmentDecision{}
}

func (p taskAssignmentPolicy) decideCandidatePool(taskID string, candidates []int64) taskAssignmentDecision {
	if len(candidates) == 0 {
		return taskAssignmentDecision{done: true, assignment: p.noCapacity(taskID)}
	}
	return taskAssignmentDecision{}
}

func (p taskAssignmentPolicy) decideAssignedTask(taskID string, selectedAccount taskAssignmentSelectedAccount, leaseToken string, updatedRows int64) taskAssignmentDecision {
	if updatedRows == 0 {
		return taskAssignmentDecision{done: true, assignment: p.discard(taskID)}
	}
	return taskAssignmentDecision{
		done:       true,
		assignment: p.assigned(taskID, selectedAccount, leaseToken),
	}
}
