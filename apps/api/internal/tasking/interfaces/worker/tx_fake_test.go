package worker

// In-memory single-transaction settlement harness for the worker loop tests.
//
// It mirrors workspacetasks/settle_tx_test.go: every seam the
// transactional settlement requires is declared explicitly; every write
// joins the same transaction:
//
//   - txWorkerStore wraps the merged claim+attempt adapter with the
//     settlement-transaction seams. Task rows are written immediately
//     (same-tx read-your-write) and every durable in-memory write
//     registers its undo on the fake transaction, so a rejected submit
//     leaves no claim behind to poison its key.
//   - txPGServiceStore wraps the real postgres task store for the
//     fixture-DB twins. Task rows ride the real transaction; the durable
//     submit claim and the fake holder/queue legs are fenced in memory
//     with rollback undos registered on the shim transaction, because
//     the fixture schema carries no claim table and the fake legs live
//     outside the database transaction. (Production proves the
//     claim+ledger+enqueue rollback together in workspacetasks/settle_test.go
//     over the real credits service.)
//   - workerTestAcceptance mirrors the production submit gate: Board-bound
//     submits resolve the live posture onto the tasking submit sentinels,
//     accepted inputs and the mask ride the AssetReader readiness gate
//     (a nil reader skips the asset check, never fails it).
//
// workerNoopSnapshotSaver satisfies the atomic pin requirement for suites
// that overwrite the snapshot explicitly (pinWorkerSnapshot); the pinning
// itself is proven in workspacetasks/settle_test.go.

import (
	"context"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/db/dbutil/testscope"
	"ideaboard/api/internal/generation/routing"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	tasks "ideaboard/api/internal/tasking/domain"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	taskports "ideaboard/api/internal/tasking/ports/tasks"

	"github.com/jackc/pgx/v5"
)

// workerFakeTx aliases the shared settlement fake so the three hand-copied
// harnesses cannot drift again. Neighboring suite files keep referring to
// workerFakeTx; this alias keeps that wiring compiling.
type workerFakeTx = testscope.Tx

// workerShimTx aliases the shared shim: it wraps a real postgres
// settlement transaction so the database-outside fake legs (in-memory
// claim fence, fake holder maps, fake queue slice) can register rollback
// undos on it. Task rows need no undo: the real rollback discards them.
type workerShimTx = testscope.ShimTx

// workerOnRollback registers an in-memory cleanup on either transaction
// flavor. It is a no-op for foreign transactions (e.g. ambient carriers),
// which own their own commit/rollback.
func workerOnRollback(tx pgx.Tx, fn func()) {
	testscope.OnRollback(tx, fn)
}

// txWorkerStore wraps the merged claim+attempt adapter with the
// settlement-transaction seams for database-free worker loop tests.
type txWorkerStore struct {
	*attemptClaimStore
	fence  testscope.Claims[tasks.SubmissionClaim]
	lastTx *workerFakeTx
}

func newTxWorkerStore(store *attemptClaimStore) *txWorkerStore {
	return &txWorkerStore{attemptClaimStore: store}
}

func (s *txWorkerStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	scope, tx := testscope.BeginScope(ctx)
	s.lastTx = tx
	return scope, nil
}

func (s *txWorkerStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	workspaceID, taskID := task.WorkspaceID, task.ID
	workerOnRollback(tx, func() {
		_ = s.DeleteTask(context.Background(), workspaceID, taskID)
	})
	return created, nil
}

func (s *txWorkerStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *txWorkerStore) CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	completed, err := s.CompleteTask(ctx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	restoreWorkerTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return completed, nil
}

func (s *txWorkerStore) FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	failed, err := s.FailTask(ctx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	restoreWorkerTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return failed, nil
}

func (s *txWorkerStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	cancelled, already, err := s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
	if err != nil || already {
		return cancelled, already, err
	}
	restoreWorkerTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return cancelled, already, nil
}

// restoreWorkerTaskOnRollback registers the pre-image of one task row on
// the fake transaction so a settlement rollback restores it instead of
// leaking a flipped task. The mechanics live in testscope; this only
// binds the store's restore helper. It is a no-op for foreign
// transactions, which own their own rollback.
func restoreWorkerTaskOnRollback(tx pgx.Tx, store *taskmemory.InMemoryStore, workspaceID, taskID string, before *tasks.WorkspaceTask) {
	if store == nil || before == nil {
		return
	}
	snapshot := *before
	testscope.OnRollback(tx, func() {
		store.RestoreTaskForTest(workspaceID, taskID, &snapshot)
	})
}

func (s *txWorkerStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

func (s *txWorkerStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim tasks.SubmissionClaim) (*tasks.SubmissionClaim, bool, error) {
	return s.fence.Claim(tx, testscope.ClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey), claim)
}

