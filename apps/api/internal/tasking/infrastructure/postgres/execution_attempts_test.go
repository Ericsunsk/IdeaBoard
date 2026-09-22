package postgres

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testdb"
)

var executionAttemptTaskSeq atomic.Int64

// testExecutionAttemptTask seeds a minimal task row for the FK. Account
// fields stay null: attempts never touch the account world.
func testExecutionAttemptTask(t *testing.T, repo *Repository) string {
	t.Helper()
	taskID := fmt.Sprintf("exec-attempt-task-%d", executionAttemptTaskSeq.Add(1))
	if err := repo.Create(t.Context(), &tasks.Task{
		TaskID: taskID,
		UserID: 1,
		Status: tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	return taskID
}

func newExecutionAttemptTestRepo(t *testing.T) *Repository {
	t.Helper()
	db := testdb.Open(t)
	return NewRepository(db)
}

func TestExecutionAttemptValidation(t *testing.T) {
	repo := &Repository{}
	ctx := t.Context()
	if _, err := repo.StartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{}); err == nil {
		t.Fatal("empty start accepted")
	}
	if _, err := repo.FinalizeExecutionAttempt(ctx, tasks.ExecutionAttemptFinish{Status: "running"}); err == nil {
		t.Fatal("running status accepted")
	}
	if _, err := repo.GetExecutionAttemptByToken(ctx, "x"); err == nil {
		t.Fatal("nil repository accepted")
	}
}

func TestExecutionAttemptLifecycle(t *testing.T) {
	repo := newExecutionAttemptTestRepo(t)
	ctx := t.Context()

	taskID := testExecutionAttemptTask(t, repo)
	token := "exec-token-lifecycle"

	id, err := repo.StartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID: taskID, RiverJobID: 7, AttemptNo: 1, ExecutionToken: token,
		FeatureSlug: "render", RequestedModel: "openai/gpt-image-1", Provider: "openai",
		StartedAt: time.Now(),
	})
	if err != nil || id <= 0 {
		t.Fatalf("StartExecutionAttempt() = %d, %v", id, err)
	}
	if _, err := repo.StartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID: taskID, AttemptNo: 2, ExecutionToken: token,
	}); err == nil {
		t.Fatal("duplicate token accepted, want unique violation")
	}
	got, err := repo.GetExecutionAttemptByToken(ctx, token)
	if err != nil || got.Status != "running" || got.TaskID != taskID {
		t.Fatalf("GetExecutionAttemptByToken() = %+v, %v", got, err)
	}
	affected, err := repo.FinalizeExecutionAttempt(ctx, tasks.ExecutionAttemptFinish{
		ExecutionToken: token, TaskID: taskID, Status: "completed",
		ResolvedModel: "openai/gpt-image-1", GatewayStep: "0",
		ReceiptLogID: "log-1", ReceiptEventID: "evt-1",
		FinishedAt: time.Now(), CompletedAt: time.Now(),
	})
	if err != nil || affected != 1 {
		t.Fatalf("FinalizeExecutionAttempt() = %d, %v", affected, err)
	}
	// Second finalize hits a non-running row: zero rows, no error. The
	// caller lost the race and must back off, not retry the write.
	again, err := repo.FinalizeExecutionAttempt(ctx, tasks.ExecutionAttemptFinish{
		ExecutionToken: token, TaskID: taskID, Status: "failed",
		FailureStage: "provider", FailureCategory: "timeout", ErrorMsg: "late",
		FinishedAt: time.Now(), CompletedAt: time.Now(),
	})
	if err != nil || again != 0 {
		t.Fatalf("second finalize = %d, %v, want 0 rows", again, err)
	}
	closed, err := repo.GetExecutionAttemptByToken(ctx, token)
	if err != nil || closed.Status != "completed" {
		t.Fatalf("status = %+v, %v, want completed", closed.Status, err)
	}
}

func TestExecutionAttemptFailedTerminal(t *testing.T) {
	repo := newExecutionAttemptTestRepo(t)
	ctx := t.Context()

	taskID := testExecutionAttemptTask(t, repo)
	token := "exec-token-failed"
	if _, err := repo.StartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID: taskID, AttemptNo: 1, ExecutionToken: token,
	}); err != nil {
		t.Fatalf("StartExecutionAttempt() error = %v", err)
	}
	affected, err := repo.FinalizeExecutionAttempt(ctx, tasks.ExecutionAttemptFinish{
		ExecutionToken: token, TaskID: taskID, Status: "failed",
		FailureStage: "provider", FailureCategory: "timeout", ErrorMsg: "boom",
		FinishedAt: time.Now(), CompletedAt: time.Now(),
	})
	if err != nil || affected != 1 {
		t.Fatalf("FinalizeExecutionAttempt() = %d, %v", affected, err)
	}
	got, err := repo.GetExecutionAttemptByToken(ctx, token)
	if err != nil || got.FailureCategory != "timeout" || !strings.Contains(got.ErrorMsg, "boom") {
		t.Fatalf("failure facts = %+v, %v", got, err)
	}
}
