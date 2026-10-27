package workspacetasks

// In-memory single-transaction settlement harness.
//
// The board suite below runs Service.Create through createInTx (the
// single-PostgreSQL-transaction path): every seam createInTx requires is
// present here as an in-memory fake. Transaction capabilities are explicit;
// production still fails closed when a required dependency is nil. A nil seam
// still fails closed (ErrSettlementUnavailable), it is the TEST wiring
// that now supplies every seam.
//
// Rollback is faithful for the durable facts the tests touch (claim +
// task row + hold + enqueue): the fake transaction records compensating
// cleanups and runs them on Rollback, so a rejected submit leaves no
// claim behind to poison its key.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/generation/routing"

	"github.com/jackc/pgx/v5"
)

// fakeTx is a no-op settlement transaction: Commit/Rollback only record
// their outcome (plus deferred cleanups on rollback). All other pgx.Tx
// legs are never called by the in-memory fakes below.
type fakeTx struct {
	pgx.Tx
	committed  bool
	rolledBack bool
	onRollback []func()
}

func (t *fakeTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return nil
}

func fakeTxFrom(tx pgx.Tx) *fakeTx {
	fake, _ := tx.(*fakeTx)
	return fake
}

// restoreOnRollback registers the pre-image of one task row on the fake
// transaction so a settlement rollback restores it. Production rolls the
// state flip back when the settlement unit fails (Capture error,
// settlement write error); the immediate in-memory write above restores
// its pre-image on rollback so the fake keeps that atomicity instead of
// leaking a flipped task.
func restoreOnRollback(fake *fakeTx, store *InMemoryStore, workspaceID, taskID string, before *WorkspaceTask) {
	if fake == nil || store == nil || before == nil {
		return
	}
	snapshot := *before
	fake.onRollback = append(fake.onRollback, func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.tasks[taskKey(workspaceID, taskID)] = &snapshot
	})
}

// txTestStore wraps the InMemoryStore with the settlement-transaction
// seams. Task rows are written immediately (same-tx read-your-write), and
// every durable write registers its undo on the fake transaction so a
// rollback removes it again.
type txTestStore struct {
	*InMemoryStore
	mu     sync.Mutex
	claims map[string]*SubmissionClaim
	lastTx *fakeTx
}

func newTxTestStore() *txTestStore {
	return &txTestStore{InMemoryStore: NewInMemoryStoreForTests(), claims: make(map[string]*SubmissionClaim)}
}

func claimScopeKey(workspaceID, operation, key string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.TrimSpace(key)
}

func (s *txTestStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	tx := &fakeTx{}
	s.mu.Lock()
	s.lastTx = tx
	s.mu.Unlock()
	return dbutil.ScopeForTest(ctx, tx, true), nil
}

func (s *txTestStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) (*WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	if fake := fakeTxFrom(tx); fake != nil {
		workspaceID, taskID := task.WorkspaceID, task.ID
		fake.onRollback = append(fake.onRollback, func() {
			_ = s.DeleteTask(context.Background(), workspaceID, taskID)
		})
	}
	return created, nil
}

func (s *txTestStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *txTestStore) CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	completed, err := s.CompleteTask(ctx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	restoreOnRollback(fakeTxFrom(tx), s.InMemoryStore, workspaceID, taskID, before)
	return completed, nil
}

func (s *txTestStore) FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	failed, err := s.FailTask(ctx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	restoreOnRollback(fakeTxFrom(tx), s.InMemoryStore, workspaceID, taskID, before)
	return failed, nil
}

func (s *txTestStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	cancelled, already, err := s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
	if err != nil || already {
		return cancelled, already, err
	}
	restoreOnRollback(fakeTxFrom(tx), s.InMemoryStore, workspaceID, taskID, before)
	return cancelled, already, nil
}

func (s *txTestStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

// ResolveBillingAccountInTx serves the stable funds identity seam from a
// fixed fixture account: fake settlement spends the fixture account, never
// an owner wallet.
func (s *txTestStore) ResolveBillingAccountInTx(_ context.Context, _ pgx.Tx, _ string) (int64, error) {
	return 9101, nil
}

func (s *txTestStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim SubmissionClaim) (*SubmissionClaim, bool, error) {
	key := claimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.claims[key]; ok {
		dup := *existing
		return &dup, false, nil
	}
	stored := claim
	s.claims[key] = &stored
	if fake := fakeTxFrom(tx); fake != nil {
		fake.onRollback = append(fake.onRollback, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.claims, key)
		})
	}
	return nil, true, nil
}

func (s *txTestStore) claimCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.claims)
}

