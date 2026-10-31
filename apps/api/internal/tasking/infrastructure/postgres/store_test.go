package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	taskdomain "ideaboard/api/internal/tasking/domain"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTaskTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

func randomHex(t *testing.T, nbytes int) string {
	t.Helper()
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random hex failed: %v", err)
	}
	return hex.EncodeToString(buf)
}

func seedTaskWorkspace(t *testing.T, db *pgxpool.Pool) (workspacePublicID string, ownerUserID int64) {
	t.Helper()
	ctx := context.Background()
	storageUID := "T" + strings.ToUpper(randomHex(t, 3))[:5]
	var userID int64
	if err := db.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('task-test-user', $1, 'user', 1) RETURNING id`,
		storageUID,
	).Scan(&userID); err != nil {
		t.Fatalf("seed task test user failed: %v", err)
	}
	workspacePublicID = "ws_test" + randomHex(t, 6)
	if _, err := db.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'task-test-ws')`,
		workspacePublicID, userID,
	); err != nil {
		t.Fatalf("seed task test workspace failed: %v", err)
	}
	var workspaceInternalID int64
	if err := db.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, workspacePublicID).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve seeded workspace failed: %v", err)
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		workspaceInternalID, userID,
	); err != nil {
		t.Fatalf("seed task test membership failed: %v", err)
	}
	return workspacePublicID, userID
}

func newPendingTask(taskID, workspaceID string, ownerUserID int64) *taskdomain.WorkspaceTask {
	token, err := taskdomain.NewExecutionToken()
	if err != nil {
		panic(err)
	}
	return &taskdomain.WorkspaceTask{
		ID:             taskID,
		WorkspaceID:    workspaceID,
		OwnerUserID:    ownerUserID,
		Service:        "render",
		Params:         map[string]any{"quality": "high"},
		Status:         taskdomain.StatusPending,
		ExecutionToken: token,
		BillingSnapshot: taskdomain.BillingSnapshot{
			SKU: "ai-image:render", DisplayName: "Render",
			PriceVersionID: 1, PriceVersion: 1,
			OutputResolution: "default", UnitCredits: 10, Quantity: 1, TotalCredits: 10,
			SettlementState: "held", HoldTransactionID: "hold_test",
		},
	}
}

