package execution

import (
	"errors"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
)

// TestEvaluateTaskTransitionCurrentContract documents the state-machine
// boundary without coupling the tests to persistence or River jobs.
func TestEvaluateTaskTransitionCurrentContract(t *testing.T) {
	validAssignment := taskTransitionTestAssignment("transition-contract-task")
	missingLease := taskqueueports.TaskAssignment{TaskID: validAssignment.TaskID}

	tests := []struct {
		name           string
		transition     taskTransition
		status         string
		assignment     taskqueueports.TaskAssignment
		wantReady      bool
		wantIdempotent bool
		wantErr        error
	}{
		{name: "complete from running", transition: taskTransitionComplete, status: tasks.StatusRunning, assignment: validAssignment, wantReady: true},
		{name: "complete requires lease", transition: taskTransitionComplete, status: tasks.StatusRunning, assignment: missingLease, wantErr: ErrTaskStateConflict},
		{name: "complete rejects terminal", transition: taskTransitionComplete, status: tasks.StatusCompleted, assignment: validAssignment, wantErr: ErrTaskStateConflict},
		{name: "fail pending from pending", transition: taskTransitionFailPending, status: tasks.StatusPending, assignment: missingLease, wantReady: true},
		{name: "fail pending is idempotent", transition: taskTransitionFailPending, status: tasks.StatusFailed, assignment: missingLease, wantIdempotent: true},
		{name: "fail pending rejects running", transition: taskTransitionFailPending, status: tasks.StatusRunning, assignment: missingLease, wantErr: ErrTaskStateConflict},
		{name: "fail running from running", transition: taskTransitionFailRunning, status: tasks.StatusRunning, assignment: validAssignment, wantReady: true},
		{name: "fail running requires lease", transition: taskTransitionFailRunning, status: tasks.StatusRunning, assignment: missingLease, wantErr: ErrTaskStateConflict},
		{name: "fail running rejects terminal", transition: taskTransitionFailRunning, status: tasks.StatusFailed, assignment: validAssignment, wantErr: ErrTaskStateConflict},
		{name: "reset from running", transition: taskTransitionReset, status: tasks.StatusRunning, assignment: validAssignment, wantReady: true},
		{name: "reset is idempotent", transition: taskTransitionReset, status: tasks.StatusPending, assignment: missingLease, wantIdempotent: true},
		{name: "reset requires lease", transition: taskTransitionReset, status: tasks.StatusRunning, assignment: missingLease, wantErr: ErrTaskStateConflict},
		{name: "requeue from running", transition: taskTransitionRequeue, status: tasks.StatusRunning, assignment: validAssignment, wantReady: true},
		{name: "requeue is idempotent", transition: taskTransitionRequeue, status: tasks.StatusPending, assignment: missingLease, wantIdempotent: true},
		{name: "requeue rejects stale terminal", transition: taskTransitionRequeue, status: tasks.StatusFailed, assignment: validAssignment, wantErr: ErrTaskStateConflict},
		{name: "final fail from running", transition: taskTransitionFinalFail, status: tasks.StatusRunning, assignment: validAssignment, wantReady: true},
		{name: "final fail is idempotent after requeue", transition: taskTransitionFinalFail, status: tasks.StatusPending, assignment: missingLease, wantIdempotent: true},
		{name: "final fail rejects stale terminal", transition: taskTransitionFinalFail, status: tasks.StatusFailed, assignment: validAssignment, wantErr: ErrTaskStateConflict},
		{name: "recovery fail from pending", transition: taskTransitionRecoveryFail, status: tasks.StatusPending, assignment: missingLease, wantReady: true},
		{name: "recovery fail from running", transition: taskTransitionRecoveryFail, status: tasks.StatusRunning, assignment: missingLease, wantReady: true},
		{name: "recovery fail is idempotent", transition: taskTransitionRecoveryFail, status: tasks.StatusFailed, assignment: missingLease, wantIdempotent: true},
		{name: "recovery fail terminal is not stale conflict", transition: taskTransitionRecoveryFail, status: tasks.StatusCompleted, assignment: missingLease, wantErr: ErrTaskTerminal},
		{name: "cancel from pending", transition: taskTransitionCancel, status: tasks.StatusPending, assignment: missingLease, wantReady: true},
		{name: "cancel from running", transition: taskTransitionCancel, status: tasks.StatusRunning, assignment: missingLease, wantReady: true},
		{name: "cancel is idempotent", transition: taskTransitionCancel, status: tasks.StatusCancelled, assignment: missingLease, wantIdempotent: true},
		{name: "cancel with results is idempotent", transition: taskTransitionCancel, status: tasks.StatusCancelledWithResults, assignment: missingLease, wantIdempotent: true},
		{name: "cancel completed task is terminal", transition: taskTransitionCancel, status: tasks.StatusCompleted, assignment: missingLease, wantErr: ErrTaskTerminal},
		{name: "unknown transition conflicts", transition: taskTransition("unknown"), status: tasks.StatusPending, assignment: missingLease, wantErr: ErrTaskStateConflict},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := evaluateTaskTransitionCurrent(tc.transition, taskTransitionTestTask(tc.status), tc.assignment)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v, want %v", err, tc.wantErr)
			}
			if result.ready != tc.wantReady || result.idempotent != tc.wantIdempotent {
				t.Fatalf("result=%+v, want ready=%v idempotent=%v", result, tc.wantReady, tc.wantIdempotent)
			}
		})
	}
}

