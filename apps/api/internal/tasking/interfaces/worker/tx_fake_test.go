package worker

// In-memory single-transaction settlement harness for the worker loop tests.
//
// It mirrors workspacetasks/settle_tx_fake_test.go: every seam the
// transactional settlement requires is present, so settlementSeams()
// reports handled=true and no compensating fallback can fire:
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
	"sync"

	"ideaboard/api/internal/generation/routing"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// workerFakeTx is a no-op settlement transaction: Commit/Rollback only
// record their outcome (plus deferred cleanups on rollback). All other
// pgx.Tx legs are never called by the in-memory fakes below.
type workerFakeTx struct {
	pgx.Tx
	committed  bool
	rolledBack bool
	onRollback []func()
}

func (t *workerFakeTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *workerFakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return nil
}

// workerShimTx wraps a real postgres settlement transaction so the
// database-outside fake legs (in-memory claim fence, fake holder maps,
// fake queue slice) can register rollback undos on it. Task rows need no
// undo: the real rollback discards them.
type workerShimTx struct {
	pgx.Tx
	onRollback []func()
}

func (t *workerShimTx) Rollback(ctx context.Context) error {
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return t.Tx.Rollback(ctx)
}

// workerOnRollback registers an in-memory cleanup on either transaction
// flavor. It is a no-op for foreign transactions (e.g. ambient carriers),
// which own their own commit/rollback.
func workerOnRollback(tx pgx.Tx, fn func()) {
	switch t := tx.(type) {
	case *workerFakeTx:
		t.onRollback = append(t.onRollback, fn)
	case *workerShimTx:
		t.onRollback = append(t.onRollback, fn)
	}
}

func workerClaimScopeKey(workspaceID, operation, key string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.TrimSpace(key)
}

// txWorkerStore wraps the merged claim+attempt adapter with the
// settlement-transaction seams for database-free worker loop tests.
type txWorkerStore struct {
	*attemptClaimStore
	mu     sync.Mutex
	claims map[string]*workspacetasks.SubmissionClaim
	lastTx *workerFakeTx
}

func newTxWorkerStore(store *attemptClaimStore) *txWorkerStore {
	return &txWorkerStore{attemptClaimStore: store, claims: make(map[string]*workspacetasks.SubmissionClaim)}
}

func (s *txWorkerStore) BeginSettlementTx(context.Context) (pgx.Tx, error) {
	tx := &workerFakeTx{}
	s.mu.Lock()
	s.lastTx = tx
	s.mu.Unlock()
	return tx, nil
}

func (s *txWorkerStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	if _, ok := tx.(*workerFakeTx); ok {
		workspaceID, taskID := task.WorkspaceID, task.ID
		workerOnRollback(tx, func() {
			_ = s.DeleteTask(context.Background(), workspaceID, taskID)
		})
	}
	return created, nil
}

func (s *txWorkerStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *txWorkerStore) CompleteTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	return s.CompleteTask(ctx, workspaceID, taskID, outputs)
}

func (s *txWorkerStore) FailTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error) {
	return s.FailTask(ctx, workspaceID, taskID, errorMsg)
}

func (s *txWorkerStore) CancelTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*workspacetasks.WorkspaceTask, bool, error) {
	return s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
}

func (s *txWorkerStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*workspacetasks.WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

func (s *txWorkerStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim workspacetasks.SubmissionClaim) (*workspacetasks.SubmissionClaim, bool, error) {
	key := workerClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.claims[key]; ok {
		dup := *existing
		return &dup, false, nil
	}
	stored := claim
	s.claims[key] = &stored
	workerOnRollback(tx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.claims, key)
	})
	return nil, true, nil
}

// txPGServiceStore wraps the real postgres task store for the fixture-DB
// worker twins. Task state transitions ride the real transaction; only the
// durable submit claim is fenced in memory (the fixture schema carries no
// claim table).
type txPGServiceStore struct {
	*taskpostgres.Store
	mu     sync.Mutex
	claims map[string]*workspacetasks.SubmissionClaim
}

func newTxPGServiceStore(store *taskpostgres.Store) *txPGServiceStore {
	return &txPGServiceStore{Store: store, claims: make(map[string]*workspacetasks.SubmissionClaim)}
}

func (s *txPGServiceStore) BeginSettlementTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.Store.BeginSettlementTx(ctx)
	if err != nil {
		return nil, err
	}
	return &workerShimTx{Tx: tx}, nil
}

func (s *txPGServiceStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim workspacetasks.SubmissionClaim) (*workspacetasks.SubmissionClaim, bool, error) {
	key := workerClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.claims[key]; ok {
		dup := *existing
		return &dup, false, nil
	}
	stored := claim
	s.claims[key] = &stored
	workerOnRollback(tx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.claims, key)
	})
	return nil, true, nil
}

// workerTestAcceptance mirrors the production submit gate without a
// database: Board-bound submits resolve the live posture onto the tasking
// submit sentinels and accepted inputs ride the AssetReader readiness
// gate. A nil authorizer fails closed on bound submits, never an unbound
// task.
type workerTestAcceptance struct {
	boards workspacetasks.BoardAuthorizer
	assets workspacetasks.AssetReader
}

func (f *workerTestAcceptance) AuthorizeSubmission(ctx context.Context, _ pgx.Tx, input workspacetasks.CreateInput) (workspacetasks.TaskBoardBinding, error) {
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

// workerNoopSnapshotSaver satisfies the atomic pin requirement for suites
// that persist the snapshot explicitly (pinWorkerSnapshot).
func workerNoopSnapshotSaver(context.Context, pgx.Tx, string, routing.RoutingSnapshot) error {
	return nil
}

// newTxWorkerService wires one in-memory settlement through createInTx for
// the database-free worker loop tests.
func newTxWorkerService(txStore *txWorkerStore, holder *workerTestHolder, queue *workerTestQueue, assets workspacetasks.AssetReader, boards workspacetasks.BoardAuthorizer) *workspacetasks.Service {
	return workspacetasks.NewService(workspacetasks.ServiceDeps{
		TaskRead: txStore, TaskLifecycle: txStore,
		Quoter: workerTestQuoter{}, Holder: holder, Queue: queue,
		Assets: assets, Resolver: allowTestPoolResolver(),
		SnapshotSaver: workerNoopSnapshotSaver,
		Acceptance:    &workerTestAcceptance{boards: boards, assets: assets},
	})
}
