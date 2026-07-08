package taskqueue

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestTaskLeaseTokenBindsToRiverJobID(t *testing.T) {
	token, err := newTaskLeaseToken(987)
	if err != nil {
		t.Fatalf("newTaskLeaseToken failed: %v", err)
	}
	if !strings.HasPrefix(token, "river:987:") {
		t.Fatalf("token %q should include river job id prefix", token)
	}
	if !LeaseBelongsToRiverJob(token, 987) {
		t.Fatalf("token %q should belong to river job 987", token)
	}
	if LeaseBelongsToRiverJob(token, 986) {
		t.Fatalf("token %q should not belong to another river job", token)
	}
	if LeaseBelongsToRiverJob("legacy-token", 987) {
		t.Fatal("unscoped lease token should not belong to a river job")
	}
}

func TestPostgresTaskAssignerAssignsLowestLoadRatioAccount(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	highRatio := createAssignerAccount(t, db, "high-ratio", 2)
	lowRatio := createAssignerAccount(t, db, "low-ratio", 4)
	createAssignerTask(t, db, "running-high", highRatio, tasks.StatusRunning, "lease-high")
	createAssignerTask(t, db, "running-low", lowRatio, tasks.StatusRunning, "lease-low")
	createAssignerTask(t, db, "pending-lowest-ratio", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{highRatio, lowRatio}, &tasks.TaskPayload{
		TaskID:     "pending-lowest-ratio",
		RiverJobID: 101,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != lowRatio {
		t.Fatalf("assignment=%+v, want assigned to lowest load ratio account %d", assignment, lowRatio)
	}
	assertTaskLease(t, db, "pending-lowest-ratio", lowRatio, assignment.LeaseToken)
	assertTaskAssignmentMetrics(t, db, "pending-lowest-ratio")
}

func TestPostgresTaskAssignerAssignsNextAccountWhenFirstIsFull(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	full := createAssignerAccount(t, db, "full", 1)
	next := createAssignerAccount(t, db, "next", 3)
	createAssignerTask(t, db, "running-full", full, tasks.StatusRunning, "lease-full")
	createAssignerTask(t, db, "running-next-1", next, tasks.StatusRunning, "lease-next-1")
	createAssignerTask(t, db, "running-next-2", next, tasks.StatusRunning, "lease-next-2")
	createAssignerTask(t, db, "pending-next", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{full, next}, &tasks.TaskPayload{
		TaskID:     "pending-next",
		RiverJobID: 102,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != next {
		t.Fatalf("assignment=%+v, want assigned to non-full account %d", assignment, next)
	}
}

func TestPostgresTaskAssignerIgnoresReleasedLeasesWhenCheckingCapacity(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "released-capacity", 1)
	createAssignerTask(t, db, "released-capacity-load", accountID, tasks.StatusRunning, "")
	insertAssignerTaskLease(t, db, "released-capacity-load", accountID, "lease-released", "", true)
	createAssignerTask(t, db, "pending-released-capacity", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-released-capacity",
		RiverJobID: 112,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want released lease to leave account capacity open", assignment)
	}
	assertTaskLease(t, db, "pending-released-capacity", accountID, assignment.LeaseToken)
}

func TestPostgresTaskAssignerAssignReturnsNoCapacityWhenAllAccountsFull(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	first := createAssignerAccount(t, db, "full-1", 1)
	second := createAssignerAccount(t, db, "full-2", 1)
	createAssignerTask(t, db, "running-full-1", first, tasks.StatusRunning, "lease-full-1")
	createAssignerTask(t, db, "running-full-2", second, tasks.StatusRunning, "lease-full-2")
	createAssignerTask(t, db, "pending-no-capacity", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{first, second}, &tasks.TaskPayload{
		TaskID:     "pending-no-capacity",
		RiverJobID: 103,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity", assignment)
	}
	assertTaskPendingWithoutLease(t, db, "pending-no-capacity")
}

func TestPostgresTaskAssignerAssignReturnsNoCapacityWithoutCandidates(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	createAssignerTask(t, db, "pending-empty-candidates", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{0, 0}, &tasks.TaskPayload{
		TaskID:     "pending-empty-candidates",
		RiverJobID: 109,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity", assignment)
	}
}

func TestPostgresTaskAssignerSkipsDisabledProviderEndpoint(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "disabled-endpoint", 3)
	setAssignerProviderEndpointStatus(t, db, accountID, 0)
	createAssignerTask(t, db, "pending-disabled-endpoint", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-disabled-endpoint",
		RiverJobID: 110,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity when endpoint is disabled", assignment)
	}
}

func TestPostgresTaskAssignerWritesRuntimeAssignmentSnapshot(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "snapshot", 3)
	createAssignerTask(t, db, "pending-snapshot", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-snapshot",
		Params:     `{"feature":"pattern-gen","model":"gpt-image-1"}`,
		RiverJobID: 111,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned {
		t.Fatalf("assignment=%+v, want assigned", assignment)
	}
	assertRuntimeAssignmentSnapshot(t, db, "pending-snapshot", assignment)
}

func TestPostgresTaskAssignerHonorsModelMaxConcurrent(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	modelFull := createAssignerAccount(t, db, "model-full", 3)
	modelOpen := createAssignerAccount(t, db, "model-open", 3)
	setAssignerModelLimit(t, db, modelFull, "gpt-image-1", 1)
	setAssignerModelLimit(t, db, modelOpen, "gpt-image-1", 1)
	createAssignerTaskWithModel(t, db, "running-model-full", modelFull, tasks.StatusRunning, "lease-model-full", "gpt-image-1")
	createAssignerTask(t, db, "pending-model-capacity", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{modelFull, modelOpen}, &tasks.TaskPayload{
		TaskID:     "pending-model-capacity",
		Params:     `{"model":"gpt-image-1"}`,
		RiverJobID: 107,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != modelOpen {
		t.Fatalf("assignment=%+v, want assigned to model-open account %d", assignment, modelOpen)
	}
	if assignment.ResolvedModel != "gpt-image-1" {
		t.Fatalf("assignment resolved_model=%q, want gpt-image-1", assignment.ResolvedModel)
	}
	assertTaskResolvedModel(t, db, "pending-model-capacity", "gpt-image-1")
}

func TestPostgresTaskAssignerDefersBusyUserForCompetingUser(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "fair-user", 3)
	createAssignerTaskWithOptions(t, db, "running-fair-user", accountID, tasks.StatusRunning, "lease-fair-user", "", assignerTaskOptions{
		UserID: 1,
	})
	createAssignerTaskWithOptions(t, db, "pending-busy-user", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now(),
	})
	createAssignerTaskWithOptions(t, db, "pending-competing-user", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    3,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now().Add(-time.Minute),
	})

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-busy-user",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 201,
	})
	if err != nil {
		t.Fatalf("Assign busy user failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity so competing user can dispatch", assignment)
	}
	assertTaskPendingWithoutLease(t, db, "pending-busy-user")

	assignment, err = assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-competing-user",
		UserID:     3,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 202,
	})
	if err != nil {
		t.Fatalf("Assign competing user failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want competing user assigned to account %d", assignment, accountID)
	}
}

