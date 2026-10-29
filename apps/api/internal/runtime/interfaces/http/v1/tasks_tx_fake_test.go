package v1

// In-memory single-transaction settlement harness for the v1 HTTP fixtures.
//
// It mirrors workspacetasks/settle_tx_fake_test.go: every seam the
// transactional settlement requires is declared explicitly; every write
// joins the same transaction:
//
//   - v1TxTaskStore wraps the task InMemoryStore with the
//     settlement-transaction seams. Task rows are written immediately
//     (same-tx read-your-write) and every durable in-memory write
//     registers its undo on the fake transaction, so a rejected submit
//     leaves no claim behind to poison its key.
//   - v1TxSubmissionAcceptance mirrors the production submit gate without
//     a database: Board-bound submits resolve the live posture onto the
//     tasking submit sentinels and accepted inputs ride the configured
//     AssetReader (missing assets fail as the submit path maps them; a
//     nil reader skips the asset check, never fails it).
//
// The fake quoter/holder/queue InTx legs live next to their recorders in
// tasks_test.go; the pool stores serve ListPoolInTx there too.

import (
	"context"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/db/dbutil/testscope"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// v1FakeTx aliases the shared settlement fake so the three hand-copied
// harnesses cannot drift again. Neighboring fixture files keep referring
// to v1FakeTx/v1OnRollback; these aliases keep that wiring compiling.
type v1FakeTx = testscope.Tx

// v1OnRollback registers an in-memory cleanup on the fake settlement
// transaction. It is a no-op for foreign transactions, which own their
// own commit/rollback.
func v1OnRollback(tx pgx.Tx, fn func()) {
	testscope.OnRollback(tx, fn)
}

// v1TxTaskStore wraps the task InMemoryStore with the
// settlement-transaction seams for database-free HTTP fixtures. Task rows
// are written immediately (same-tx read-your-write); every durable
// in-memory write registers its undo on the fake transaction, and terminal
// flips register their pre-image restore, so a rejected submit leaves no
// claim behind and a rolled-back settlement leaks no flipped task. The
// mechanics live in testscope; this type only keeps the fixture wiring.
type v1TxTaskStore struct {
	*workspacetasks.InMemoryStore
	fence testscope.Claims[workspacetasks.SubmissionClaim]
}

func newV1TxTaskStore(store *workspacetasks.InMemoryStore) *v1TxTaskStore {
	return &v1TxTaskStore{InMemoryStore: store}
}

func (s *v1TxTaskStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	scope, _ := testscope.BeginScope(ctx)
	return scope, nil
}

func (s *v1TxTaskStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	workspaceID, taskID := task.WorkspaceID, task.ID
	v1OnRollback(tx, func() {
		_ = s.DeleteTask(context.Background(), workspaceID, taskID)
	})
	return created, nil
}

func (s *v1TxTaskStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *v1TxTaskStore) CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	completed, err := s.CompleteTask(ctx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	restoreV1TaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return completed, nil
}

func (s *v1TxTaskStore) FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	failed, err := s.FailTask(ctx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	restoreV1TaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return failed, nil
}

func (s *v1TxTaskStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*workspacetasks.WorkspaceTask, bool, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	cancelled, already, err := s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
	if err != nil || already {
		return cancelled, already, err
	}
	restoreV1TaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return cancelled, already, nil
}

// restoreV1TaskOnRollback registers the pre-image of one task row on the
// fake transaction so a settlement rollback restores it instead of leaking
// a flipped task. The mechanics live in testscope; this only binds the
// store's restore helper. It is a no-op for foreign transactions, which
// own their own rollback.
func restoreV1TaskOnRollback(tx pgx.Tx, store *workspacetasks.InMemoryStore, workspaceID, taskID string, before *workspacetasks.WorkspaceTask) {
	if store == nil || before == nil {
		return
	}
	snapshot := *before
	testscope.OnRollback(tx, func() {
		store.RestoreTaskForTest(workspaceID, taskID, &snapshot)
	})
}

func (s *v1TxTaskStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*workspacetasks.WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

func (s *v1TxTaskStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim workspacetasks.SubmissionClaim) (*workspacetasks.SubmissionClaim, bool, error) {
	return s.fence.Claim(tx, testscope.ClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey), claim)
}

// v1TxSubmissionAcceptance mirrors the production submit gate without a
// database: Board-bound submits resolve the live posture onto the tasking
// submit sentinels; accepted inputs and the mask ride the configured
// AssetReader readiness gate.
type v1TxSubmissionAcceptance struct {
	boards workspacetasks.BoardAuthorizer
	assets workspacetasks.AssetReader
}

func (f *v1TxSubmissionAcceptance) AuthorizeSubmission(ctx context.Context, _ pgx.Tx, input workspacetasks.CreateInput) (workspacetasks.TaskBoardBinding, error) {
	if trimmed := strings.TrimSpace(input.BoardID); trimmed != "" {
		if f.boards == nil {
			return workspacetasks.TaskBoardBinding{}, workspacetasks.ErrSettlementUnavailable
		}
		access, err := f.boards.ResolveBoardAccess(ctx, strings.TrimSpace(input.WorkspaceID), trimmed, input.OwnerUserID, input.WorkspaceRole)
		if err != nil {
			return workspacetasks.TaskBoardBinding{}, err
		}
		var facts workspacetasks.BoardTaskFacts
		if access != nil {
			facts = workspacetasks.BoardTaskFacts{
				CanRead: access.CanRead, CanGenerate: access.CanGenerate,
				CanWrite: access.CanWrite, Archived: access.Archived,
			}
		}
		switch workspacetasks.BoardPolicy.Check(facts, workspacetasks.BoardOpSubmit) {
		case workspacetasks.BoardDecisionAllow:
			return workspacetasks.TaskBoardBinding{BoardID: trimmed, AccessVersion: access.AccessVersion}, nil
		case workspacetasks.BoardDecisionArchived:
			return workspacetasks.TaskBoardBinding{}, workspacetasks.ErrBoardArchived
		case workspacetasks.BoardDecisionForbidden:
			return workspacetasks.TaskBoardBinding{}, workspacetasks.ErrForbidden
		default:
			return workspacetasks.TaskBoardBinding{}, workspacetasks.ErrBoardNotFound
		}
	}
	if f.assets != nil {
		ids := append([]string(nil), input.MediaAssetIDs...)
		if maskID, ok := workspacetasks.MaskAssetIDFromParams(input.Params); ok {
			ids = append(ids, maskID)
		}
		for _, id := range ids {
			if _, err := f.assets.GetAsset(ctx, strings.TrimSpace(input.WorkspaceID), id); err != nil {
				return workspacetasks.TaskBoardBinding{}, err
			}
		}
	}
	return workspacetasks.TaskBoardBinding{}, nil
}

// v1TxSnapshotSaver captures the pinned snapshot for fixtures that assert
// pinning; the zero value is never used (atomic settlement fails closed on
// a nil saver).
func v1TxSnapshotSaver(saved map[string]routing.RoutingSnapshot) workspacetasks.RoutingSnapshotSaver {
	return func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		saved[taskID] = snap
		return nil
	}
}
