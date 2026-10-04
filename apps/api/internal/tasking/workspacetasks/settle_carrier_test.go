// Package workspacetasks_test proves the settlement joins the ambient
// guard transaction (control audit atomicity, step 3).
//
// What is proven here, against the isolated fixture database:
//   - Cancel on a carrier context settles inside the carrier: the
//     terminal write is a real INSERT on the carrier transaction,
//     readable from the still-open carrier and invisible on the pool,
//     then durable after the carrier commits. The store's own
//     BeginSettlementTx is never called (no nested transaction).
//   - A carrier rollback leaves no settlement artifact behind (the
//     step-2 gap: the settlement commit used to survive on its own).
//   - With no carrier the settlement-transactor path runs unchanged
//     (settle_atomic_test.go is that proof; it is not duplicated here).
package workspacetasks_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

// carrierSettleStore is a tx-observing settlement fake: every seam the
// cancel path needs is present, but the terminal flip performs a REAL
// INSERT on the transaction it is handed, so the test observes exactly
// which transaction the settlement joined.
type carrierSettleStore struct {
	t                *testing.T
	probe            string
	beginCalled      bool
	cancelCalls      int
	lastTxNil        bool
	cancelledTaskIDs []string
}

func (f *carrierSettleStore) BeginSettlementTx(context.Context) (pgx.Tx, error) {
	f.beginCalled = true
	return nil, fmt.Errorf("carrier settle store: settlement must join the carrier, never begin its own transaction")
}

func (f *carrierSettleStore) GetTask(_ context.Context, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	return &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, Status: workspacetasks.StatusRunning}, nil
}

func (f *carrierSettleStore) ListTasks(_ context.Context, _ string, _, _ int) ([]*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) CountTasks(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (f *carrierSettleStore) CreateTask(_ context.Context, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	return task, nil
}

func (f *carrierSettleStore) CancelTask(_ context.Context, _, _ string, _ int64, _ string) (*workspacetasks.WorkspaceTask, bool, error) {
	f.t.Fatal("non-transactional CancelTask must not run when the tx seams are wired")
	return nil, false, nil
}

func (f *carrierSettleStore) TransitionForRetry(_ context.Context, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) CompleteTask(_ context.Context, _, _ string, _ []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) FailTask(_ context.Context, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) DeleteTask(_ context.Context, _, _ string) error { return nil }

func (f *carrierSettleStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, _ int64, _ string) (*workspacetasks.WorkspaceTask, bool, error) {
	f.cancelCalls++
	if tx == nil {
		f.lastTxNil = true
		return nil, false, fmt.Errorf("carrier settle store: terminal flip requires a transaction")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+f.probe+` (id, note) VALUES ($1, 'cancelled')`, taskID); err != nil {
		return nil, false, err
	}
	f.cancelledTaskIDs = append(f.cancelledTaskIDs, taskID)
	// An empty billing snapshot keeps the holder out of this proof: the
	// terminal task write alone decides which transaction settled.
	return &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, OwnerUserID: 7, Status: workspacetasks.StatusCancelled}, false, nil
}

func (f *carrierSettleStore) CompleteTaskInTx(_ context.Context, _ pgx.Tx, _, _ string, _ []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) FailTaskInTx(_ context.Context, _ pgx.Tx, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) TransitionForRetryInTx(_ context.Context, _ pgx.Tx, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) CreateTaskInTx(_ context.Context, _ pgx.Tx, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	return task, nil
}

func (f *carrierSettleStore) GetTaskInTx(_ context.Context, _ pgx.Tx, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	return &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID}, nil
}

// carrierSettleHolder satisfies the holder seams without touching the
// ledger: the fabricated task carries no hold, so the holder is never
// called in this proof.
type carrierSettleHolder struct{}

func (carrierSettleHolder) Hold(_ context.Context, _ int64, _ int64, _, _ string) (string, error) {
	return "", nil
}

func (carrierSettleHolder) Capture(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (carrierSettleHolder) Release(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (carrierSettleHolder) HoldInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, _, _ string) (string, error) {
	return "", nil
}

func (carrierSettleHolder) CaptureInTx(_ context.Context, _ pgx.Tx, _ int64, _ string, _ int64, _ string) (string, error) {
	return "", nil
}

func (carrierSettleHolder) ReleaseInTx(_ context.Context, _ pgx.Tx, _ int64, _ string, _ int64, _ string) (string, error) {
	return "", nil
}

func TestCancelJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := workspacetasks.NewService(workspacetasks.ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	const taskID = "task_carrier_1"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on carrier error = %v", err)
	}
	if store.beginCalled {
		_ = tx.Rollback(ctx)
		t.Fatal("settlement opened its own transaction despite the staged carrier (nested tx)")
	}
	if store.cancelCalls != 1 || store.lastTxNil {
		_ = tx.Rollback(ctx)
		t.Fatalf("terminal flip calls = %d nil-tx = %v, want 1/false", store.cancelCalls, store.lastTxNil)
	}
	// Same-transaction visibility: the terminal write must be readable
	// from INSIDE the still-open carrier ...
	var inside int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("settlement rows visible inside carrier = %d, want 1", inside)
	}
	// ... and invisible outside it before the carrier commits.
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe on pool: %v", err)
	}
	if outside != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("settlement rows visible on pool before commit = %d, want 0", outside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		t.Fatalf("count probe after commit: %v", err)
	}
	if outside != 1 {
		t.Fatalf("settlement rows after carrier commit = %d, want 1", outside)
	}
	// The terminal event still stages under a borrowed transaction (the
	// caller flushes it after the commit it can see).
	if staged := settler.StagedEvents(); len(staged) != 1 {
		t.Fatalf("staged events = %d, want 1", len(staged))
	}
}

func TestCancelRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := workspacetasks.NewService(workspacetasks.ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	const taskID = "task_carrier_2"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on carrier error = %v", err)
	}
	// The business fails after settlement: the guard rolls the whole
	// carrier back, settlement write included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		t.Fatalf("count probe after rollback: %v", err)
	}
	if outside != 0 {
		t.Fatalf("settlement rows after carrier rollback = %d, want 0", outside)
	}
	_ = settler
}