// TestWorkspaceTaskLifecycle proves create -> claim -> complete at the
// repository level, including same-token redelivery recovery, lease fencing,
// and output persistence.
func TestWorkspaceTaskLifecycle(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if created.ID != taskID || created.Status != taskdomain.StatusPending {
		t.Fatalf("created = %+v, want pending %s", created, taskID)
	}
	if created.Service != "render" {
		t.Fatalf("service = %q, want canonical render", created.Service)
	}
	if created.BillingSnapshot.TotalCredits != 10 || created.BillingSnapshot.HoldTransactionID != "hold_test" {
		t.Fatalf("billing snapshot did not round-trip: %+v", created.BillingSnapshot)
	}
	if len(created.Params) == 0 {
		t.Fatal("params did not round-trip")
	}

	claimed, err := store.ClaimTask(ctx, workspaceID, taskID, created.ExecutionToken)
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if claimed.Status != taskdomain.StatusRunning {
		t.Fatalf("claimed status = %q, want running", claimed.Status)
	}
	if claimed.ExecutionToken != created.ExecutionToken {
		t.Fatal("claim must preserve the execution token")
	}

	// The same River job may be redelivered after an interruption and must
	// reacquire its own lease. A different token remains fenced out.
	if _, err := store.ClaimTask(ctx, workspaceID, taskID, created.ExecutionToken); err != nil {
		t.Fatalf("same-token redelivery claim failed: %v", err)
	}
	if _, err := store.ClaimTask(ctx, workspaceID, taskID, "another-execution-token"); !errors.Is(err, taskdomain.ErrStateConflict) {
		t.Fatalf("different-token claim err = %v, want state conflict", err)
	}

	checksum := strings.Repeat("c", 64)
	completed, err := store.CompleteTask(ctx, workspaceID, taskID, []taskdomain.OutputRef{
		{Kind: taskdomain.OutputKindRemote, URL: "https://cdn.example.com/o.png", ChecksumSHA256: checksum, ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if completed.Status != taskdomain.StatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("completed = %+v, want completed with timestamp", completed)
	}
	if len(completed.OutputRefs) != 1 || completed.OutputRefs[0].URL != "https://cdn.example.com/o.png" {
		t.Fatalf("outputs did not round-trip: %+v", completed.OutputRefs)
	}

	// Terminal transitions fail closed.
	if _, err := store.CompleteTask(ctx, workspaceID, taskID, nil); !errors.Is(err, taskdomain.ErrStateConflict) {
		t.Fatalf("re-complete err = %v, want state conflict", err)
	}
	if _, err := store.FailTask(ctx, workspaceID, taskID, "boom"); !errors.Is(err, taskdomain.ErrStateConflict) {
		t.Fatalf("fail-after-complete err = %v, want state conflict", err)
	}
	cancelled, already, err := store.CancelTask(ctx, workspaceID, taskID, ownerID, "late")
	if err != nil || !already || cancelled.Status != taskdomain.StatusCompleted {
		t.Fatalf("cancel-after-complete = %+v %v %v, want idempotent replay", cancelled, already, err)
	}
}

func TestWorkspaceTaskCancelAndFail(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	if _, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID)); err != nil {
		t.Fatal(err)
	}
	cancelled, already, err := store.CancelTask(ctx, workspaceID, taskID, ownerID, "no longer needed")
	if err != nil || already {
		t.Fatalf("cancel = %+v %v %v", cancelled, already, err)
	}
	if cancelled.Status != taskdomain.StatusCancelled || cancelled.CompletedAt == nil {
		t.Fatalf("cancelled = %+v, want cancelled with timestamp", cancelled)
	}
	if cancelled.CancelReason != "no longer needed" {
		t.Fatalf("cancel reason = %q, want round-trip", cancelled.CancelReason)
	}
	replay, already, err := store.CancelTask(ctx, workspaceID, taskID, ownerID, "again")
	if err != nil || !already || replay.Status != taskdomain.StatusCancelled {
		t.Fatalf("cancel replay = %+v %v %v", replay, already, err)
	}

	failID := "task_test" + randomHex(t, 6)
	if _, err := store.CreateTask(ctx, newPendingTask(failID, workspaceID, ownerID)); err != nil {
		t.Fatal(err)
	}
	failed, err := store.FailTask(ctx, workspaceID, failID, "provider exploded")
	if err != nil {
		t.Fatalf("FailTask failed: %v", err)
	}
	if failed.Status != taskdomain.StatusFailed || failed.ErrorMsg != "provider exploded" {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestResolveBillingAccountInTxRejectsClosedOrganization(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	tasks := NewStore(db)
	_, ownerID := seedTaskWorkspace(t, db)
	organizations := tenancypg.NewStore(db, nil)
	organization, err := organizations.CreateOrganization(ctx, ownerID, "closed-task-billing")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	workspace, err := organizations.CreateOrganizationWorkspace(ctx, organization.PublicID, ownerID, "task-space")
	if err != nil {
		t.Fatalf("create organization workspace: %v", err)
	}

	resolve := func() (int64, error) {
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)
		return tasks.ResolveBillingAccountInTx(ctx, tx, workspace.PublicID)
	}

	if accountID, err := resolve(); err != nil || accountID <= 0 {
		t.Fatalf("open organization account resolution = %d, %v; want a valid account", accountID, err)
	}
	if _, err := db.Exec(ctx, `UPDATE organizations SET closed_at = now() WHERE internal_id = $1`, organization.InternalID); err != nil {
		t.Fatalf("close organization: %v", err)
	}
	if accountID, err := resolve(); accountID != 0 || !errors.Is(err, taskdomain.ErrNotFound) {
		t.Fatalf("closed organization account resolution = %d, %v; want not found", accountID, err)
	}
}

func TestWorkspaceTaskRetry(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	taskID := "task_test" + randomHex(t, 6)
	created, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask(ctx, workspaceID, taskID, created.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	newToken, err := taskdomain.NewExecutionToken()
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.TransitionForRetry(ctx, workspaceID, taskID, newToken)
	if err != nil {
		t.Fatalf("TransitionForRetry failed: %v", err)
	}
	if retried.Status != taskdomain.StatusPending || retried.RetryCount != 1 {
		t.Fatalf("retried = %+v, want pending retry_count=1", retried)
	}
	if retried.ExecutionToken != newToken {
		t.Fatal("retry must record the fresh execution token")
	}
}

func TestWorkspaceTaskVisibilityAndPaging(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)
	otherWS, _ := seedTaskWorkspace(t, db)

	for i := 0; i < 3; i++ {
		task := newPendingTask(fmt.Sprintf("task_test%s%d", randomHex(t, 4), i), workspaceID, ownerID)
		if _, err := store.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}

	// Cross-workspace id is invisible, never 403.
	some, err := store.ListTasks(ctx, workspaceID, 10, 0)
	if err != nil || len(some) == 0 {
		t.Fatalf("list = %d %v", len(some), err)
	}
	if _, err := store.GetTask(ctx, otherWS, some[0].ID); !errors.Is(err, taskdomain.ErrNotFound) {
		t.Fatalf("cross-workspace get err = %v, want not found", err)
	}
	// Malformed shape is invisible.
	if _, err := store.GetTask(ctx, workspaceID, "nope"); !errors.Is(err, taskdomain.ErrNotFound) {
		t.Fatalf("malformed get err = %v, want not found", err)
	}
	// Unknown workspace is invisible.
	if _, err := store.GetTask(ctx, "ws_missing123", some[0].ID); !errors.Is(err, taskdomain.ErrNotFound) {
		t.Fatalf("unknown workspace get err = %v, want not found", err)
	}

	page, err := store.ListTasks(ctx, workspaceID, 2, 0)
	if err != nil || len(page) != 2 {
		t.Fatalf("page1 = %d %v, want 2", len(page), err)
	}
	rest, err := store.ListTasks(ctx, workspaceID, 2, 2)
	if err != nil || len(rest) != 1 {
		t.Fatalf("page2 = %d %v, want 1", len(rest), err)
	}
	count, err := store.CountTasks(ctx, workspaceID)
	if err != nil || count != 3 {
		t.Fatalf("count = %d %v, want 3", count, err)
	}
	// Newest-first ordering.
	if !page[0].CreatedAt.After(page[1].CreatedAt) && !page[0].CreatedAt.Equal(page[1].CreatedAt) {
		t.Fatal("list must order newest-first")
	}
}

func TestWorkspaceTaskAssetOutputs(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	assetID := "asset_test" + randomHex(t, 6)
	seedMediaAsset(t, db, workspaceID, ownerID, assetID)

	taskID := "task_test" + randomHex(t, 6)
	if _, err := store.CreateTask(ctx, newPendingTask(taskID, workspaceID, ownerID)); err != nil {
		t.Fatal(err)
	}
	updated, err := store.AddOutputs(ctx, workspaceID, taskID, []taskdomain.OutputRef{
		{Kind: taskdomain.OutputKindAsset, AssetID: assetID, Role: "result", ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("AddOutputs failed: %v", err)
	}
	if len(updated.OutputRefs) != 1 || updated.OutputRefs[0].AssetID != assetID {
		t.Fatalf("asset output did not round-trip: %+v", updated.OutputRefs)
	}
	// Unknown asset fails closed.
	if _, err := store.AddOutputs(ctx, workspaceID, taskID, []taskdomain.OutputRef{
		{Kind: taskdomain.OutputKindAsset, AssetID: "asset_missing123", ResultIndex: 1},
	}); !errors.Is(err, taskdomain.ErrMediaNotFound) {
		t.Fatalf("missing asset err = %v, want media not found", err)
	}
}

func seedMediaAsset(t *testing.T, db *pgxpool.Pool, workspacePublicID string, ownerUserID int64, assetPublicID string) {
	t.Helper()
	ctx := context.Background()
	var workspaceInternalID int64
	if err := db.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, workspacePublicID).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve workspace for media seed failed: %v", err)
	}
	uploadPublicID := "upload_test" + randomHex(t, 6)
	var uploadInternalID int64
	if err := db.QueryRow(ctx,
		`INSERT INTO upload_sessions (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, status)
		 VALUES ($1, $2, $3, 'image/png', 1024, 'pending') RETURNING internal_id`,
		uploadPublicID, workspaceInternalID, ownerUserID,
	).Scan(&uploadInternalID); err != nil {
		t.Fatalf("seed upload session failed: %v", err)
	}
	storageKey := "test/tasks/" + randomHex(t, 8)
	if _, err := db.Exec(ctx,
		`INSERT INTO media_assets (public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, metadata)
		 VALUES ($1, $2, $3, $4, $5, 'image/png', 1024, 'deadbeef', 'other', '{}')`,
		assetPublicID, workspaceInternalID, uploadInternalID, ownerUserID, storageKey,
	); err != nil {
		t.Fatalf("seed media asset failed: %v", err)
	}
}

func TestWorkspaceTaskValidation(t *testing.T) {
	db := newTaskTestDB(t)
	ctx := context.Background()
	store := NewStore(db)
	workspaceID, ownerID := seedTaskWorkspace(t, db)

	bad := newPendingTask("task_test"+randomHex(t, 6), workspaceID, ownerID)
	bad.Service = "nope"
	if _, err := store.CreateTask(ctx, bad); !taskdomain.IsValidationError(err) {
		t.Fatalf("unknown kind err = %v, want validation", err)
	}
	// Duplicate public id conflicts.
	dup := newPendingTask("task_test"+randomHex(t, 6), workspaceID, ownerID)
	if _, err := store.CreateTask(ctx, dup); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTask(ctx, dup); !errors.Is(err, taskdomain.ErrStateConflict) {
		t.Fatalf("duplicate err = %v, want state conflict", err)
	}
	if err := store.DeleteTask(ctx, workspaceID, dup.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}
	if _, err := store.GetTask(ctx, workspaceID, dup.ID); !errors.Is(err, taskdomain.ErrNotFound) {
		t.Fatalf("get-after-delete err = %v, want not found", err)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	task := &taskdomain.WorkspaceTask{
		MediaAssetIDs:   []string{"asset_abc"},
		Params:          map[string]any{"prompt": "x"},
		CanonicalParams: `{"schema_version":6,"service":"render"}`,
		// The transient worker handoff must not leak into the durable
		// envelope (#854: no resolved_model key survives marshal).
		ResolvedModel:  "openrouter/fake-image-model",
		ExecutionToken: "exec_abc",
		CancelReason:   "done",
		BillingSnapshot: taskdomain.BillingSnapshot{
			SKU: "ai-image:render", TotalCredits: 10, HoldTransactionID: "hold_1",
		},
	}
	raw, err := marshalEnvelope(task)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := parseEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ExecutionToken != "exec_abc" || envelope.CancelReason != "done" {
		t.Fatalf("envelope = %+v", envelope)
	}
	if envelope.BillingSnapshot == nil || envelope.BillingSnapshot.HoldTransactionID != "hold_1" {
		t.Fatalf("snapshot = %+v", envelope.BillingSnapshot)
	}
	if envelope.CanonicalParams != `{"schema_version":6,"service":"render"}` {
		t.Fatalf("canonical_params = %q, want round-trip", envelope.CanonicalParams)
	}
	if strings.Contains(string(raw), "resolved_model") {
		t.Fatalf("envelope = %s, must not persist a resolved_model key", string(raw))
	}
}

// merged from claim_attempt_test.go (store.go aggregation: ClaimTaskWithAttempt atomicity and marshalEnvelope/mapTaskRow board binding).
// Claim+attempt atomicity over the real fixture schema (DESIGN.md contract
// 4: "claim 与 attempt 行在同一 SQL 语句原子提交，不存在有 token 无 attempt
// 行的中间态"). The guarantee is a PostgreSQL property of the single
// statement, so it is proven here against the real engine and not against a
// fake. SKIPs gracefully without API_TEST_DATABASE_DSN like every other
// fixture-DB test in this package.

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

	claimed, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, lease, taskdomain.AttemptStart{
		RiverJobID:     41,
		Service:        created.Service,
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     taskdomain.AttemptRetryLayerGateway,
	})
	if err != nil {
		t.Fatalf("ClaimTaskWithAttempt failed: %v", err)
	}
	if !started {
		t.Fatal("started = false, want true for a fresh claim")
	}
	if claimed.Status != taskdomain.StatusRunning {
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
	if facts[0].AttemptNo != 1 || facts[0].RetryLayer != taskdomain.AttemptRetryLayerGateway || facts[0].Status != "running" {
		t.Fatalf("attempt fact = %+v, want attempt_no 1 / gateway / running", facts[0])
	}

	// The finish fence closes exactly the row the claim wrote.
	rows, err := store.FinishAttempt(ctx, taskdomain.AttemptFinish{
		ExecutionToken: lease, TaskID: taskID,
		Status:        taskdomain.AttemptStatusCompleted,
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

	if _, _, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, created.ExecutionToken, taskdomain.AttemptStart{
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
	if status != string(taskdomain.StatusPending) {
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
	if _, started, err := store.ClaimTaskWithAttempt(ctx, workspaceID, taskID, created.ExecutionToken, taskdomain.AttemptStart{
		RiverJobID:     7,
		Service:        created.Service,
		RequestedModel: "openrouter/model-a",
		ResolvedModel:  "openrouter/model-a",
		Provider:       "openrouter",
		RetryLayer:     taskdomain.AttemptRetryLayerGateway,
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
	start := taskdomain.AttemptStart{
		RiverJobID: 9, Service: created.Service,
		RequestedModel: "openrouter/model-a", ResolvedModel: "openrouter/model-a",
		Provider: "openrouter", RetryLayer: taskdomain.AttemptRetryLayerGateway,
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
	if claimed.Status != taskdomain.StatusRunning {
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

// merged from envelope_board_test.go (store.go aggregation: ClaimTaskWithAttempt atomicity and marshalEnvelope/mapTaskRow board binding).
func TestEnvelopeRoundTripsBoardBinding(t *testing.T) {
	task := &taskdomain.WorkspaceTask{
		ID: "task_board_env_1", WorkspaceID: "ws_abc", OwnerUserID: 7,
		Service: "render", MediaAssetIDs: []string{"asset_input_1"},
		Params:  map[string]any{"mask_asset_id": "asset_mask_1"},
		BoardID: "board_canvas_1", BoardAccessVersion: 9,
	}
	raw, err := marshalEnvelope(task)
	if err != nil {
		t.Fatalf("marshalEnvelope: %v", err)
	}
	loaded, err := mapTaskRow(task.WorkspaceID, dbsqlc.Task{WorkflowInput: raw}, nil)
	if err != nil {
		t.Fatalf("mapTaskRow: %v", err)
	}
	if loaded.BoardID != "board_canvas_1" {
		t.Fatalf("board_id = %q, want the bound Board", loaded.BoardID)
	}
	if loaded.BoardAccessVersion != 9 {
		t.Fatalf("board_access_version = %d, want the submit-time snapshot", loaded.BoardAccessVersion)
	}
	if len(loaded.MediaAssetIDs) != 1 || loaded.MediaAssetIDs[0] != "asset_input_1" {
		t.Fatalf("media_asset_ids = %v, want the accepted inputs", loaded.MediaAssetIDs)
	}
	if maskID, ok := taskdomain.MaskAssetIDFromParams(loaded.Params); !ok || maskID != "asset_mask_1" {
		t.Fatalf("mask = (%q, %v), want the accepted mask", maskID, ok)
	}
}

func TestEnvelopeWithoutBoardReadsBoardLess(t *testing.T) {
	raw := []byte(`{"media_asset_ids":["asset_input_1"],"params":{"output_resolution":"1k"}}`)
	envelope, err := parseEnvelope(raw)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if envelope.BoardID != "" || envelope.BoardAccessVersion != 0 {
		t.Fatalf("unbound envelope = %+v, want Board-less (submitter-only)", envelope)
	}
}
