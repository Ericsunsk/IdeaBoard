package workspacetasks

// Free terminal settlement: free tasks (zero total, no hold) converge
// their settlement read model in the same terminal transaction instead of
// forking on the initial state.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

type freeSettlementCall struct {
	taskID, state                   string
	outputCount, captured, released int64
	captureTxID, releaseTxID        string
}

type fakeFreeTaskBillingStore struct {
	mu          sync.Mutex
	facts       int
	settlements []freeSettlementCall
}

func (f *fakeFreeTaskBillingStore) CreateBillingFactsInTx(context.Context, pgx.Tx, *WorkspaceTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.facts++
	return nil
}

func (f *fakeFreeTaskBillingStore) SetBillingSettlementInTx(_ context.Context, _ pgx.Tx, taskID, state string, outputCount, captured, released int64, captureTxID, releaseTxID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settlements = append(f.settlements, freeSettlementCall{
		taskID: taskID, state: state,
		outputCount: outputCount, captured: captured, released: released,
		captureTxID: captureTxID, releaseTxID: releaseTxID,
	})
	return nil
}

func (f *fakeFreeTaskBillingStore) last() (freeSettlementCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.settlements) == 0 {
		return freeSettlementCall{}, false
	}
	return f.settlements[len(f.settlements)-1], true
}

type freeTerminalEnv struct {
	store   *InMemoryStore
	txStore *txTestStore
	holder  *txTestHolder
	billing *fakeFreeTaskBillingStore
	service *Service
}

func newFreeTerminalEnv(totalCredits int64) *freeTerminalEnv {
	env := &freeTerminalEnv{
		txStore: newTxTestStore(),
		holder:  &txTestHolder{fakeHolder: newFakeHolder()},
		billing: &fakeFreeTaskBillingStore{},
	}
	env.store = env.txStore.InMemoryStore
	snapshot := testSnapshot()
	snapshot.UnitCredits = totalCredits
	snapshot.TotalCredits = totalCredits
	env.service = NewService(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.txStore, SubmissionClaims: env.txStore, BillingAccounts: env.txStore, TaskBilling: env.billing,
		Quoter: &txTestQuoter{fakeQuoter: &fakeQuoter{snapshot: snapshot}},
		Holder: env.holder, Queue: &txTestEnqueuer{fakeEnqueuer: &fakeEnqueuer{}},
		Resolver: allowTestPoolResolverInTx(), SnapshotSaver: noopSnapshotSaver,
		Acceptance: &fakeSubmissionAcceptance{},
	})
	return env
}

func freeRemoteOutput() []OutputRef {
	return []OutputRef{{
		Kind: OutputKindRemote, URL: "https://cdn.example.com/free-o.png",
		ChecksumSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		ResultIndex:    0,
	}}
}

func TestFreeTaskCompleteWritesCapturedSettlement(t *testing.T) {
	ctx := context.Background()
	env := newFreeTerminalEnv(0)
	created, err := env.service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "free-complete-1")
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	if created.BillingSnapshot.SettlementState != "free" {
		t.Fatalf("settlement = %q, want free", created.BillingSnapshot.SettlementState)
	}
	if len(env.holder.holds) != 0 {
		t.Fatalf("free Create held %d times, want no hold", len(env.holder.holds))
	}
	completed, err := env.service.Complete(ctx, "ws_abc", created.ID, freeRemoteOutput())
	if err != nil {
		t.Fatalf("free Complete: %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if completed.BillingSnapshot.SettlementState != "captured" {
		t.Fatalf("settlement = %q, want captured", completed.BillingSnapshot.SettlementState)
	}
	call, ok := env.billing.last()
	if !ok {
		t.Fatal("free Complete wrote no settlement row")
	}
	if call.taskID != created.ID || call.state != "captured" || call.outputCount != 1 ||
		call.captured != 0 || call.released != 0 || call.captureTxID != "" || call.releaseTxID != "" {
		t.Fatalf("settlement call = %+v, want captured(1,0,0,\"\")", call)
	}
	if len(env.holder.captured) != 0 || len(env.holder.released) != 0 {
		t.Fatal("free Complete must not touch the ledger")
	}
}

func TestFreeTaskFailWritesReleasedSettlement(t *testing.T) {
	ctx := context.Background()
	env := newFreeTerminalEnv(0)
	created, err := env.service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "free-fail-1")
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	failed, err := env.service.Fail(ctx, "ws_abc", created.ID, "provider exploded")
	if err != nil {
		t.Fatalf("free Fail: %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", failed.Status)
	}
	if failed.BillingSnapshot.SettlementState != "released" {
		t.Fatalf("settlement = %q, want released", failed.BillingSnapshot.SettlementState)
	}
	call, ok := env.billing.last()
	if !ok {
		t.Fatal("free Fail wrote no settlement row")
	}
	if call.taskID != created.ID || call.state != "released" || call.outputCount != 0 ||
		call.captured != 0 || call.released != 0 || call.captureTxID != "" || call.releaseTxID != "" {
		t.Fatalf("settlement call = %+v, want released(0,0,0,\"\")", call)
	}
	if len(env.holder.captured) != 0 || len(env.holder.released) != 0 {
		t.Fatal("free Fail must not touch the ledger")
	}
}

func TestFreeTaskCancelWritesReleasedSettlement(t *testing.T) {
	ctx := context.Background()
	env := newFreeTerminalEnv(0)
	created, err := env.service.Create(ctx, CreateInput{WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render"}, "free-cancel-1")
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	cancelled, already, err := env.service.Cancel(ctx, "ws_abc", created.ID, 7, "user cancel")
	if err != nil || already {
		t.Fatalf("free Cancel = %v %v %v", cancelled, already, err)
	}
	if cancelled.BillingSnapshot.SettlementState != "released" {
		t.Fatalf("settlement = %q, want released", cancelled.BillingSnapshot.SettlementState)
	}
	call, ok := env.billing.last()
	if !ok {
		t.Fatal("free Cancel wrote no settlement row")
	}
	if call.taskID != created.ID || call.state != "released" || call.outputCount != 0 ||
		call.captured != 0 || call.released != 0 {
		t.Fatalf("settlement call = %+v, want released(0,0,0)", call)
	}
}

// TestTerminalSettlementWithoutHoldFailsClosed proves a positive total
// without a hold (a torn durable fact) fails the terminal write instead
// of completing with no settlement: the task stays non-terminal and
// operator-visible.
func TestTerminalSettlementWithoutHoldFailsClosed(t *testing.T) {
	ctx := context.Background()
	env := newFreeTerminalEnv(10)
	stored, err := env.store.CreateTask(ctx, &WorkspaceTask{
		ID: "task_torn", WorkspaceID: "ws_abc", OwnerUserID: 7,
		Service: "render", Status: StatusRunning, BillingSnapshot: BillingSnapshot{
			BillingAccountID: 9101, TotalCredits: 10, UnitCredits: 10, Quantity: 1,
			SettlementState: "held",
		},
	})
	if err != nil {
		t.Fatalf("seed torn task: %v", err)
	}
	if _, err := env.service.Complete(ctx, "ws_abc", stored.ID, freeRemoteOutput()); err == nil {
		t.Fatal("Complete without hold must fail closed")
	} else if !strings.Contains(err.Error(), "without hold") {
		t.Fatalf("Complete err = %v, want terminal-without-hold", err)
	}
	current, err := env.store.GetTask(ctx, "ws_abc", stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.IsTerminal() {
		t.Fatalf("torn task status = %q, want non-terminal", current.Status)
	}
	if _, ok := env.billing.last(); ok {
		t.Fatal("torn task must not write a settlement row")
	}
}