// txTestQuoter serves both quote seams from one snapshot.
type txTestQuoter struct {
	*fakeQuoter
	inTxCalls int
}

func (q *txTestQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, kind, resolution string, quantity int64) (BillingSnapshot, error) {
	q.inTxCalls++
	return q.Quote(ctx, kind, resolution, quantity)
}

// txTestHolder serves both hold seams from the fake ledger maps.
type txTestHolder struct {
	*fakeHolder
	inTxHolds int
}

func (h *txTestHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	h.inTxHolds++
	holdID, err := h.Hold(ctx, billingAccountID, actorUserID, amount, key, taskID)
	if err != nil {
		return "", err
	}
	if fake := fakeTxFrom(tx); fake != nil && holdID != "" {
		fake.onRollback = append(fake.onRollback, func() {
			delete(h.holds, holdID)
		})
	}
	return holdID, nil
}

func (h *txTestHolder) CaptureInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, holdID string, amount int64, _ string) (string, error) {
	h.captured[holdID] = amount
	return "", nil
}

func (h *txTestHolder) ReleaseInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, holdID string, amount int64, _ string) (string, error) {
	h.released[holdID] = amount
	return "", nil
}

// txTestEnqueuer serves both enqueue seams from one queue.
type txTestEnqueuer struct {
	*fakeEnqueuer
	inTxCalls int
}

func (q *txTestEnqueuer) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) error {
	q.inTxCalls++
	if err := q.Enqueue(ctx, task); err != nil {
		return err
	}
	if fake := fakeTxFrom(tx); fake != nil {
		fake.onRollback = append(fake.onRollback, func() {
			for i, enqueued := range q.enqueued {
				if enqueued == task {
					q.enqueued = append(q.enqueued[:i], q.enqueued[i+1:]...)
					return
				}
			}
		})
	}
	return nil
}

// allowAllTxPoolStore serves every service/resolution inside the
// settlement transaction too (ResolveInTx rejects a store without
// ListPoolInTx instead of falling back to a separate connection).
type allowAllTxPoolStore struct {
	allowAllPoolStore
}

func (allowAllTxPoolStore) ListPoolInTx(ctx context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllPoolStore{}.ListPool(ctx, service)
}

func allowTestPoolResolverInTx() *routing.RoutingResolver {
	seed := routing.SelectionSeed{1, 2, 3}
	return routing.NewRoutingResolver(allowAllTxPoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

// fakeSubmissionAcceptance mirrors the production submit gate on the
// settlement transaction: Board-bound submits resolve the live posture
// and map it onto the tasking submit sentinels; accepted inputs and the
// mask ride the same AssetReader readiness gate the submit path owns
// (a nil reader skips the asset check, never fails it); a nil authorizer
// fails closed with ErrSettlementUnavailable, never an unbound task.
type fakeSubmissionAcceptance struct {
	boards BoardAuthorizer
	assets AssetReader
}

func (f *fakeSubmissionAcceptance) AuthorizeSubmission(ctx context.Context, _ pgx.Tx, input CreateInput) (TaskBoardBinding, error) {
	var binding TaskBoardBinding
	if trimmed := strings.TrimSpace(input.BoardID); trimmed != "" {
		if f.boards == nil {
			return TaskBoardBinding{}, fmt.Errorf("workspacetasks: board authorizer unavailable: %w", ErrSettlementUnavailable)
		}
		access, err := f.boards.ResolveBoardAccess(ctx, strings.TrimSpace(input.WorkspaceID), trimmed, input.OwnerUserID, input.WorkspaceRole)
		if err != nil {
			return TaskBoardBinding{}, err
		}
		if err := checkBoardSubmit(access); err != nil {
			return TaskBoardBinding{}, err
		}
		binding = TaskBoardBinding{BoardID: trimmed, AccessVersion: access.AccessVersion}
	}
	if f.assets != nil {
		ids := append([]string(nil), input.MediaAssetIDs...)
		if maskID, ok := MaskAssetIDFromParams(input.Params); ok {
			ids = append(ids, maskID)
		}
		for i, id := range ids {
			if _, err := f.assets.GetAsset(ctx, strings.TrimSpace(input.WorkspaceID), id); err != nil {
				return TaskBoardBinding{}, fmt.Errorf("workspacetasks: input asset %d: %w", i, mapAssetGateError(err))
			}
		}
	}
	return binding, nil
}

type txBoardTestEnv struct {
	store   *InMemoryStore
	txStore *txTestStore
	quoter  *txTestQuoter
	holder  *txTestHolder
	queue   *txTestEnqueuer
	saved   map[string]routing.RoutingSnapshot
	service *Service
}

func newTxBoardTestEnv(boards BoardAuthorizer) *txBoardTestEnv {
	env := &txBoardTestEnv{
		txStore: newTxTestStore(),
		quoter:  &txTestQuoter{fakeQuoter: &fakeQuoter{snapshot: testSnapshot()}},
		holder:  &txTestHolder{fakeHolder: newFakeHolder()},
		queue:   &txTestEnqueuer{fakeEnqueuer: &fakeEnqueuer{}},
		saved:   make(map[string]routing.RoutingSnapshot),
	}
	env.store = env.txStore.InMemoryStore
	saved := env.saved
	env.service = NewService(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.txStore, SubmissionClaims: env.txStore, BillingAccounts: env.txStore,
		Quoter: env.quoter, Holder: env.holder, Queue: env.queue,
		Resolver: allowTestPoolResolverInTx(),
		SnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			saved[taskID] = snap
			return nil
		},
		Acceptance: &fakeSubmissionAcceptance{boards: boards},
	})
	return env
}

