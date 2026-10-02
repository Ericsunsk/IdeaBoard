package postgres

// Claim+attempt atomicity over the real fixture schema (DESIGN.md contract
// 4: "claim 与 attempt 行在同一 SQL 语句原子提交，不存在有 token 无 attempt
// 行的中间态"). The guarantee is a PostgreSQL property of the single
// statement, so it is proven here against the real engine and not against a
// fake. SKIPs gracefully without API_TEST_DATABASE_DSN like every other
// fixture-DB test in this package.

import (
	"context"
	"reflect"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5/pgxpool"
)

// attemptFact is the durable send fact the claim statement must write.
type attemptFact struct {
	AttemptNo      int32
	ExecutionToken string
	RetryLayer     string
	Status         string
}

func readAttemptFacts(t *testing.T, db *pgxpool.Pool, taskID string) []attemptFact {
	t.Helper()
	rows, err := db.Query(context.Background(),
		`SELECT attempt_no, execution_token, retry_layer, status
		 FROM task_execution_attempts WHERE task_id = $1 ORDER BY attempt_no`, taskID)
	if err != nil {
		t.Fatalf("read attempt facts: %v", err)
	}
	defer rows.Close()
	var out []attemptFact
	for rows.Next() {
		var f attemptFact
		if err := rows.Scan(&f.AttemptNo, &f.ExecutionToken, &f.RetryLayer, &f.Status); err != nil {
			t.Fatalf("scan attempt fact: %v", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate attempt facts: %v", err)
	}
	return out
}

func readTaskRow(t *testing.T, db *pgxpool.Pool, taskID string) (status, leaseToken string) {
	t.Helper()
	if err := db.QueryRow(context.Background(),
		`SELECT status, lease_token FROM tasks WHERE public_id = $1`, taskID).
		Scan(&status, &leaseToken); err != nil {
		t.Fatalf("read task row: %v", err)
	}
	return status, leaseToken
}

// TestClaimTaskWithAttemptCommitsLeaseAndFactAtomically proves the positive
// contract: a successful claim has BOTH the running lease and its attempt
// fact, and the fact carries the lease token itself (one token per execution,
// not the two the split claim/StartAttempt pair used to mint).
func TestClaimTaskWithAttemptCommitsLeaseAndFactAtomically(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	lease := created.ExecutionToken

	claimed, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, lease, workspacetasks.AttemptStart{
		RiverJobID:     41,
		Service:        created.Service,
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     workspacetasks.AttemptRetryLayerGateway,
	})
	if err != nil {
		t.Fatalf("ClaimTaskWithAttempt failed: %v", err)
	}
	if !started {
		t.Fatal("started = false, want true for a fresh claim")
	}
	if claimed.Status != workspacetasks.StatusRunning {
		t.Fatalf("claimed status = %q, want running", claimed.Status)
	}

	// The contract: the attempt row exists in the same commit as the lease.
	facts := readAttemptFacts(t, db, taskID)
	if len(facts) != 1 {
		t.Fatalf("attempt facts = %d, want exactly 1 (no token-without-fact gap)", len(facts))
	}
	if facts[0].ExecutionToken != lease {
		t.Fatalf("attempt execution_token = %q, want the lease token %q", facts[0].ExecutionToken, lease)
	}
	if facts[0].AttemptNo != 1 || facts[0].RetryLayer != workspacetasks.AttemptRetryLayerGateway || facts[0].Status != "running" {
		t.Fatalf("attempt fact = %+v, want attempt_no 1 / gateway / running", facts[0])
	}

	// The finish fence closes exactly the row the claim wrote.
	rows, err := store.FinishAttempt(ctx, workspacetasks.AttemptFinish{
		ExecutionToken: lease, TaskID: taskID,
		Status:        workspacetasks.AttemptStatusCompleted,
		ResolvedModel: "openrouter/model-a",
	})
	if err != nil || rows != 1 {
		t.Fatalf("FinishAttempt by lease token = (%d, %v), want 1/nil", rows, err)
	}
}

// TestClaimTaskWithAttemptRollsBackLeaseWhenAttemptInsertFails proves the
// negative contract, which is the whole point of the merged statement: the
// attempt INSERT is forced to fail (retry_layer outside the
// ck_task_execution_attempts_retry_layer CHECK, so the error is raised by
// PostgreSQL AFTER the lease UPDATE matched) and the lease flip must roll back
// with it. The split two-statement path could not do this: the lease would
// already be committed, leaving the forbidden "running token, zero attempt
// rows" state behind.
func TestClaimTaskWithAttemptRollsBackLeaseWhenAttemptInsertFails(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if _, _, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, created.ExecutionToken, workspacetasks.AttemptStart{
		RiverJobID:     7,
		Service:        created.Service,
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     "not_a_real_layer",
	}); err == nil {
		t.Fatal("claim with an invalid retry_layer succeeded, want the attempt INSERT to fail")
	}

	status, lease := readTaskRow(t, db, taskID)
	if status != string(workspacetasks.StatusPending) {
		t.Fatalf("task status = %q, want pending (the lease flip rolled back)", status)
	}
	if lease != "" {
		t.Fatalf("lease_token = %q, want empty (no committed token without its attempt row)", lease)
	}
	if facts := readAttemptFacts(t, db, taskID); len(facts) != 0 {
		t.Fatalf("attempt facts = %d, want 0 after the rolled-back claim", len(facts))
	}

	// The claim is retryable, and the retry succeeds: the fence never left
	// the task wedged.
	if _, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, created.ExecutionToken, workspacetasks.AttemptStart{
		RiverJobID:     7,
		Service:        created.Service,
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     workspacetasks.AttemptRetryLayerGateway,
	}); err != nil || !started {
		t.Fatalf("retry claim = (%v, started=%v), want success", err, started)
	}
	if facts := readAttemptFacts(t, db, taskID); len(facts) != 1 || facts[0].AttemptNo != 1 {
		t.Fatalf("attempt facts = %+v, want one attempt_no 1 fact (the rolled-back insert consumed no number)", facts)
	}
}

