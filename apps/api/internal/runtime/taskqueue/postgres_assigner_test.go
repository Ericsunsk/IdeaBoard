package taskqueue

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"api/internal/runtime/queuetypes"
	"api/internal/tasks"
	"api/internal/testdb"
)

func TestTaskLeaseTokenBindsToRiverJobID(t *testing.T) {
	token, err := newTaskLeaseToken(987)
	if err != nil {
		t.Fatalf("newTaskLeaseToken failed: %v", err)
	}
	if !strings.HasPrefix(token, "river:987:") {
		t.Fatalf("token %q should include river job id prefix", token)
	}
	if !leaseBelongsToRiverJob(token, 987) {
		t.Fatalf("token %q should belong to river job 987", token)
	}
	if leaseBelongsToRiverJob(token, 986) {
		t.Fatalf("token %q should not belong to another river job", token)
	}
	if leaseBelongsToRiverJob("legacy-token", 987) {
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

	assignment, err := assigner.Assign(ctx, []int64{highRatio, lowRatio}, &queuetypes.TaskPayload{
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

	assignment, err := assigner.Assign(ctx, []int64{full, next}, &queuetypes.TaskPayload{
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

func TestPostgresTaskAssignerAssignReturnsNoCapacityWhenAllAccountsFull(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	first := createAssignerAccount(t, db, "full-1", 1)
	second := createAssignerAccount(t, db, "full-2", 1)
	createAssignerTask(t, db, "running-full-1", first, tasks.StatusRunning, "lease-full-1")
	createAssignerTask(t, db, "running-full-2", second, tasks.StatusRunning, "lease-full-2")
	createAssignerTask(t, db, "pending-no-capacity", 0, tasks.StatusPending, "")

	assignment, err := assigner.Assign(ctx, []int64{first, second}, &queuetypes.TaskPayload{
		TaskID:     "pending-no-capacity",
		RiverJobID: 103,
	})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}
	if assignment.Status != AssignStatusNoCapacity || assignment.AccountID != 0 {
		t.Fatalf("assignment=%+v, want no_capacity", assignment)
	}
}

func TestPostgresTaskAssignerReusesRunningLeaseForSameRiverJob(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	assigner := NewPostgresTaskAssigner(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "reuse", 1)
	createAssignerTask(t, db, "running-reuse", accountID, tasks.StatusRunning, "river:104:existing")

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &queuetypes.TaskPayload{
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

	assignment, err := assigner.Assign(ctx, []int64{accountID}, &queuetypes.TaskPayload{
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

func newPostgresAssignerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func createAssignerAccount(t *testing.T, db *sql.DB, name string, maxConcurrent int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
INSERT INTO api_accounts (name, channel_id, platform, api_type, model_set, api_key, proxy_url, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, 1, 'openai-compatible', 'openai-compatible', 'gpt-image-1', 'k', 'https://example.com', $2, 120, '', 1, 1)
RETURNING id`, name, maxConcurrent).Scan(&id)
	if err != nil {
		t.Fatalf("create account %q failed: %v", name, err)
	}
	return id
}

func createAssignerTask(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string) {
	t.Helper()
	_, err := db.Exec(`
INSERT INTO tasks (task_id, user_id, channel_id, account_id, prompt, user_prompt, params, status, lease_token, error_msg, result)
VALUES ($1, 1, 1, $2, 'prompt', 'prompt', '{}', $3, $4, '', '')`,
		taskID,
		accountID,
		status,
		leaseToken,
	)
	if err != nil {
		t.Fatalf("create task %q failed: %v", taskID, err)
	}
}

func assertTaskLease(t *testing.T, db *sql.DB, taskID string, wantAccountID int64, wantLeaseToken string) {
	t.Helper()
	var accountID int64
	var leaseToken string
	var status string
	err := db.QueryRow(`SELECT account_id, lease_token, status FROM tasks WHERE task_id = $1`, taskID).Scan(&accountID, &leaseToken, &status)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if accountID != wantAccountID || leaseToken != wantLeaseToken || status != tasks.StatusRunning {
		t.Fatalf("task %q state account=%d lease=%q status=%q, want account=%d lease=%q status=%q",
			taskID, accountID, leaseToken, status, wantAccountID, wantLeaseToken, tasks.StatusRunning)
	}
}
