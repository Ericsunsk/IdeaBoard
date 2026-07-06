package taskqueue

import (
	"context"
	"database/sql"
	"strings"
	"testing"

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
		TaskID:     "running-confirm-lease",
		AccountID:  accountID,
		ChannelID:  loadAssignerAccountChannelID(t, db, accountID),
		Status:     AssignStatusAssigned,
		LeaseToken: "lease-confirm",
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
		TaskID:     "running-stale-lease",
		AccountID:  accountID,
		ChannelID:  loadAssignerAccountChannelID(t, db, accountID),
		Status:     AssignStatusAssigned,
		LeaseToken: "lease-stale",
	})
	if err == nil {
		t.Fatal("expected stale lease confirmation to fail")
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
INSERT INTO channels (type, name, api_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`, "assigner-channel-"+name, "https://example.com/"+name).Scan(&channelID); err != nil {
		t.Fatalf("create channel %q failed: %v", name, err)
	}
	var id int64
	err := db.QueryRow(`
INSERT INTO api_accounts (name, channel_id, platform, api_type, proxy_url, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, 'openai-compatible', 'openai-compatible', 'https://example.com', $3, 120, '', 1, 1)
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
INSERT INTO api_account_secrets (account_id, secret_material_kind, secret_payload, status)
VALUES ($1, 'encrypted', $2, 'active')`, id, mustEncryptForTest(t, "k")); err != nil {
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

func loadAssignerAccountChannelID(t *testing.T, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var channelID int64
	if err := db.QueryRow(`SELECT channel_id FROM api_accounts WHERE id = $1`, accountID).Scan(&channelID); err != nil {
		t.Fatalf("load account %d channel failed: %v", accountID, err)
	}
	return channelID
}

func createAssignerTask(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string) {
	t.Helper()
	createAssignerTaskWithModel(t, db, taskID, accountID, status, leaseToken, "")
}

func createAssignerTaskWithModel(t *testing.T, db *sql.DB, taskID string, accountID int64, status string, leaseToken string, resolvedModel string) {
	t.Helper()
	_, err := db.Exec(`
	INSERT INTO tasks (task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params, status, lease_token, error_msg, result, resolved_model)
	VALUES ($1, 1, (SELECT channel_id FROM api_accounts WHERE id = $2), NULLIF($2, 0), 'prompt', 'prompt', '{}', $3, $4, '', '', $5)`,
		taskID,
		accountID,
		status,
		leaseToken,
		resolvedModel,
	)
	if err != nil {
		t.Fatalf("create task %q failed: %v", taskID, err)
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
	var assignedChannelID int64
	var wantChannelID int64
	var leaseToken string
	var status string
	err := db.QueryRow(`SELECT account_id, assigned_channel_id, lease_token, status FROM tasks WHERE task_id = $1`, taskID).Scan(&accountID, &assignedChannelID, &leaseToken, &status)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if err := db.QueryRow(`SELECT channel_id FROM api_accounts WHERE id = $1`, wantAccountID).Scan(&wantChannelID); err != nil {
		t.Fatalf("load account %d channel failed: %v", wantAccountID, err)
	}
	if accountID != wantAccountID || assignedChannelID != wantChannelID || leaseToken != wantLeaseToken || status != tasks.StatusRunning {
		t.Fatalf("task %q state account=%d assigned_channel=%d lease=%q status=%q, want account=%d assigned_channel=%d lease=%q status=%q",
			taskID, accountID, assignedChannelID, leaseToken, status, wantAccountID, wantChannelID, wantLeaseToken, tasks.StatusRunning)
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
