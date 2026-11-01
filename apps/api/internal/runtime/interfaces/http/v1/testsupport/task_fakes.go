package testsupport

import (
	"context"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/db/dbutil/testscope"
	"ideaboard/api/internal/generation/routing"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tenancy/funds"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"

	"github.com/jackc/pgx/v5"
)

type FakeTaskQuoter struct {
	UnitCredits int64
	Calls       int
	Err         error
}

func (f *FakeTaskQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (taskdomain.BillingSnapshot, error) {
	f.Calls++
	if f.Err != nil {
		return taskdomain.BillingSnapshot{}, f.Err
	}
	if quantity <= 0 {
		quantity = 1
	}
	unit := f.UnitCredits
	if unit == 0 {
		unit = 10
	}
	return taskdomain.BillingSnapshot{
		SKU: "ai-image:" + kind, DisplayName: kind, PriceVersionID: 1, PriceVersion: 1,
		OutputResolution: resolution, UnitCredits: unit, Quantity: quantity,
		TotalCredits: unit * quantity, QuotedAt: time.Now().UTC(), SettlementState: "held",
	}, nil
}

func (f *FakeTaskQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, kind, resolution string, quantity int64) (taskdomain.BillingSnapshot, error) {
	return f.Quote(ctx, kind, resolution, quantity)
}

func AllowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{4, 5, 6}
	return routing.NewRoutingResolver(allowAllV1PoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllV1PoolStore struct{}

func (allowAllV1PoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Provider: "openrouter", Model: "test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (allowAllV1PoolStore) ListPoolInTx(_ context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllV1PoolStore{}.ListPool(context.Background(), service)
}

type FakeBillingAccounts struct{}

func (FakeBillingAccounts) ResolveBillingAccountInTx(_ context.Context, _ pgx.Tx, _ string) (int64, error) {
	return 7001, nil
}

func (FakeBillingAccounts) ResolveFunds(_ context.Context, workspacePublicID string) (funds.Identity, error) {
	return funds.Identity{
		WorkspaceInternalID:    7001,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       7001,
		BillingAccountPublicID: "ba_quote0000001",
		OwnerType:              tenancypg.BillingOwnerWorkspace,
	}, nil
}

type FakeQuoteWallets struct {
	mu           sync.Mutex
	Available    int64
	Reserved     int64
	Restrictions []string
	Err          error
}

func NewFakeQuoteWallets() *FakeQuoteWallets {
	return &FakeQuoteWallets{Available: 1000000}
}

func (f *FakeQuoteWallets) GetWallet(_ context.Context, _ int64) (domain.WalletDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return domain.WalletDetails{}, f.Err
	}
	details := domain.WalletDetails{Status: domain.WalletStatusActive}
	details.Available = f.Available
	details.Reserved = f.Reserved
	for _, restrictionType := range f.Restrictions {
		details.ActiveRestrictions = append(details.ActiveRestrictions, domain.WalletRestriction{Type: restrictionType})
	}
	return details, nil
}

type FakeTaskHolder struct {
	Holds    map[string]int64
	Captured map[string]int64
	Released map[string]int64
	HoldErr  error
}

func NewFakeTaskHolder() *FakeTaskHolder {
	return &FakeTaskHolder{Holds: map[string]int64{}, Captured: map[string]int64{}, Released: map[string]int64{}}
}

func (f *FakeTaskHolder) Hold(ctx context.Context, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	if f.HoldErr != nil {
		return "", f.HoldErr
	}
	holdID := "hold_" + taskID
	f.Holds[holdID] = amount
	return holdID, nil
}

func (f *FakeTaskHolder) Capture(ctx context.Context, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) error {
	f.Captured[holdID] = amount
	return nil
}

func (f *FakeTaskHolder) Release(ctx context.Context, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) error {
	f.Released[holdID] = amount
	return nil
}

func (f *FakeTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	holdID, err := f.Hold(ctx, billingAccountID, actorUserID, amount, key, taskID)
	if err != nil {
		return "", err
	}
	if holdID != "" {
		testscope.OnRollback(tx, func() {
			delete(f.Holds, holdID)
		})
	}
	return holdID, nil
}

func (f *FakeTaskHolder) CaptureInTx(ctx context.Context, _ pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Capture(ctx, billingAccountID, actorUserID, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
}

func (f *FakeTaskHolder) ReleaseInTx(ctx context.Context, _ pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Release(ctx, billingAccountID, actorUserID, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
}

type FakeTaskQueue struct {
	Enqueued []*taskdomain.WorkspaceTask
	FailNext error
}

func (f *FakeTaskQueue) Enqueue(ctx context.Context, task *taskdomain.WorkspaceTask) error {
	if f.FailNext != nil {
		err := f.FailNext
		f.FailNext = nil
		return err
	}
	f.Enqueued = append(f.Enqueued, task)
	return nil
}

func (f *FakeTaskQueue) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *taskdomain.WorkspaceTask) error {
	if err := f.Enqueue(ctx, task); err != nil {
		return err
	}
	testscope.OnRollback(tx, func() {
		for i, enqueued := range f.Enqueued {
			if enqueued == task {
				f.Enqueued = append(f.Enqueued[:i], f.Enqueued[i+1:]...)
				return
			}
		}
	})
	return nil
}

type FakeTaskAssets struct {
	Assets map[string]*taskdomain.AssetInfo
}

func NewFakeTaskAssets() *FakeTaskAssets {
	return &FakeTaskAssets{Assets: make(map[string]*taskdomain.AssetInfo)}
}

func (f *FakeTaskAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*taskdomain.AssetInfo, error) {
	info, ok := f.Assets[workspaceID+"\x00"+assetID]
	if !ok {
		return nil, taskdomain.ErrMediaNotFound
	}
	return info, nil
}

type TxTaskStore struct {
	*taskmemory.InMemoryStore
	fence testscope.Claims[taskdomain.SubmissionClaim]
}

func NewTxTaskStore(store *taskmemory.InMemoryStore) *TxTaskStore {
	return &TxTaskStore{InMemoryStore: store}
}

func (s *TxTaskStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	scope, _ := testscope.BeginScope(ctx)
	return scope, nil
}

func (s *TxTaskStore) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *taskdomain.WorkspaceTask) (*taskdomain.WorkspaceTask, error) {
	created, err := s.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	workspaceID, taskID := task.WorkspaceID, task.ID
	testscope.OnRollback(tx, func() {
		_ = s.DeleteTask(context.Background(), workspaceID, taskID)
	})
	return created, nil
}

func (s *TxTaskStore) GetTaskInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID string) (*taskdomain.WorkspaceTask, error) {
	return s.GetTask(ctx, workspaceID, taskID)
}

