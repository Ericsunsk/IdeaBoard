package composition

// Adapter tx-join tests (#826, Phase 4b-02 of #809): the workspace task
// enqueuer forwards the ambient settlement transaction to River untouched.
// No DB: a stub River client captures the call shape.

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type stubWorkspaceRiverClient struct {
	insertArgs   river.JobArgs
	insertTxArgs river.JobArgs
	insertTx     pgx.Tx
	insertErr    error
	insertTxErr  error
}

func (s *stubWorkspaceRiverClient) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	s.insertArgs = args
	if s.insertErr != nil {
		return nil, s.insertErr
	}
	return &rivertype.JobInsertResult{}, nil
}

func (s *stubWorkspaceRiverClient) InsertTx(_ context.Context, tx pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	s.insertTxArgs = args
	s.insertTx = tx
	if s.insertTxErr != nil {
		return nil, s.insertTxErr
	}
	return &rivertype.JobInsertResult{}, nil
}

func TestWorkspaceTaskEnqueuerForwardsAmbientTx(t *testing.T) {
	ctx := context.Background()
	stub := &stubWorkspaceRiverClient{}
	enqueuer := newWorkspaceTaskEnqueuer(stub, "drawing_default", 25)
	task := &workspacetasks.WorkspaceTask{
		ID: "task_txforward1", WorkspaceID: "ws_txforward1",
		WorkflowKind: "render", ExecutionToken: "exec_txforward1",
	}

	if err := enqueuer.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if stub.insertArgs == nil {
		t.Fatal("Enqueue must call client.Insert")
	}
	if stub.insertTxArgs != nil {
		t.Fatal("Enqueue must not touch InsertTx")
	}
	if _, ok := stub.insertArgs.(WorkspaceTaskArgs); !ok {
		t.Fatalf("enqueued args = %T, want WorkspaceTaskArgs", stub.insertArgs)
	}
}

func TestWorkspaceTaskEnqueueInTxJoinsRiverTx(t *testing.T) {
	ctx := context.Background()
	stub := &stubWorkspaceRiverClient{}
	enqueuer := newWorkspaceTaskEnqueuer(stub, "drawing_default", 25)
	task := &workspacetasks.WorkspaceTask{
		ID: "task_txjoin1", WorkspaceID: "ws_txjoin1",
		WorkflowKind: "render", ExecutionToken: "exec_txjoin1",
	}

	var tx pgx.Tx
	if err := enqueuer.EnqueueInTx(ctx, tx, task); err == nil {
		t.Fatal("EnqueueInTx with nil tx must fail closed")
	}
	if stub.insertTxArgs != nil {
		t.Fatal("nil-tx EnqueueInTx must not reach River")
	}

	stub.insertTxErr = errors.New("river down")
	if err := enqueuer.EnqueueInTx(ctx, tx, task); err == nil {
		t.Fatal("EnqueueInTx must surface River errors")
	}
}

// TestWorkspaceTaskEnqueueInTxForwardsAmbientTx proves the settlement tx
// object itself reaches River.InsertTx untouched (fixture DB for a real
// pgx.Tx; the stub records it without touching river_job).
func TestWorkspaceTaskEnqueueInTxForwardsAmbientTx(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	stub := &stubWorkspaceRiverClient{}
	enqueuer := newWorkspaceTaskEnqueuer(stub, "drawing_default", 25)
	task := &workspacetasks.WorkspaceTask{
		ID: "task_txfwd1", WorkspaceID: env.wsA,
		WorkflowKind: "render", ExecutionToken: "exec_txfwd1",
	}

	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := enqueuer.EnqueueInTx(ctx, tx, task); err != nil {
		t.Fatalf("EnqueueInTx: %v", err)
	}
	if stub.insertTx == nil {
		t.Fatal("EnqueueInTx must forward a tx to River.InsertTx")
	}
	args, ok := stub.insertTxArgs.(WorkspaceTaskArgs)
	if !ok {
		t.Fatalf("forwarded args = %T, want WorkspaceTaskArgs", stub.insertTxArgs)
	}
	if args.TaskID != task.ID || args.WorkspaceID != task.WorkspaceID || args.ExecutionToken != task.ExecutionToken {
		t.Fatalf("forwarded args = %+v, want task identity", args)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe tx: %v", err)
	}
}

func TestWorkspaceTaskEnqueueInTxFailsClosedWithoutClient(t *testing.T) {
	ctx := context.Background()
	enqueuer := newWorkspaceTaskEnqueuer(nil, "drawing_default", 25)
	task := &workspacetasks.WorkspaceTask{ID: "task_txjoin2", WorkspaceID: "ws_txjoin2"}

	var tx pgx.Tx
	if err := enqueuer.EnqueueInTx(ctx, tx, task); !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
		t.Fatalf("clientless EnqueueInTx = %v, want quote unavailable", err)
	}
	if err := enqueuer.Enqueue(ctx, task); !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
		t.Fatalf("clientless Enqueue = %v, want quote unavailable", err)
	}
}
