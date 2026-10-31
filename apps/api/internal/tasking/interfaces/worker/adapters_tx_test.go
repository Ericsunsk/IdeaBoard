package worker

// Adapter tx-join tests (#826, Phase 4b-02 of #809): the workspace task
// enqueuer forwards the ambient settlement transaction to River untouched.
// No DB: a stub River client captures the call shape.

import (
	"context"
	"errors"
	"testing"

	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type stubWorkspaceRiverClient struct {
	insertTxArgs river.JobArgs
	insertTx     pgx.Tx
	insertTxErr  error
}

func (s *stubWorkspaceRiverClient) InsertTx(_ context.Context, tx pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	s.insertTxArgs = args
	s.insertTx = tx
	if s.insertTxErr != nil {
		return nil, s.insertTxErr
	}
	return &rivertype.JobInsertResult{}, nil
}

func TestWorkspaceTaskEnqueueInTxJoinsRiverTx(t *testing.T) {
	ctx := context.Background()
	stub := &stubWorkspaceRiverClient{}
	enqueuer := taskadapters.NewWorkspaceTaskEnqueuer(stub, "drawing_default", 25)
	task := &tasks.WorkspaceTask{
		ID: "task_txjoin1", WorkspaceID: "ws_txjoin1",
		Service: "render", ExecutionToken: "exec_txjoin1",
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
	enqueuer := taskadapters.NewWorkspaceTaskEnqueuer(stub, "drawing_default", 25)
	task := &tasks.WorkspaceTask{
		ID: "task_txfwd1", WorkspaceID: env.wsA,
		Service: "render", ExecutionToken: "exec_txfwd1",
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
	args, ok := stub.insertTxArgs.(taskqueue.WorkspaceTaskArgs)
	if !ok {
		t.Fatalf("forwarded args = %T, want taskqueue.WorkspaceTaskArgs", stub.insertTxArgs)
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
	enqueuer := taskadapters.NewWorkspaceTaskEnqueuer(nil, "drawing_default", 25)
	task := &tasks.WorkspaceTask{ID: "task_txjoin2", WorkspaceID: "ws_txjoin2"}

	var tx pgx.Tx
	if err := enqueuer.EnqueueInTx(ctx, tx, task); !errors.Is(err, tasks.ErrQuoteUnavailable) {
		t.Fatalf("clientless EnqueueInTx = %v, want quote unavailable", err)
	}

}
