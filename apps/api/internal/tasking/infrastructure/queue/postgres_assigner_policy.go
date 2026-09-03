package queue

import (
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

type taskAssignmentTaskState struct {
	Params                   string
	Status                   string
	UserID                   int64
	AccountID                int64
	ProviderEndpointID       int64
	CredentialID             int64
	LeaseToken               string
	Priority                 int
	ResolvedModel            string
	APIType                  string
	ProviderEndpointProtocol string
	ProviderEndpointName     string
	ProviderEndpointBaseURL  string
	CredentialFingerprint    string
	CredentialKeyVersion     string
	CreatedAt                time.Time
}

type taskAssignmentSelectedAccount struct {
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
}

type taskAssignmentDecision struct {
	done       bool
	assignment taskqueueports.TaskAssignment
}

type taskAssignmentPolicy struct{}

func (taskAssignmentPolicy) discard(taskID string) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{TaskID: taskID, Status: taskqueueports.AssignStatusDiscard}
}

func (taskAssignmentPolicy) noCapacity(taskID string) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{TaskID: taskID, Status: taskqueueports.AssignStatusNoCapacity}
}

func (taskAssignmentPolicy) assigned(taskID string, selectedAccount taskAssignmentSelectedAccount, leaseToken string) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{
		TaskID:                   taskID,
		AccountID:                selectedAccount.AccountID,
		ProviderEndpointID:       selectedAccount.ProviderEndpointID,
		CredentialID:             selectedAccount.CredentialID,
		ResolvedModel:            selectedAccount.ResolvedModel,
		APIType:                  selectedAccount.APIType,
		ProviderEndpointProtocol: selectedAccount.ProviderEndpointProtocol,
		ProviderEndpointName:     selectedAccount.ProviderEndpointName,
		ProviderEndpointBaseURL:  selectedAccount.ProviderEndpointBaseURL,
		CredentialFingerprint:    selectedAccount.CredentialFingerprint,
		CredentialKeyVersion:     selectedAccount.CredentialKeyVersion,
		Status:                   taskqueueports.AssignStatusAssigned,
		LeaseToken:               leaseToken,
	}
}

func (p taskAssignmentPolicy) decideLockedTask(taskID string, riverJobID int64, state taskAssignmentTaskState) taskAssignmentDecision {
	if tasks.IsTerminalStatus(state.Status) {
		return taskAssignmentDecision{done: true, assignment: p.discard(taskID)}
	}
	if state.Status == tasks.StatusRunning && state.AccountID > 0 && state.ProviderEndpointID > 0 && taskqueueports.LeaseBelongsToRiverJob(state.LeaseToken, riverJobID) {
		return taskAssignmentDecision{
			done: true,
			assignment: p.assigned(taskID, taskAssignmentSelectedAccount{
				AccountID:                state.AccountID,
				ProviderEndpointID:       state.ProviderEndpointID,
				CredentialID:             state.CredentialID,
				ResolvedModel:            state.ResolvedModel,
				APIType:                  state.APIType,
				ProviderEndpointProtocol: state.ProviderEndpointProtocol,
				ProviderEndpointName:     state.ProviderEndpointName,
				ProviderEndpointBaseURL:  state.ProviderEndpointBaseURL,
				CredentialFingerprint:    state.CredentialFingerprint,
				CredentialKeyVersion:     state.CredentialKeyVersion,
			}, state.LeaseToken),
		}
	}
	if state.Status == tasks.StatusRunning {
		return taskAssignmentDecision{done: true, assignment: p.discard(taskID)}
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