func (s *TxTaskStore) CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []taskdomain.OutputRef) (*taskdomain.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	completed, err := s.CompleteTask(ctx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	restoreTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return completed, nil
}

func (s *TxTaskStore) FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*taskdomain.WorkspaceTask, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	failed, err := s.FailTask(ctx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	restoreTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return failed, nil
}

func (s *TxTaskStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*taskdomain.WorkspaceTask, bool, error) {
	before, _ := s.GetTask(ctx, workspaceID, taskID)
	cancelled, already, err := s.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
	if err != nil || already {
		return cancelled, already, err
	}
	restoreTaskOnRollback(tx, s.InMemoryStore, workspaceID, taskID, before)
	return cancelled, already, nil
}

func restoreTaskOnRollback(tx pgx.Tx, store *taskmemory.InMemoryStore, workspaceID, taskID string, before *taskdomain.WorkspaceTask) {
	if store == nil || before == nil {
		return
	}
	snapshot := *before
	testscope.OnRollback(tx, func() {
		store.RestoreTaskForTest(workspaceID, taskID, &snapshot)
	})
}

func (s *TxTaskStore) TransitionForRetryInTx(ctx context.Context, _ pgx.Tx, workspaceID, taskID, newToken string) (*taskdomain.WorkspaceTask, error) {
	return s.TransitionForRetry(ctx, workspaceID, taskID, newToken)
}

func (s *TxTaskStore) ClaimSubmissionInTx(_ context.Context, tx pgx.Tx, claim taskdomain.SubmissionClaim) (*taskdomain.SubmissionClaim, bool, error) {
	return s.fence.Claim(tx, testscope.ClaimScopeKey(claim.WorkspaceID, claim.Operation, claim.IdempotencyKey), claim)
}

type TxSubmissionAcceptance struct {
	Boards taskports.BoardAuthorizer
	Assets taskports.AssetReader
}

func (f *TxSubmissionAcceptance) AuthorizeSubmission(ctx context.Context, _ pgx.Tx, input taskdomain.CreateInput) (taskdomain.TaskBoardBinding, error) {
	if trimmed := strings.TrimSpace(input.BoardID); trimmed != "" {
		if f.Boards == nil {
			return taskdomain.TaskBoardBinding{}, taskdomain.ErrSettlementUnavailable
		}
		access, err := f.Boards.ResolveBoardAccess(ctx, strings.TrimSpace(input.WorkspaceID), trimmed, input.OwnerUserID, input.WorkspaceRole)
		if err != nil {
			return taskdomain.TaskBoardBinding{}, err
		}
		var facts taskdomain.BoardTaskFacts
		if access != nil {
			facts = taskdomain.BoardTaskFacts{
				CanRead: access.CanRead, CanGenerate: access.CanGenerate,
				CanWrite: access.CanWrite, Archived: access.Archived,
			}
		}
		switch taskdomain.BoardPolicy.Check(facts, taskdomain.BoardOpSubmit) {
		case taskdomain.BoardDecisionAllow:
			return taskdomain.TaskBoardBinding{BoardID: trimmed, AccessVersion: access.AccessVersion}, nil
		case taskdomain.BoardDecisionArchived:
			return taskdomain.TaskBoardBinding{}, taskdomain.ErrBoardArchived
		case taskdomain.BoardDecisionForbidden:
			return taskdomain.TaskBoardBinding{}, taskdomain.ErrForbidden
		default:
			return taskdomain.TaskBoardBinding{}, taskdomain.ErrBoardNotFound
		}
	}
	if f.Assets != nil {
		ids := append([]string(nil), input.MediaAssetIDs...)
		if maskID, ok := taskdomain.MaskAssetIDFromParams(input.Params); ok {
			ids = append(ids, maskID)
		}
		for _, id := range ids {
			if _, err := f.Assets.GetAsset(ctx, strings.TrimSpace(input.WorkspaceID), id); err != nil {
				return taskdomain.TaskBoardBinding{}, err
			}
		}
	}
	return taskdomain.TaskBoardBinding{}, nil
}

func TxSnapshotSaver(saved map[string]routing.RoutingSnapshot) taskports.RoutingSnapshotSaver {
	return func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		saved[taskID] = snap
		return nil
	}
}