func TestPostgresTaskAssignerDefersBusyTenantForCompetingTenant(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	busyTenant := createAssignerAccount(t, db, "busy-tenant", 4)
	competingTenant := createAssignerAccount(t, db, "competing-tenant", 2)
	createAssignerTaskWithOptions(t, db, "running-busy-tenant", busyTenant, tasks.StatusRunning, "lease-busy-tenant", "", assignerTaskOptions{
		UserID: 3,
	})
	createAssignerTaskWithOptions(t, db, "running-competing-tenant", competingTenant, tasks.StatusRunning, "lease-competing-tenant", "", assignerTaskOptions{
		UserID: 3,
	})
	createAssignerTaskWithOptions(t, db, "pending-busy-tenant", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now(),
	})
	createAssignerTaskWithOptions(t, db, "pending-other-tenant", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now().Add(-time.Minute),
	})

	assignment, err := assigner.Assign(ctx, []int64{busyTenant, competingTenant}, &tasks.TaskPayload{
		TaskID:     "pending-busy-tenant",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 203,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity so another tenant can dispatch", assignment)
	}
	assertTaskPendingWithoutLease(t, db, "pending-busy-tenant")
}

func TestPostgresTaskAssignerKeepsPriorityOrderBeforeAgeBoost(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "priority-before-boost", 3)
	createAssignerTaskWithOptions(t, db, "running-priority-before-boost", accountID, tasks.StatusRunning, "lease-priority-before-boost", "", assignerTaskOptions{
		UserID: 1,
	})
	createAssignerTaskWithOptions(t, db, "pending-interactive-before-boost", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now(),
	})
	createAssignerTaskWithOptions(t, db, "pending-default-before-boost", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    3,
		Priority:  tasks.TaskPriorityDefault,
		CreatedAt: time.Now().Add(-taskAssignmentAgeBoostAfter / 2),
	})

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-interactive-before-boost",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 204,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want interactive task to keep priority before age boost", assignment)
	}
}

