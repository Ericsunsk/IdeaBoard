package taskstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
)

func TestCancelRunningTasksForStartupRecoveryConvergesRunningFacts(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 92)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	cancelledAt := time.Now().UTC().Truncate(time.Millisecond)
	reason := "服务重启，任务已停止"

	runningTask := seedRepositoryRunningTaskWithActiveLease(
		t,
		repo,
		db,
		ctx,
		"startup-recovery-running",
		accountID,
		channelID,
		credentialID,
		"lease-startup-recovery-running",
		"lease-startup-recovery-running",
	)
	attempt, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             runningTask.TaskID,
		LeaseToken:         runningTask.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  cancelledAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create running attempt failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_attempts SET created_at = $1 WHERE id = $2`, cancelledAt.Add(-time.Minute), attempt.ID); err != nil {
		t.Fatalf("patch running attempt created_at failed: %v", err)
	}
	pendingTask := &tasks.Task{
		TaskID: "startup-recovery-pending",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	seedTasks(t, repo, ctx, pendingTask)

	recoveredTasks, err := repo.CancelRunningTasksForStartupRecovery(ctx, reason, cancelledAt)
	if err != nil {
		t.Fatalf("CancelRunningTasksForStartupRecovery failed: %v", err)
	}
	if len(recoveredTasks) != 1 || recoveredTasks[0].TaskID != runningTask.TaskID {
		t.Fatalf("cancelled tasks=%v, want task_id=%s", recoveredTasks, runningTask.TaskID)
	}
	if recoveredTasks[0].RiverJobID != 1 {
		t.Fatalf("recovered river_job_id=%d, want 1", recoveredTasks[0].RiverJobID)
	}

	loaded, err := repo.GetByTaskID(ctx, runningTask.TaskID)
	if err != nil {
		t.Fatalf("load cancelled task failed: %v", err)
	}
	if loaded.Status != tasks.StatusCancelled {
		t.Fatalf("status=%q, want %q", loaded.Status, tasks.StatusCancelled)
	}
	if loaded.LeaseToken != "" {
		t.Fatalf("lease_token=%q, want empty", loaded.LeaseToken)
	}
	if loaded.CancelReason != reason {
		t.Fatalf("cancel_reason=%q, want %q", loaded.CancelReason, reason)
	}
	if loaded.CompletedAt == nil || !loaded.CompletedAt.Equal(cancelledAt) {
		t.Fatalf("completed_at=%v, want %v", loaded.CompletedAt, cancelledAt)
	}
	if loaded.CancelledAt == nil || !loaded.CancelledAt.Equal(cancelledAt) {
		t.Fatalf("cancelled_at=%v, want %v", loaded.CancelledAt, cancelledAt)
	}
	if tasks.IDValue(loaded.AccountID) != accountID {
		t.Fatalf("account_id=%d, want %d", tasks.IDValue(loaded.AccountID), accountID)
	}
	if loaded.ExecutionMetrics == nil {
		t.Fatal("execution metrics missing after startup recovery")
	}
	if loaded.ExecutionMetrics.CurrentStage != tasks.ExecutionStageCancelled {
		t.Fatalf("current_stage=%q, want %q", loaded.ExecutionMetrics.CurrentStage, tasks.ExecutionStageCancelled)
	}
	if loaded.ExecutionMetrics.FailureStage != tasks.ExecutionStageAssigned {
		t.Fatalf("failure_stage=%q, want %q", loaded.ExecutionMetrics.FailureStage, tasks.ExecutionStageAssigned)
	}
	if loaded.ExecutionMetrics.FailureCategory != tasks.FailureCategoryCancelled {
		t.Fatalf("failure_category=%q, want %q", loaded.ExecutionMetrics.FailureCategory, tasks.FailureCategoryCancelled)
	}
	assertRepositoryActiveLeaseCount(t, db, runningTask.TaskID, 0)
	assertTaskState(t, repo, ctx, pendingTask.TaskID, tasks.StatusPending, 0, "")

	var releaseReason string
	var releasedAt sql.NullTime
	if err := db.QueryRow(`SELECT release_reason, released_at FROM task_leases WHERE task_id = $1`, runningTask.TaskID).Scan(&releaseReason, &releasedAt); err != nil {
		t.Fatalf("load released lease failed: %v", err)
	}
	if releaseReason != tasks.TaskAttemptStatusCancelled || !releasedAt.Valid {
		t.Fatalf("lease release_reason=%q released_at_valid=%v, want cancelled true", releaseReason, releasedAt.Valid)
	}

	var attemptStatus string
	var failureStage string
	var failureCategory string
	var errorMsg string
	var attemptCompletedAt sql.NullTime
	var attemptCancelledAt sql.NullTime
	var totalLatencyMS int64
	if err := db.QueryRow(`
SELECT status, failure_stage, failure_category, error_msg, completed_at, cancelled_at, total_latency_ms
FROM task_attempts
WHERE id = $1`, attempt.ID).Scan(&attemptStatus, &failureStage, &failureCategory, &errorMsg, &attemptCompletedAt, &attemptCancelledAt, &totalLatencyMS); err != nil {
		t.Fatalf("load attempt failed: %v", err)
	}
	if attemptStatus != tasks.TaskAttemptStatusCancelled {
		t.Fatalf("attempt status=%q, want %q", attemptStatus, tasks.TaskAttemptStatusCancelled)
	}
	if failureStage != tasks.ExecutionStageAssigned || failureCategory != tasks.FailureCategoryCancelled || errorMsg != reason {
		t.Fatalf("attempt failure stage/category/error=%q/%q/%q, want assigned/cancelled/%q", failureStage, failureCategory, errorMsg, reason)
	}
	if attemptCompletedAt.Valid || !attemptCancelledAt.Valid || !attemptCancelledAt.Time.Equal(cancelledAt) {
		t.Fatalf("attempt completed_at=%v cancelled_at=%v, want null/%v", attemptCompletedAt, attemptCancelledAt, cancelledAt)
	}
	if totalLatencyMS != int64(time.Minute/time.Millisecond) {
		t.Fatalf("attempt total_latency_ms=%d, want %d", totalLatencyMS, int64(time.Minute/time.Millisecond))
	}

	var statStatus string
	if err := db.QueryRow(`SELECT status FROM task_stat_events WHERE task_id = $1`, runningTask.TaskID).Scan(&statStatus); err != nil {
		t.Fatalf("load task stat event failed: %v", err)
	}
	if statStatus != tasks.StatusCancelled {
		t.Fatalf("stat status=%q, want %q", statStatus, tasks.StatusCancelled)
	}
}
