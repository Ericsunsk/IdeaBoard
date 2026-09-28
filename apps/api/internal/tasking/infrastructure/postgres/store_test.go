package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/testdb"

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

func newPendingTask(taskID, workspaceID string, ownerUserID int64) *workspacetasks.WorkspaceTask {
	token, err := workspacetasks.NewExecutionToken()
	if err != nil {
		panic(err)
	}
	return &workspacetasks.WorkspaceTask{
		ID:             taskID,
		WorkspaceID:    workspaceID,
		OwnerUserID:    ownerUserID,
		Service:        "render",
		Params:         map[string]any{"quality": "high"},
		Status:         workspacetasks.StatusPending,
		ExecutionToken: token,
		BillingSnapshot: workspacetasks.BillingSnapshot{
			SKU: "ai-image:render", DisplayName: "Render",
			PriceVersionID: 1, PriceVersion: 1,
			OutputResolution: "default", UnitCredits: 10, Quantity: 1, TotalCredits: 10,
			SettlementState: "held", HoldTransactionID: "hold_test",
		},
	}
}

// TestWorkspaceTaskLifecycle proves create -> claim -> complete at the
// repository level, including lease fencing and output persistence.
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
	if created.ID != taskID || created.Status != workspacetasks.StatusPending {
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
	if claimed.Status != workspacetasks.StatusRunning {
		t.Fatalf("claimed status = %q, want running", claimed.Status)
	}
	if claimed.ExecutionToken != created.ExecutionToken {
		t.Fatal("claim must preserve the execution token")
	}

	// Second claim loses the race: the task is no longer pending.
	if _, err := store.ClaimTask(ctx, workspaceID, taskID, created.ExecutionToken); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("re-claim err = %v, want state conflict", err)
	}

	checksum := strings.Repeat("c", 64)
	completed, err := store.CompleteTask(ctx, workspaceID, taskID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindRemote, URL: "https://cdn.example.com/o.png", ChecksumSHA256: checksum, ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if completed.Status != workspacetasks.StatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("completed = %+v, want completed with timestamp", completed)
	}
	if len(completed.OutputRefs) != 1 || completed.OutputRefs[0].URL != "https://cdn.example.com/o.png" {
		t.Fatalf("outputs did not round-trip: %+v", completed.OutputRefs)
	}

	// Terminal transitions fail closed.
	if _, err := store.CompleteTask(ctx, workspaceID, taskID, nil); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("re-complete err = %v, want state conflict", err)
	}
	if _, err := store.FailTask(ctx, workspaceID, taskID, "boom"); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("fail-after-complete err = %v, want state conflict", err)
	}
	cancelled, already, err := store.CancelTask(ctx, workspaceID, taskID, ownerID, "late")
	if err != nil || !already || cancelled.Status != workspacetasks.StatusCompleted {
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
	if cancelled.Status != workspacetasks.StatusCancelled || cancelled.CompletedAt == nil {
		t.Fatalf("cancelled = %+v, want cancelled with timestamp", cancelled)
	}
	if cancelled.CancelReason != "no longer needed" {
		t.Fatalf("cancel reason = %q, want round-trip", cancelled.CancelReason)
	}
	replay, already, err := store.CancelTask(ctx, workspaceID, taskID, ownerID, "again")
	if err != nil || !already || replay.Status != workspacetasks.StatusCancelled {
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
	if failed.Status != workspacetasks.StatusFailed || failed.ErrorMsg != "provider exploded" {
		t.Fatalf("failed = %+v", failed)
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
	newToken, err := workspacetasks.NewExecutionToken()
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.TransitionForRetry(ctx, workspaceID, taskID, newToken)
	if err != nil {
		t.Fatalf("TransitionForRetry failed: %v", err)
	}
	if retried.Status != workspacetasks.StatusPending || retried.RetryCount != 1 {
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
	if _, err := store.GetTask(ctx, otherWS, some[0].ID); !errors.Is(err, workspacetasks.ErrNotFound) {
		t.Fatalf("cross-workspace get err = %v, want not found", err)
	}
	// Malformed shape is invisible.
	if _, err := store.GetTask(ctx, workspaceID, "nope"); !errors.Is(err, workspacetasks.ErrNotFound) {
		t.Fatalf("malformed get err = %v, want not found", err)
	}
	// Unknown workspace is invisible.
	if _, err := store.GetTask(ctx, "ws_missing123", some[0].ID); !errors.Is(err, workspacetasks.ErrNotFound) {
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
	updated, err := store.AddOutputs(ctx, workspaceID, taskID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: assetID, Role: "result", ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("AddOutputs failed: %v", err)
	}
	if len(updated.OutputRefs) != 1 || updated.OutputRefs[0].AssetID != assetID {
		t.Fatalf("asset output did not round-trip: %+v", updated.OutputRefs)
	}
	// Unknown asset fails closed.
	if _, err := store.AddOutputs(ctx, workspaceID, taskID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_missing123", ResultIndex: 1},
	}); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
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
	if _, err := store.CreateTask(ctx, bad); !workspacetasks.IsValidationError(err) {
		t.Fatalf("unknown kind err = %v, want validation", err)
	}
	// Duplicate public id conflicts.
	dup := newPendingTask("task_test"+randomHex(t, 6), workspaceID, ownerID)
	if _, err := store.CreateTask(ctx, dup); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTask(ctx, dup); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("duplicate err = %v, want state conflict", err)
	}
	if err := store.DeleteTask(ctx, workspaceID, dup.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}
	if _, err := store.GetTask(ctx, workspaceID, dup.ID); !errors.Is(err, workspacetasks.ErrNotFound) {
		t.Fatalf("get-after-delete err = %v, want not found", err)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	task := &workspacetasks.WorkspaceTask{
		MediaAssetIDs:   []string{"asset_abc"},
		Params:          map[string]any{"prompt": "x"},
		CanonicalParams: `{"schema_version":6,"service":"render"}`,
		ResolvedModel:   "openrouter/fake-image-model",
		ExecutionToken:  "exec_abc",
		CancelReason:    "done",
		BillingSnapshot: workspacetasks.BillingSnapshot{
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
	if envelope.ResolvedModel != "openrouter/fake-image-model" {
		t.Fatalf("resolved_model = %q, want round-trip", envelope.ResolvedModel)
	}
}