func TestPostgresTaskAssignerAgeBoostDefersNewerInteractiveForOlderLowerPriority(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "age-boost", 3)
	createAssignerTaskWithOptions(t, db, "running-age-boost", accountID, tasks.StatusRunning, "lease-age-boost", "", assignerTaskOptions{
		UserID: 1,
	})
	createAssignerTaskWithOptions(t, db, "pending-newer-interactive", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now(),
	})
	createAssignerTaskWithOptions(t, db, "pending-older-default", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    3,
		Priority:  tasks.TaskPriorityDefault,
		CreatedAt: time.Now().Add(-taskAssignmentAgeBoostAfter - time.Minute),
	})

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-newer-interactive",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 205,
	})
	if err != nil {
		t.Fatalf("Assign newer interactive failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity for older lower-priority task", assignment)
	}

	assignment, err = assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-older-default",
		UserID:     3,
		Priority:   tasks.TaskPriorityDefault,
		RiverJobID: 206,
	})
	if err != nil {
		t.Fatalf("Assign older default failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want age-boosted lower-priority task assigned", assignment)
	}

	assignment, err = assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-newer-interactive",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 208,
	})
	if err != nil {
		t.Fatalf("Assign retried interactive failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want retried interactive task assigned after older work", assignment)
	}
}

func TestPostgresTaskAssignerDoesNotDeferForCompetingTaskWithoutModelCapacity(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "fair-capacity", 3)
	createAssignerTaskWithOptions(t, db, "running-fair-capacity", accountID, tasks.StatusRunning, "lease-fair-capacity", "", assignerTaskOptions{
		UserID: 1,
	})
	createAssignerTaskWithOptions(t, db, "pending-capacity-current", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:    1,
		Priority:  tasks.TaskPriorityInteractive,
		CreatedAt: time.Now(),
	})
	createAssignerTaskWithOptions(t, db, "pending-no-model-capacity", 0, tasks.StatusPending, "", "", assignerTaskOptions{
		UserID:         3,
		Priority:       tasks.TaskPriorityInteractive,
		CreatedAt:      time.Now().Add(-time.Minute),
		RequestedModel: "missing-model",
	})

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-capacity-current",
		UserID:     1,
		Priority:   tasks.TaskPriorityInteractive,
		RiverJobID: 207,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want current task assigned because competing task has no model capacity", assignment)
	}
}

func TestPostgresTaskAssignerUsesOpenModelWithinSameAccount(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "same-account-model-fallback", 3)
	setAssignerModelLimit(t, db, accountID, "gpt-image-1", 1)
	addAssignerModel(t, db, accountID, "imagen-4", 1, 2)
	createAssignerTaskWithModel(t, db, "running-first-model-full", accountID, tasks.StatusRunning, "lease-first-model-full", "gpt-image-1")
	createAssignerTask(t, db, "pending-same-account-open-model", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "pending-same-account-open-model",
		RiverJobID: 108,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID {
		t.Fatalf("assignment=%+v, want assigned to account %d", assignment, accountID)
	}
	if assignment.ResolvedModel != "imagen-4" {
		t.Fatalf("assignment resolved_model=%q, want imagen-4", assignment.ResolvedModel)
	}
	assertTaskResolvedModel(t, db, "pending-same-account-open-model", "imagen-4")
}

func TestPostgresTaskAssignerReusesRunningLeaseForSameRiverJob(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "reuse", 1)
	createAssignerTask(t, db, "running-reuse", accountID, tasks.StatusRunning, "river:104:existing")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "running-reuse",
		RiverJobID: 104,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusAssigned || assignment.AccountID != accountID || assignment.LeaseToken != "river:104:existing" {
		t.Fatalf("assignment=%+v, want existing running lease", assignment)
	}
}