func boardTestServiceInTx(boards BoardAuthorizer) (*InMemoryStore, *Service) {
	env := newTxBoardTestEnv(boards)
	return env.store, env.service
}

// TestBoardSubmitWalksSingleTransactionSeams proves the board suite
// settles through createInTx: the Tx legs ran (not the compensating
// fallback), the durable claim committed with the task, and exactly one
// pending event staged.
func TestBoardSubmitWalksSingleTransactionSeams(t *testing.T) {
	ctx := context.Background()
	env := newTxBoardTestEnv(&fakeBoardAuthorizer{access: ownerBoardAccess()})
	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "tx-seam-walk-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if env.quoter.inTxCalls != 1 {
		t.Fatalf("QuoteInTx calls = %d, want 1 (settlement must walk the tx seam)", env.quoter.inTxCalls)
	}
	if env.holder.inTxHolds != 1 {
		t.Fatalf("HoldInTx calls = %d, want 1 (settlement must walk the tx seam)", env.holder.inTxHolds)
	}
	if env.queue.inTxCalls != 1 {
		t.Fatalf("EnqueueInTx calls = %d, want 1 (settlement must walk the tx seam)", env.queue.inTxCalls)
	}
	if got := env.txStore.claimCount(); got != 1 {
		t.Fatalf("durable claims = %d, want 1 (claim commits with the settlement)", got)
	}
	if env.txStore.lastTx == nil || !env.txStore.lastTx.committed || env.txStore.lastTx.rolledBack {
		t.Fatalf("settlement tx = %+v, want committed without rollback", env.txStore.lastTx)
	}
	if _, ok := env.saved[created.ID]; !ok {
		t.Fatal("routing snapshot was not pinned in the settlement transaction")
	}
	if created.BoardID != "" || created.BoardAccessVersion != 0 {
		t.Fatalf("boardless binding = (%q, %d), want empty", created.BoardID, created.BoardAccessVersion)
	}
	staged := env.service.StagedEvents()
	if len(staged) != 1 || staged[0].Status != StatusPending {
		t.Fatalf("staged events = %+v, want one pending", staged)
	}
}

// TestCreateInTxInvalidInputIsValidationNotAvailability pins the InTx
// routing-input mapping: a non-canonical resolution is a caller bug (422
// ValidationError on resolution), never the availability sentinel, and it
// reserves nothing, including no durable claim.
func TestCreateInTxInvalidInputIsValidationNotAvailability(t *testing.T) {
	ctx := context.Background()
	env := newTxBoardTestEnv(&fakeBoardAuthorizer{access: ownerBoardAccess()})
	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Resolution: "8k",
	}, "routing-intx-invalid-1")
	if !IsValidationError(err) {
		t.Fatalf("Create err = %T %v, want *ValidationError", err, err)
	}
	if errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err matched the availability sentinel; garbage input is a caller bug")
	}
	validationErr, _ := err.(*ValidationError)
	if validationErr.Fields["resolution"] == "" {
		t.Fatalf("Create fields = %v, want a resolution entry", validationErr.Fields)
	}
	count, _ := env.store.CountTasks(ctx, "ws_abc")
	if count != 0 || len(env.holder.holds) != 0 || len(env.queue.enqueued) != 0 {
		t.Fatalf("invalid input reserved work: tasks=%d holds=%v queued=%d", count, env.holder.holds, len(env.queue.enqueued))
	}
	if got := env.txStore.claimCount(); got != 0 {
		t.Fatalf("durable claims = %d, want 0 (rejected input must not fence its key)", got)
	}
	if env.txStore.lastTx == nil || !env.txStore.lastTx.rolledBack {
		t.Fatalf("settlement tx = %+v, want rolled back", env.txStore.lastTx)
	}
}