func TestTaskTransitionRulesMaintainStateInvariants(t *testing.T) {
	validStatuses := map[string]bool{
		tasks.StatusPending: true, tasks.StatusRunning: true,
		tasks.StatusCompleted: true, tasks.StatusFailed: true,
		tasks.StatusCancelled: true, tasks.StatusCancelledWithResults: true,
	}
	for transition, rule := range taskTransitionRules {
		t.Run(string(transition), func(t *testing.T) {
			if !validStatuses[rule.target] {
				t.Fatalf("target status %q is not a known task status", rule.target)
			}
			if len(rule.from) == 0 {
				t.Fatal("transition must declare at least one source status")
			}
			for _, status := range append(append([]string{}, rule.from...), rule.idempotent...) {
				if !validStatuses[status] {
					t.Fatalf("source/idempotent status %q is not a known task status", status)
				}
			}
			if rule.requiresLease {
				for _, status := range rule.from {
					if status != tasks.StatusRunning {
						t.Fatalf("lease-owned transition has non-running source %q", status)
					}
				}
			}
		})
	}
}

func TestTaskTransitionLeaseIdentityContract(t *testing.T) {
	assignment := taskTransitionTestAssignment("lease-contract-task")
	if !assignment.HasLeaseIdentity() {
		t.Fatal("fixture assignment must have lease identity")
	}
	if !taskMatchesLeaseAssignment(taskTransitionTestTask(tasks.StatusRunning), assignment) {
		t.Fatal("matching task and assignment should pass lease identity check")
	}
	for name, mutate := range map[string]func(*taskqueueports.TaskAssignment){
		"account":    func(a *taskqueueports.TaskAssignment) { a.AccountID++ },
		"endpoint":   func(a *taskqueueports.TaskAssignment) { a.ProviderEndpointID++ },
		"credential": func(a *taskqueueports.TaskAssignment) { a.CredentialID++ },
		"token":      func(a *taskqueueports.TaskAssignment) { a.LeaseToken = "stale" },
	} {
		t.Run(name, func(t *testing.T) {
			stale := assignment
			mutate(&stale)
			if taskMatchesLeaseAssignment(taskTransitionTestTask(tasks.StatusRunning), stale) {
				t.Fatal("mismatched lease identity should not match")
			}
		})
	}
	if taskMatchesLeaseAssignment(nil, assignment) {
		t.Fatal("nil task should never match a lease assignment")
	}
}
