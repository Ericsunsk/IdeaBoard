package v1

// In-memory single-transaction settlement harness for the v1 HTTP fixtures.
//
// It mirrors workspacetasks/settle_tx_fake_test.go: every seam the
// transactional settlement requires is present, so settlementSeams()
// reports handled=true and no compensating fallback can fire:
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
	"sync"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// v1FakeTx is a no-op settlement transaction: Commit/Rollback only record
// their outcome (plus deferred cleanups on rollback). All other pgx.Tx
// legs are never called by the in-memory fakes below.
type v1FakeTx struct {
	pgx.Tx
	committed  bool
	rolledBack bool
	onRollback []func()
}

func (t *v1FakeTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *v1FakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return nil
}

func v1OnRollback(tx pgx.Tx, fn func()) {
	if fake, ok := tx.(*v1FakeTx); ok {
		fake.onRollback = append(fake.onRollback, fn)
	}
}

func v1ClaimScopeKey(workspaceID, operation, key string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.TrimSpace(key)
}

// v1TxTaskStore wraps the task InMemoryStore with the
// settlement-transaction seams for database-free HTTP fixtures.
type v1TxTaskStore struct {
	*workspacetasks.InMemoryStore
	mu     sync.Mutex
	claims map[string]*workspacetasks.SubmissionClaim
}

func newV1TxTaskStore(store *workspacetasks.InMemoryStore) *v1TxTaskStore {
	return &v1TxTaskStore{InMemoryStore: store, claims: make(map[string]*workspacetasks.SubmissionClaim)}
}

func (s *v1TxTaskStore) BeginSettlementTx(context.Context) (pgx.Tx, error) {
	return &v1FakeTx{}, nil
}

func (s *v1TxTaskStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	if _, ok := tx.(*v1FakeTx); ok {
		workspaceID, taskID := task.WorkspaceID, task.ID
		v1OnRollback(tx, func() {
			_ = s.DeleteTask(context.Background(), workspaceID, taskID)
		})
	}
	return created, nil
}

func (s *v1TxTaskStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *v1TxTaskStore) CompleteTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	return s.CompleteTask(ctx, workspaceID, taskID, outputs)
}

func (s *v1TxTaskStore) FailTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error) {
	return s.FailTask(ctx, workspaceID, taskID, errorMsg)
}

func (s *v1TxTaskStore) CancelTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*workspacetasks.WorkspaceTask, bool, error) {
	return s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
}

func (s *v1TxTaskStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*workspacetasks.WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

func (s *v1TxTaskStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim workspacetasks.SubmissionClaim) (*workspacetasks.SubmissionClaim, bool, error) {
	key := v1ClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.claims[key]; ok {
		dup := *existing
		return &dup, false, nil
	}
	stored := claim
	s.claims[key] = &stored
	v1OnRollback(tx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.claims, key)
	})
	return nil, true, nil
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