// txPGServiceStore wraps the real postgres task store for the fixture-DB
// worker twins. Task state transitions ride the real transaction; only the
// durable submit claim is fenced in memory (the fixture schema carries no
// claim table).
type txPGServiceStore struct {
	*taskpostgres.Store
	fence testscope.Claims[tasks.SubmissionClaim]
}

func newTxPGServiceStore(store *taskpostgres.Store) *txPGServiceStore {
	return &txPGServiceStore{Store: store}
}

func (s *txPGServiceStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	scope, err := s.Store.BeginSettlementScope(ctx)
	if err != nil {
		return nil, err
	}
	return dbutil.ScopeForTest(ctx, testscope.WrapShim(scope.Tx()), true), nil
}

func (s *txPGServiceStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim tasks.SubmissionClaim) (*tasks.SubmissionClaim, bool, error) {
	return s.fence.Claim(tx, testscope.ClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey), claim)
}

// workerTestAcceptance mirrors the production submit gate without a
// database: Board-bound submits resolve the live posture onto the tasking
// submit sentinels and accepted inputs ride the AssetReader readiness
// gate. A nil authorizer fails closed on bound submits, never an unbound
// task.
type workerTestAcceptance struct {
	boards taskports.BoardAuthorizer
	assets taskports.AssetReader
}

func (f *workerTestAcceptance) AuthorizeSubmission(ctx context.Context, _ pgx.Tx, input tasks.CreateInput) (tasks.TaskBoardBinding, error) {
	if trimmed := strings.TrimSpace(input.BoardID); trimmed != "" {
		if f.boards == nil {
			return tasks.TaskBoardBinding{}, tasks.ErrSettlementUnavailable
		}
		access, err := f.boards.ResolveBoardAccess(ctx, strings.TrimSpace(input.WorkspaceID), trimmed, input.OwnerUserID, input.WorkspaceRole)
		if err != nil {
			return tasks.TaskBoardBinding{}, err
		}
		var facts tasks.BoardTaskFacts
		if access != nil {
			facts = tasks.BoardTaskFacts{
				CanRead: access.CanRead, CanGenerate: access.CanGenerate,
				CanWrite: access.CanWrite, Archived: access.Archived,
			}
		}
		switch tasks.BoardPolicy.Check(facts, tasks.BoardOpSubmit) {
		case tasks.BoardDecisionAllow:
			return tasks.TaskBoardBinding{BoardID: trimmed, AccessVersion: access.AccessVersion}, nil
		case tasks.BoardDecisionArchived:
			return tasks.TaskBoardBinding{}, tasks.ErrBoardArchived
		case tasks.BoardDecisionForbidden:
			return tasks.TaskBoardBinding{}, tasks.ErrForbidden
		default:
			return tasks.TaskBoardBinding{}, tasks.ErrBoardNotFound
		}
	}
	if f.assets != nil {
		ids := append([]string(nil), input.MediaAssetIDs...)
		if maskID, ok := tasks.MaskAssetIDFromParams(input.Params); ok {
			ids = append(ids, maskID)
		}
		for _, id := range ids {
			if _, err := f.assets.GetAsset(ctx, strings.TrimSpace(input.WorkspaceID), id); err != nil {
				return tasks.TaskBoardBinding{}, err
			}
		}
	}
	return tasks.TaskBoardBinding{}, nil
}

// workerNoopSnapshotSaver satisfies the atomic pin requirement for suites
// that persist the snapshot explicitly (pinWorkerSnapshot).
func workerNoopSnapshotSaver(context.Context, pgx.Tx, string, routing.RoutingSnapshot) error {
	return nil
}

// workerTestBillingAccounts serves the stable funds-identity seam from a
// fixed fixture account: in-memory settlement spends the fixture account,
// never an owner wallet.
type workerTestBillingAccounts struct{}

func (workerTestBillingAccounts) ResolveBillingAccountInTx(_ context.Context, _ pgx.Tx, _ string) (int64, error) {
	return 7001, nil
}

// newTxWorkerService wires one in-memory settlement through createInTx for
// the database-free worker loop tests.
func newTxWorkerService(txStore *txWorkerStore, holder *workerTestHolder, queue *workerTestQueue, assets taskports.AssetReader, boards taskports.BoardAuthorizer) *taskapp.Service {
	return taskapp.NewService(taskapp.ServiceDeps{
		TaskRead: txStore, TaskLifecycle: txStore, SubmissionClaims: txStore,
		Quoter: workerTestQuoter{}, Holder: holder, Queue: queue,
		Assets: assets, Resolver: allowTestPoolResolver(),
		SnapshotSaver:   workerNoopSnapshotSaver,
		Acceptance:      &workerTestAcceptance{boards: boards, assets: assets},
		BillingAccounts: workerTestBillingAccounts{},
	})
}