// TestClaimTaskWithAttemptSameTokenResumesOneFact pins the redelivery
// branch: a River redelivery re-acquires its own lease, but it must NOT open
// a second attempt fact (uq_task_execution_attempts_token makes the token the
// execution's identity). The resumed execution closes the SAME row, and no
// attempt number is consumed.
func TestClaimTaskWithAttemptSameTokenResumesOneFact(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	lease := created.ExecutionToken
	start := workspacetasks.AttemptStart{
		RiverJobID: 9, Service: created.Service,
		RequestedModel: "openrouter/model-a", ResolvedModel: "openrouter/model-a",
		Provider: "openrouter", RetryLayer: workspacetasks.AttemptRetryLayerGateway,
	}
	if _, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, lease, start); err != nil || !started {
		t.Fatalf("first claim = (%v, started=%v)", err, started)
	}

	// Same token: the interrupted lease is reacquired and reported as a
	// resume, not a new send fact.
	claimed, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, lease, start)
	if err != nil {
		t.Fatalf("same-token redelivery claim failed: %v", err)
	}
	if started {
		t.Fatal("started = true, want false for a resume of an already-recorded lease")
	}
	if claimed.Status != workspacetasks.StatusRunning {
		t.Fatalf("resumed status = %q, want running", claimed.Status)
	}
	if facts := readAttemptFacts(t, db, taskID); len(facts) != 1 {
		t.Fatalf("attempt facts = %d, want 1 (one token, one fact)", len(facts))
	}

	// A different token stays fenced out entirely (no lease, no fact).
	if _, _, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, "exec_someone_else", start); err == nil {
		t.Fatal("different-token claim succeeded, want the lease fence to reject it")
	}
	if facts := readAttemptFacts(t, db, taskID); len(facts) != 1 {
		t.Fatalf("attempt facts = %d, want 1 (a fenced-out claim writes nothing)", len(facts))
	}
}

// TestClaimRowToTaskPreservesEveryTaskColumn guards the narrowing the merged
// row needs: the statement appends attempt_no/attempt_started to the task
// columns, so a column added to tasks but not threaded through claimRowToTask
// would silently read back as a zero value.
func TestClaimRowToTaskPreservesEveryTaskColumn(t *testing.T) {
	taskType := reflect.TypeOf(dbsqlc.Task{})
	if got, want := reflect.TypeOf(claimRowToTask(dbsqlc.ClaimWorkspaceTaskWithAttemptRow{})), taskType; got != want {
		t.Fatalf("claimRowToTask returns %s, want %s", got, want)
	}
	claimRowType := reflect.TypeOf(dbsqlc.ClaimWorkspaceTaskWithAttemptRow{})
	for i := 0; i < taskType.NumField(); i++ {
		field := taskType.Field(i)
		claimField, ok := claimRowType.FieldByName(field.Name)
		if !ok {
			t.Fatalf("claim row is missing tasks.%s", field.Name)
		}
		if claimField.Type != field.Type {
			t.Fatalf("claim row field %s is %s, want %s", field.Name, claimField.Type, field.Type)
		}
	}
}