func TestPostgresTaskAssignerDiscardsRunningTaskForDifferentRiverJob(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "discard", 1)
	createAssignerTask(t, db, "running-discard", accountID, tasks.StatusRunning, "river:105:existing")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &tasks.TaskPayload{
		TaskID:     "running-discard",
		RiverJobID: 106,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusDiscard || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want discard", assignment)
	}
}

func TestPostgresTaskAssignerConfirmsMatchingLease(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "confirm-lease", 1)
	createAssignerTask(t, db, "running-confirm-lease", accountID, tasks.StatusRunning, "lease-confirm")

	err := assigner.ConfirmTaskLease(ctx, TaskAssignment{
		TaskID:             "running-confirm-lease",
		AccountID:          accountID,
		ProviderEndpointID: loadAssignerAccountProviderEndpointID(t, db, accountID),
		CredentialID:       loadAssignerAccountCredentialID(t, db, accountID),
		Status:             AssignStatusAssigned,
		LeaseToken:         "lease-confirm",
	})
	if err != nil {
		t.Fatalf("ConfirmTaskLease failed: %v", err)
	}
}

func TestPostgresTaskAssignerRejectsStaleLease(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "stale-lease", 1)
	createAssignerTask(t, db, "running-stale-lease", accountID, tasks.StatusRunning, "lease-current")

	err := assigner.ConfirmTaskLease(ctx, TaskAssignment{
		TaskID:             "running-stale-lease",
		AccountID:          accountID,
		ProviderEndpointID: loadAssignerAccountProviderEndpointID(t, db, accountID),
		CredentialID:       loadAssignerAccountCredentialID(t, db, accountID),
		Status:             AssignStatusAssigned,
		LeaseToken:         "lease-stale",
	})
	if err == nil {
		t.Fatal("expected stale lease confirmation to fail")
	}
	if !errors.Is(err, ErrTaskLeaseConflict) {
		t.Fatalf("expected ErrTaskLeaseConflict, got %v", err)
	}
}

func TestPostgresTaskAssignerRejectsTaskRowNotRunning(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "confirm-not-running", 1)
	createAssignerTask(t, db, "completed-confirm-lease", accountID, tasks.StatusRunning, "lease-confirm")
	if _, err := db.Exec(`UPDATE tasks SET status = $1 WHERE task_id = $2`, tasks.StatusCompleted, "completed-confirm-lease"); err != nil {
		t.Fatalf("mark task completed failed: %v", err)
	}

	err := assigner.ConfirmTaskLease(ctx, TaskAssignment{
		TaskID:             "completed-confirm-lease",
		AccountID:          accountID,
		ProviderEndpointID: loadAssignerAccountProviderEndpointID(t, db, accountID),
		CredentialID:       loadAssignerAccountCredentialID(t, db, accountID),
		Status:             AssignStatusAssigned,
		LeaseToken:         "lease-confirm",
	})
	if !errors.Is(err, ErrTaskLeaseConflict) {
		t.Fatalf("expected ErrTaskLeaseConflict, got %v", err)
	}
}

func TestPostgresTaskAssignerRejectsTaskRowLeaseMismatch(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "confirm-mismatch", 1)
	createAssignerTask(t, db, "mismatch-confirm-lease", accountID, tasks.StatusRunning, "lease-confirm")
	if _, err := db.Exec(`UPDATE tasks SET assigned_provider_endpoint_id = assigned_provider_endpoint_id + 1000 WHERE task_id = $1`, "mismatch-confirm-lease"); err != nil {
		t.Fatalf("mutate task assignment failed: %v", err)
	}

	err := assigner.ConfirmTaskLease(ctx, TaskAssignment{
		TaskID:             "mismatch-confirm-lease",
		AccountID:          accountID,
		ProviderEndpointID: loadAssignerAccountProviderEndpointID(t, db, accountID),
		CredentialID:       loadAssignerAccountCredentialID(t, db, accountID),
		Status:             AssignStatusAssigned,
		LeaseToken:         "lease-confirm",
	})
	if !errors.Is(err, ErrTaskLeaseConflict) {
		t.Fatalf("expected ErrTaskLeaseConflict, got %v", err)
	}
}

func newPostgresAssignerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	secretstest.EnableEncryption(t)
	return testdb.Open(t)
}

