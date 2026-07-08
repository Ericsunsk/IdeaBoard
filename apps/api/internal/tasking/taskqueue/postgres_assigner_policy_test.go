package taskqueue

import (
	"testing"

	"ideaboard/api/internal/tasking/tasks"
)

func TestTaskAssignmentPolicyReusesRunningLeaseForSameRiverJob(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideLockedTask("task-1", 42, taskAssignmentTaskState{
		Status:             tasks.StatusRunning,
		AccountID:          11,
		ProviderEndpointID: 22,
		ResolvedModel:      "gpt-image-1",
		LeaseToken:         "river:42:existing",
	})

	if !decision.done {
		t.Fatal("expected policy to finish with existing lease assignment")
	}
	if decision.assignment.Status != AssignStatusAssigned ||
		decision.assignment.AccountID != 11 ||
		decision.assignment.ProviderEndpointID != 22 ||
		decision.assignment.ResolvedModel != "gpt-image-1" ||
		decision.assignment.LeaseToken != "river:42:existing" {
		t.Fatalf("assignment=%+v, want existing running lease", decision.assignment)
	}
}

func TestTaskAssignmentPolicyDiscardsRunningTaskWithStaleLease(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideLockedTask("task-1", 43, taskAssignmentTaskState{
		Status:             tasks.StatusRunning,
		AccountID:          11,
		ProviderEndpointID: 22,
		LeaseToken:         "river:42:existing",
	})

	if !decision.done || decision.assignment.Status != AssignStatusDiscard {
		t.Fatalf("decision=%+v, want discard", decision)
	}
}

func TestTaskAssignmentPolicyDiscardsTerminalTasks(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideLockedTask("task-1", 42, taskAssignmentTaskState{
		Status: tasks.StatusCompleted,
	})

	if !decision.done || decision.assignment.Status != AssignStatusDiscard {
		t.Fatalf("decision=%+v, want discard", decision)
	}
}

func TestTaskAssignmentPolicyReportsNoCapacityWithoutCandidates(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideCandidatePool("task-1", nil)

	if !decision.done || decision.assignment.Status != AssignStatusNoCapacity {
		t.Fatalf("decision=%+v, want no capacity", decision)
	}
}

func TestTaskAssignmentPolicyContinuesWithCandidateAccounts(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideCandidatePool("task-1", []int64{10})

	if decision.done {
		t.Fatalf("decision=%+v, want assignment to continue to account selection", decision)
	}
}

func TestTaskAssignmentPolicyDiscardsWhenPendingUpdateLosesRace(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideAssignedTask("task-1", taskAssignmentSelectedAccount{
		AccountID:          11,
		ProviderEndpointID: 22,
		ResolvedModel:      "gpt-image-1",
	}, "river:42:new", 0)

	if !decision.done || decision.assignment.Status != AssignStatusDiscard {
		t.Fatalf("decision=%+v, want discard", decision)
	}
}

func TestTaskAssignmentPolicyBuildsSuccessfulAssignment(t *testing.T) {
	policy := taskAssignmentPolicy{}

	decision := policy.decideAssignedTask("task-1", taskAssignmentSelectedAccount{
		AccountID:          11,
		ProviderEndpointID: 22,
		ResolvedModel:      "gpt-image-1",
	}, "river:42:new", 1)

	if !decision.done {
		t.Fatal("expected policy to finish after successful assignment write")
	}
	if decision.assignment.Status != AssignStatusAssigned ||
		decision.assignment.AccountID != 11 ||
		decision.assignment.ProviderEndpointID != 22 ||
		decision.assignment.ResolvedModel != "gpt-image-1" ||
		decision.assignment.LeaseToken != "river:42:new" {
		t.Fatalf("assignment=%+v, want successful assignment", decision.assignment)
	}
}