func createAssignerAccount(t *testing.T, db *sql.DB, name string, maxConcurrent int) int64 {
	t.Helper()
	var channelID int64
	if err := db.QueryRow(`
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`, "assigner-channel-"+name, "https://example.com/"+name).Scan(&channelID); err != nil {
		t.Fatalf("create channel %q failed: %v", name, err)
	}
	var id int64
	err := db.QueryRow(`
INSERT INTO api_accounts (name, provider_endpoint_id, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, $3, 120, '', 1, 1)
RETURNING id`, name, channelID, maxConcurrent).Scan(&id)
	if err != nil {
		t.Fatalf("create account %q failed: %v", name, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_account_models (account_id, model_name, enabled, sort_order)
VALUES ($1, 'gpt-image-1', TRUE, 0)`, id); err != nil {
		t.Fatalf("create account model %q failed: %v", name, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_account_secrets (account_id, secret_material_kind, secret_payload, key_version, fingerprint, status)
VALUES ($1, 'encrypted', $2, 'test-key-v1', $3, 'active')`, id, mustEncryptForTest(t, "k"), "fp-"+name); err != nil {
		t.Fatalf("create account secret %q failed: %v", name, err)
	}
	return id
}

func mustEncryptForTest(t *testing.T, plain string) string {
	t.Helper()
	if err := secrets.LoadEncryptionKey(); err != nil {
		t.Fatalf("load test encryption key failed: %v", err)
	}
	encrypted, err := secrets.EncryptString(plain)
	if err != nil {
		t.Fatalf("encrypt test secret failed: %v", err)
	}
	return encrypted
}

func setAssignerModelLimit(t *testing.T, db *sql.DB, accountID int64, modelName string, maxConcurrent int) {
	t.Helper()
	if _, err := db.Exec(`
	UPDATE api_account_models
	SET max_concurrent = $3
	WHERE account_id = $1
	  AND model_name = $2`, accountID, modelName, maxConcurrent); err != nil {
		t.Fatalf("set model limit for account %d failed: %v", accountID, err)
	}
}

func addAssignerModel(t *testing.T, db *sql.DB, accountID int64, modelName string, sortOrder int, maxConcurrent int) {
	t.Helper()
	if _, err := db.Exec(`
	INSERT INTO api_account_models (account_id, model_name, enabled, sort_order, max_concurrent)
	VALUES ($1, $2, TRUE, $3, $4)`, accountID, modelName, sortOrder, maxConcurrent); err != nil {
		t.Fatalf("add model %q for account %d failed: %v", modelName, accountID, err)
	}
}

func loadAssignerAccountProviderEndpointID(t *testing.T, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var channelID int64
	if err := db.QueryRow(`SELECT provider_endpoint_id FROM api_accounts WHERE id = $1`, accountID).Scan(&channelID); err != nil {
		t.Fatalf("load account %d channel failed: %v", accountID, err)
	}
	return channelID
}

func loadAssignerAccountCredentialID(t *testing.T, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var credentialID int64
	if err := db.QueryRow(`
SELECT id
FROM api_account_secrets
WHERE account_id = $1 AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, accountID).Scan(&credentialID); err != nil {
		t.Fatalf("load account %d credential failed: %v", accountID, err)
	}
	return credentialID
}

func setAssignerProviderEndpointStatus(t *testing.T, db *sql.DB, accountID int64, status int) {
	t.Helper()
	if _, err := db.Exec(`
UPDATE provider_endpoints
SET status = $2
WHERE id = (SELECT provider_endpoint_id FROM api_accounts WHERE id = $1)`, accountID, status); err != nil {
		t.Fatalf("set provider endpoint status for account %d failed: %v", accountID, err)
	}
}

func createAssignerTask(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string) {
	t.Helper()
	createAssignerTaskWithModel(t, db, taskID, accountID, status, leaseToken, "")
}

func createAssignerTaskWithModel(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string, resolvedModel string) {
	t.Helper()
	createAssignerTaskWithOptions(t, db, taskID, accountID, status, leaseToken, resolvedModel, assignerTaskOptions{})
}

type assignerTaskOptions struct {
	UserID         int64
	Priority       int
	CreatedAt      time.Time
	FeatureSlug    string
	RequestedModel string
}

func createAssignerTaskWithOptions(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string, resolvedModel string, opts assignerTaskOptions) {
	t.Helper()
	userID := opts.UserID
	if userID == 0 {
		userID = 1
	}
	priority := opts.Priority
	if priority == 0 {
		priority = tasks.TaskPriorityDefault
	}
	createdAt := opts.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err := db.Exec(`
	INSERT INTO tasks (
		task_id, user_id, assigned_provider_endpoint_id, assigned_credential_id, account_id,
		prompt, user_prompt, params, status, lease_token, error_msg, result, resolved_model,
		feature_slug, requested_model, priority, created_at,
		api_type, provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version
	)
	VALUES (
		$1,
		$6,
		(SELECT provider_endpoint_id FROM api_accounts WHERE id = $2),
		(SELECT id FROM api_account_secrets WHERE account_id = $2 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1),
		NULLIF($2, 0),
		'prompt',
		'prompt',
		'{}',
		$3,
		$4,
		'',
		'',
		CASE WHEN NULLIF($5, '') IS NOT NULL THEN $5 WHEN $2 = 0 THEN '' ELSE 'gpt-image-1' END,
		$7,
		$8,
		$9,
		$10,
		COALESCE((SELECT pe.api_type FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $2), ''),
		COALESCE((SELECT pe.name FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $2), ''),
		COALESCE((SELECT pe.base_url FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $2), ''),
		COALESCE((SELECT fingerprint FROM api_account_secrets WHERE account_id = $2 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1), ''),
		COALESCE((SELECT key_version FROM api_account_secrets WHERE account_id = $2 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1), '')
	)`,
		taskID,
		accountID,
		status,
		leaseToken,
		resolvedModel,
		userID,
		opts.FeatureSlug,
		opts.RequestedModel,
		priority,
		createdAt,
	)
	if err != nil {
		t.Fatalf("create task %q failed: %v", taskID, err)
	}
	if status == tasks.StatusRunning && accountID > 0 && leaseToken != "" {
		insertAssignerTaskLease(t, db, taskID, accountID, leaseToken, resolvedModel, false)
	}
}

func insertAssignerTaskLease(t *testing.T, db *sql.DB, taskID string, accountID int64, leaseToken string, resolvedModel string, released bool) {
	t.Helper()
	releasedAtSQL := "NULL"
	releaseReason := ""
	if released {
		releasedAtSQL = "now()"
		releaseReason = "completed"
	}
	_, err := db.Exec(`
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  feature_slug, requested_model, resolved_model, api_type,
  provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version,
  acquired_at, released_at, release_reason
)
VALUES (
  $1, $2, 1, $3,
  (SELECT provider_endpoint_id FROM api_accounts WHERE id = $3),
  (SELECT id FROM api_account_secrets WHERE account_id = $3 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1),
  'image-generate', '',
  CASE WHEN NULLIF($4, '') IS NOT NULL THEN $4 ELSE 'gpt-image-1' END,
  COALESCE((SELECT pe.api_type FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $3), ''),
  COALESCE((SELECT pe.name FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $3), ''),
  COALESCE((SELECT pe.base_url FROM provider_endpoints pe JOIN api_accounts a ON a.provider_endpoint_id = pe.id WHERE a.id = $3), ''),
  COALESCE((SELECT fingerprint FROM api_account_secrets WHERE account_id = $3 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1), ''),
  COALESCE((SELECT key_version FROM api_account_secrets WHERE account_id = $3 AND status = 'active' ORDER BY rotated_at DESC, id DESC LIMIT 1), ''),
  now(), `+releasedAtSQL+`, $5
)`,
		taskID,
		leaseToken,
		accountID,
		resolvedModel,
		releaseReason,
	)
	if err != nil {
		t.Fatalf("insert task lease %q failed: %v", taskID, err)
	}
}

func assertTaskResolvedModel(t *testing.T, db *sql.DB, taskID string, wantModel string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT resolved_model FROM tasks WHERE task_id = $1`, taskID).Scan(&got); err != nil {
		t.Fatalf("load task %q resolved model failed: %v", taskID, err)
	}
	if got != wantModel {
		t.Fatalf("task %q resolved_model=%q, want %q", taskID, got, wantModel)
	}
}

func assertTaskLease(t *testing.T, db *sql.DB, taskID string, wantAccountID int64, wantLeaseToken string) {
	t.Helper()
	var accountID int64
	var assignedProviderEndpointID int64
	var wantProviderEndpointID int64
	var leaseToken string
	var status string
	err := db.QueryRow(`SELECT account_id, assigned_provider_endpoint_id, lease_token, status FROM tasks WHERE task_id = $1`, taskID).Scan(&accountID, &assignedProviderEndpointID, &leaseToken, &status)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if err := db.QueryRow(`SELECT provider_endpoint_id FROM api_accounts WHERE id = $1`, wantAccountID).Scan(&wantProviderEndpointID); err != nil {
		t.Fatalf("load account %d channel failed: %v", wantAccountID, err)
	}
	if accountID != wantAccountID || assignedProviderEndpointID != wantProviderEndpointID || leaseToken != wantLeaseToken || status != tasks.StatusRunning {
		t.Fatalf("task %q state account=%d assigned_channel=%d lease=%q status=%q, want account=%d assigned_channel=%d lease=%q status=%q",
			taskID, accountID, assignedProviderEndpointID, leaseToken, status, wantAccountID, wantProviderEndpointID, wantLeaseToken, tasks.StatusRunning)
	}
}

func assertTaskPendingWithoutLease(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	var status string
	var accountID sql.NullInt64
	var leaseToken string
	err := db.QueryRow(`SELECT status, account_id, lease_token FROM tasks WHERE task_id = $1`, taskID).Scan(&status, &accountID, &leaseToken)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if status != tasks.StatusPending || accountID.Valid || leaseToken != "" {
		t.Fatalf("task %q status=%q account_valid=%v lease=%q, want pending without lease", taskID, status, accountID.Valid, leaseToken)
	}
	var activeLeaseCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_leases WHERE task_id = $1 AND released_at IS NULL`, taskID).Scan(&activeLeaseCount); err != nil {
		t.Fatalf("count task %q active leases failed: %v", taskID, err)
	}
	if activeLeaseCount != 0 {
		t.Fatalf("task %q active leases=%d, want 0", taskID, activeLeaseCount)
	}
}

func assertRuntimeAssignmentSnapshot(t *testing.T, db *sql.DB, taskID string, assignment TaskAssignment) {
	t.Helper()
	var assignedCredentialID sql.NullInt64
	var apiType string
	var providerEndpointName string
	var providerEndpointBaseURL string
	var credentialFingerprint string
	var credentialKeyVersion string
	var resolvedModel string
	err := db.QueryRow(`
SELECT assigned_credential_id, api_type, provider_endpoint_name, provider_endpoint_base_url,
       credential_fingerprint, credential_key_version, resolved_model
FROM tasks
WHERE task_id = $1`, taskID).Scan(
		&assignedCredentialID,
		&apiType,
		&providerEndpointName,
		&providerEndpointBaseURL,
		&credentialFingerprint,
		&credentialKeyVersion,
		&resolvedModel,
	)
	if err != nil {
		t.Fatalf("load task %q runtime assignment snapshot failed: %v", taskID, err)
	}
	if !assignedCredentialID.Valid || assignedCredentialID.Int64 != assignment.CredentialID {
		t.Fatalf("assigned_credential_id=%v, want %d", assignedCredentialID, assignment.CredentialID)
	}
	if apiType != assignment.APIType ||
		providerEndpointName != assignment.ProviderEndpointName ||
		providerEndpointBaseURL != assignment.ProviderEndpointBaseURL ||
		credentialFingerprint != assignment.CredentialFingerprint ||
		credentialKeyVersion != assignment.CredentialKeyVersion ||
		resolvedModel != assignment.ResolvedModel {
		t.Fatalf("snapshot api_type=%q endpoint_name=%q endpoint_base=%q credential_fp=%q key_version=%q model=%q, want assignment=%+v",
			apiType,
			providerEndpointName,
			providerEndpointBaseURL,
			credentialFingerprint,
			credentialKeyVersion,
			resolvedModel,
			assignment,
		)
	}
}

func assertTaskAssignmentMetrics(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	var currentStage string
	var bottleneckSource string
	var queueWaitMS int64
	err := db.QueryRow(`
SELECT current_stage, bottleneck_source, queue_wait_ms
FROM task_execution_metrics
WHERE task_id = $1`, taskID).Scan(&currentStage, &bottleneckSource, &queueWaitMS)
	if err != nil {
		t.Fatalf("load task %q assignment metrics failed: %v", taskID, err)
	}
	if currentStage != tasks.ExecutionStageAssigned || bottleneckSource != tasks.BottleneckSourceQueue || queueWaitMS < 0 {
		t.Fatalf("task %q metrics stage=%q bottleneck=%q queue_wait_ms=%d, want assigned queue non-negative wait",
			taskID, currentStage, bottleneckSource, queueWaitMS)
	}
}
