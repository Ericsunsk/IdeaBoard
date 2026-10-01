// Package workspacetasks_test proves the single-transaction settlement
// boundary (#826, Phase 4b-02 of #809) over the canonical fixture database.
//
// What is proven here (all legs in ONE pgx transaction opened on the task
// store pool):
//   - Create: Quote snapshot + Credit Hold + task insert + enqueue commit
//     atomically; an enqueue failure rolls everything back (no task row,
//     no hold ledger row, no enqueue probe row).
//   - Complete: state flip + outputs persist + Capture (+ remainder
//     Release) commit atomically; a Capture failure leaves the task
//     unchanged with the hold intact and no capture artifacts.
//   - Fail/Cancel: state flip + Release commit atomically; a Release
//     failure leaves the task unchanged.
//   - A failed task write (unknown id) leaves no billing artifacts
//     (orphans query over credit_transactions).
//
// The enqueue leg joins the business tx through TxEnqueuer: the probe
// INSERT below rides the same tx (rolled back on failure). The production
// River join (workspaceTaskEnqueuer.EnqueueInTx -> client.InsertTx) is
// proven at the adapter level in composition (mock RiverTxClient asserts
// the ambient tx is forwarded); the fixture snapshot carries no river_job
// table, so no real River insert runs here.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).
package workspacetasks_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// allowTestPoolResolver serves every service/resolution from one enabled
// entry. Settlement tests run against real PG for the tx legs; the routing
// decision itself needs no database.
func allowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{9, 9, 9}
	return routing.NewRoutingResolver(allowAllAtomicPoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllAtomicPoolStore struct{}

func (allowAllAtomicPoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Model: "openrouter/test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (allowAllAtomicPoolStore) ListPoolInTx(ctx context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllAtomicPoolStore{}.ListPool(ctx, service)
}

const atomicTestChecksum = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func randomAtomicHex(t *testing.T, nbytes int) string {
	t.Helper()
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random hex failed: %v", err)
	}
	return hex.EncodeToString(buf)
}

// atomicQuoter adapts the real Billing catalog to both settlement seams:
// Quote for the compensating fallback, QuoteInTx for the single-tx path.
type atomicQuoter struct {
	catalog *billingapplication.BillableItemCatalog
}

func (q *atomicQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	quote, err := q.catalog.QuoteAtResolution(ctx, "ai-image:"+kind, billing.OutputResolution(resolution), quantity)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, err
	}
	return mapAtomicQuote(quote), nil
}

func (q *atomicQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, kind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	quote, err := q.catalog.QuoteAtResolutionInTx(ctx, tx, "ai-image:"+kind, billing.OutputResolution(resolution), quantity)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, err
	}
	return mapAtomicQuote(quote), nil
}

func mapAtomicQuote(quote billing.Quote) workspacetasks.BillingSnapshot {
	return workspacetasks.BillingSnapshot{
		SKU: string(quote.SKU), DisplayName: quote.DisplayName,
		PriceVersionID: quote.PriceVersionID, PriceVersion: quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits, Quantity: quote.Quantity, TotalCredits: quote.TotalCredits,
		QuotedAt: quote.QuotedAt, SettlementState: "quoted",
	}
}

func atomicMetadata(key, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:            "task_submission",
		Reason:            "AI 生图任务 Idea 预授权",
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}

// atomicHolder adapts the real Credits service to both settlement seams.
// failCapture/failRelease inject billing-side failures to prove the task
// flip rolls back with them.
type atomicHolder struct {
	credits     *credits.Service
	mu          sync.Mutex
	failHold    error
	failCapture error
	failRelease error
}

func (h *atomicHolder) setFailures(hold, capture, release error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failHold, h.failCapture, h.failRelease = hold, capture, release
}

func (h *atomicHolder) holdFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failHold
}

func (h *atomicHolder) captureFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failCapture
}

func (h *atomicHolder) releaseFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failRelease
}

func (h *atomicHolder) Hold(ctx context.Context, userID int64, amount int64, key, taskID string) (string, error) {
	if err := h.holdFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.Hold(ctx, ports.HoldCommand{UserID: userID, Amount: creditAmount, Metadata: atomicMetadata(key, taskID, userID)})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *atomicHolder) Capture(ctx context.Context, holdID string, amount int64, taskID string) error {
	return errors.New("atomicHolder.Capture must not run on the single-tx path")
}

func (h *atomicHolder) Release(ctx context.Context, holdID string, amount int64, taskID string) error {
	return errors.New("atomicHolder.Release must not run on the single-tx path")
}

func (h *atomicHolder) HoldInTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, key, taskID string) (string, error) {
	if err := h.holdFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.InTx(tx).Hold(ctx, ports.HoldCommand{UserID: userID, Amount: creditAmount, Metadata: atomicMetadata(key, taskID, userID)})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *atomicHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, userID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := h.captureFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.InTx(tx).Capture(ctx, ports.CaptureCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: atomicMetadata("task-capture:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *atomicHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := h.releaseFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: atomicMetadata("task-release:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// atomicQueue proves the enqueue leg joins the business transaction: the
// probe INSERT rides the same pgx.Tx, so a rollback removes it. The
// production River join is covered in composition (InsertTx delegation).
type atomicQueue struct {
	mu       sync.Mutex
	pool     *pgxpool.Pool
	failNext error
	calls    int
}

func (q *atomicQueue) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	return errors.New("atomicQueue.Enqueue must not run on the single-tx path")
}

func (q *atomicQueue) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) error {
	q.mu.Lock()
	fail := q.failNext
	q.failNext = nil
	q.calls++
	q.mu.Unlock()
	if fail != nil {
		return fail
	}
	if tx == nil {
		return errors.New("atomicQueue: settlement transaction is nil")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settle_enqueue_probe (task_id) VALUES ($1) ON CONFLICT (task_id) DO NOTHING`, strings.TrimSpace(task.ID)); err != nil {
		return fmt.Errorf("atomicQueue: probe enqueue: %w", err)
	}
	return nil
}

func (q *atomicQueue) failOnce(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failNext = err
}

type atomicEnv struct {
	pool     *pgxpool.Pool
	store    *taskpostgres.Store
	credits  *credits.Service
	holder   *atomicHolder
	queue    *atomicQueue
	service  *workspacetasks.Service
	ws       string
	userID   int64
	unitCost int64
}

func openAtomicEnv(t *testing.T, unitCost int64) *atomicEnv {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)

	if _, err := pool.Exec(ctx, `CREATE TABLE settle_enqueue_probe (task_id TEXT PRIMARY KEY, enqueued_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create enqueue probe table: %v", err)
	}

	storageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('atomic-settle-user', $1, 'user', 1) RETURNING id`,
		storageUID,
	).Scan(&userID); err != nil {
		t.Fatalf("seed atomic settle user: %v", err)
	}
	ws := "ws_atomic" + randomAtomicHex(t, 6)
	if _, err := pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'atomic-settle-ws')`,
		ws, userID,
	); err != nil {
		t.Fatalf("seed atomic settle workspace: %v", err)
	}

	creditService := credits.NewService(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(ctx, userID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision atomic settle wallet: %v", err)
	}
	grantAmount, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := creditService.InTx(tx).Grant(ctx, ports.GrantCommand{UserID: userID, Amount: grantAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "atomic-settle-seed-" + randomAtomicHex(t, 4),
		Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "atomic settlement seed",
		BusinessReference: domain.BusinessReference{Type: "test", ID: "atomic-settle-seed"},
	}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant atomic settle funds: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	billingStore := billingpostgres.NewStore(pool)
	definition := billing.BillableItemDefinition{
		SKU: "ai-image:render", Namespace: "ai-image", ProductKind: "workflow-feature",
		DisplayName: "Render", PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	billingCatalog := billingapplication.NewBillableItemCatalog(registry, billingStore)
	if _, err := billingStore.PublishPrice(ctx, billing.PublishPriceInput{Definition: definition, UnitCredits: unitCost, ActorID: userID}); err != nil {
		t.Fatalf("publish atomic settle price: %v", err)
	}

	store := taskpostgres.NewStore(pool)
	holder := &atomicHolder{credits: creditService}
	queue := &atomicQueue{pool: pool}
	snapshotSaver := func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		if tx == nil {
			return errors.New("routing snapshot saver requires the settlement transaction")
		}
		return catalog.SaveRoutingSnapshot(ctx, dbsqlc.New(tx), taskID, snap)
	}
	service := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: store, TaskLifecycle: store, TaskBilling: store, Quoter: &atomicQuoter{catalog: billingCatalog}, Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(), SnapshotSaver: snapshotSaver,
	})
	return &atomicEnv{pool: pool, store: store, credits: creditService, holder: holder, queue: queue, service: service, ws: ws, userID: userID, unitCost: unitCost}
}

func (e *atomicEnv) wallet(t *testing.T) (available, reserved int64) {
	t.Helper()
	details, err := e.credits.GetWallet(context.Background(), e.userID)
	if err != nil {
		t.Fatalf("read atomic settle wallet: %v", err)
	}
	return details.Available, details.Reserved
}

func (e *atomicEnv) ledgerCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM credit_transactions WHERE `+where, args...).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}

func (e *atomicEnv) probeCount(t *testing.T, taskID string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM settle_enqueue_probe WHERE task_id = $1`, taskID).Scan(&count); err != nil {
		t.Fatalf("count enqueue probe rows: %v", err)
	}
	return count
}

func (e *atomicEnv) createPaidTask(t *testing.T, quantity int64) *workspacetasks.WorkspaceTask {
	t.Helper()
	created, err := e.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: e.ws, OwnerUserID: e.userID,
		Service: "render", Quantity: quantity, Resolution: "1k",
	}, "atomic-key-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("atomic Create: %v", err)
	}
	_ = e.service.StagedEvents()
	return created
}

func (e *atomicEnv) claimTask(t *testing.T, taskID, token string) {
	t.Helper()
	if _, err := e.store.ClaimTask(context.Background(), e.ws, taskID, token); err != nil {
		t.Fatalf("atomic ClaimTask: %v", err)
	}
}

func atomicRemoteOutput() []workspacetasks.OutputRef {
	return []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindRemote, URL: "https://cdn.example.com/atomic-o.png", ChecksumSHA256: atomicTestChecksum, ResultIndex: 0},
	}
}

// TestAtomicCreateSettlesHoldInsertEnqueue proves submission commits Quote
// snapshot + Hold + task insert + enqueue atomically: the task row, the
// hold ledger row, the reserved balance, and the enqueue probe all exist.
func TestAtomicCreateSettlesHoldInsertEnqueue(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)

	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask after atomic Create: %v", err)
	}
	if stored.Status != workspacetasks.StatusPending {
		t.Fatalf("status = %q, want pending", stored.Status)
	}
	if stored.BillingSnapshot.HoldTransactionID == "" || stored.BillingSnapshot.SettlementState != "held" {
		t.Fatalf("snapshot = %+v, want held with hold id", stored.BillingSnapshot)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-hold:"+created.ID) != 1 {
		t.Fatal("hold ledger row missing after atomic Create")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10)", available, reserved)
	}
	if env.probeCount(t, created.ID) != 1 {
		t.Fatal("enqueue probe missing: enqueue did not join the settlement tx")
	}
}

// TestAtomicCreateRollsBackOnEnqueueFailure proves an enqueue failure rolls
// the whole submission back: no task row, no hold ledger row, no reserved
// balance, no probe row, no staged events.
func TestAtomicCreateRollsBackOnEnqueueFailure(t *testing.T) {
	env := openAtomicEnv(t, 10)
	env.queue.failOnce(errors.New("river unavailable"))

	_, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-key-"+randomAtomicHex(t, 4))
	if err == nil {
		t.Fatal("Create with failing enqueue must fail")
	}
	count, err := env.store.CountTasks(context.Background(), env.ws)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks = %d, want 0 (insert rolled back)", count)
	}
	if env.ledgerCount(t, `wallet_user_id = $1 AND command_type = 'hold'`, env.userID) != 0 {
		t.Fatal("hold ledger row leaked after rolled-back Create")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0) (hold rolled back)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed settlement")
	}
}

// TestAtomicCompleteCaptures proves terminal Complete commits the state
// flip + outputs + Capture (+ remainder Release) atomically, with partial
// capture math (quantity 2, one output -> capture 10, release 10).
func TestAtomicCompleteCaptures(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 2)
	env.claimTask(t, created.ID, created.ExecutionToken)

	completed, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("atomic Complete: %v", err)
	}
	if completed.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if len(completed.OutputRefs) != 1 {
		t.Fatalf("outputs = %+v, want 1 persisted ref", completed.OutputRefs)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-capture:"+created.ID) != 1 {
		t.Fatal("capture ledger row missing after atomic Complete")
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("remainder release ledger row missing after partial capture")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (990, 0) (captured 10, released 10)", available, reserved)
	}
	events := env.service.StagedEvents()
	if len(events) != 1 || events[0].Status != workspacetasks.StatusCompleted {
		t.Fatalf("staged events = %+v, want one completed", events)
	}
}

// TestAtomicCompleteBillingFailureLeavesTaskUnchanged proves billing-fails
// -> task-unchanged: a Capture failure rolls the state flip and the output
// writes back, keeping the hold intact with no capture artifacts.
func TestAtomicCompleteBillingFailureLeavesTaskUnchanged(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	env.holder.setFailures(nil, errors.New("ledger unavailable"), nil)

	_, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err == nil {
		t.Fatal("Complete with failing Capture must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != workspacetasks.StatusRunning {
		t.Fatalf("status = %q, want running (flip rolled back)", stored.Status)
	}
	if len(stored.OutputRefs) != 0 {
		t.Fatalf("outputs = %+v, want none (output writes rolled back)", stored.OutputRefs)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-capture:"+created.ID) != 0 {
		t.Fatal("capture ledger row leaked after rolled-back Complete")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10) (hold intact)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed terminal settlement")
	}
}

// TestAtomicFailReleases proves Fail commits the state flip + Release
// atomically with the full hold returned to available.
func TestAtomicFailReleases(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	failed, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded")
	if err != nil {
		t.Fatalf("atomic Fail: %v", err)
	}
	if failed.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", failed.Status)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("release ledger row missing after atomic Fail")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0) (fully released)", available, reserved)
	}
}

// TestAtomicFailBillingFailureLeavesTaskUnchanged proves a Release failure
// rolls the Fail flip back: the task keeps running with the hold intact.
func TestAtomicFailBillingFailureLeavesTaskUnchanged(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	env.holder.setFailures(nil, nil, errors.New("ledger unavailable"))

	_, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded")
	if err == nil {
		t.Fatal("Fail with failing Release must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != workspacetasks.StatusRunning {
		t.Fatalf("status = %q, want running (flip rolled back)", stored.Status)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10) (hold intact)", available, reserved)
	}
}

// TestAtomicCancelReleases proves Cancel commits the state flip + Release
// atomically and replays idempotently without a second Release.
func TestAtomicCancelReleases(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)

	cancelled, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "no longer needed")
	if err != nil || already {
		t.Fatalf("Cancel = (%v, %v, %v), want fresh cancel", cancelled, already, err)
	}
	if cancelled.Status != workspacetasks.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", cancelled.Status)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("release ledger row missing after atomic Cancel")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
	replay, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "again")
	if err != nil || !already || replay.Status != workspacetasks.StatusCancelled {
		t.Fatalf("replay = (%v, %v, %v), want idempotent cancelled", replay, already, err)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("cancel replay must not write a second Release")
	}
}

// TestAtomicTaskFailureWritesNoBillingArtifacts proves task-fails -> no
// billing artifacts: completing an unknown task writes no capture/release
// rows (orphans query over the task business reference).
func TestAtomicTaskFailureWritesNoBillingArtifacts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ghostID := "task_ghost" + randomAtomicHex(t, 6)

	_, err := env.service.Complete(context.Background(), env.ws, ghostID, atomicRemoteOutput())
	if err == nil {
		t.Fatal("Complete of an unknown task must fail")
	}
	if count := env.ledgerCount(t, `business_reference_type = 'task' AND business_reference_id = $1`, ghostID); count != 0 {
		t.Fatalf("billing artifacts = %d for failed task %s, want 0", count, ghostID)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want untouched (1000, 0)", available, reserved)
	}
}

// TestAtomicRetryRequeuesInTx proves Retry consumes the budget entry and
// requeues atomically; a requeue failure keeps the running task untouched.
func TestAtomicRetryRequeuesInTx(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	retried, err := env.service.Retry(context.Background(), env.ws, created.ID, true)
	if err != nil {
		t.Fatalf("atomic Retry: %v", err)
	}
	if retried.Status != workspacetasks.StatusPending || retried.RetryCount != 1 {
		t.Fatalf("retried = %+v, want pending retry_count=1", retried)
	}
	if env.probeCount(t, created.ID) != 1 {
		t.Fatal("requeue probe missing: requeue did not join the settlement tx")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want hold untouched (990, 10)", available, reserved)
	}

	env.queue.failOnce(errors.New("river unavailable"))
	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err == nil {
		t.Fatal("Retry with failing requeue must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1 (budget bump rolled back)", stored.RetryCount)
	}
}

// TestAtomicFreeTaskSettlesWithoutLedger proves zero-price tasks flow
// through the same transaction with no Hold/Capture/Release rows.
func TestAtomicFreeTaskSettlesWithoutLedger(t *testing.T) {
	env := openAtomicEnv(t, 0)
	created, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-free-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	_ = env.service.StagedEvents()
	if created.BillingSnapshot.SettlementState != "free" {
		t.Fatalf("settlement = %q, want free", created.BillingSnapshot.SettlementState)
	}
	env.claimTask(t, created.ID, created.ExecutionToken)
	completed, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("free Complete: %v", err)
	}
	if completed.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if count := env.ledgerCount(t, `wallet_user_id = $1`, env.userID); count != 1 {
		t.Fatalf("ledger rows = %d, want only the seed grant (no hold/capture/release)", count)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want untouched (1000, 0)", available, reserved)
	}
}

// Submit-path routing snapshot tests over the fixture database.
// The resolver-wired service below shares openAtomicEnv's store, holder,
// queue, wallet, and billing price; only the pool, resolver, and snapshot
// saver are added, so the existing atomic tests continue to prove the
// settlement transaction independently.

// openRoutingAtomicEnv rebuilds the env service with the shared
// RoutingResolver over a seeded render pool plus the production snapshot
// saver (catalog.SaveRoutingSnapshot with the settlement tx's Queries).
// The billing catalog is rebuilt over the same pool; the ai-image:render
// 1k price published by openAtomicEnv persists in the database.
func openRoutingAtomicEnv(t *testing.T) *atomicEnv {
	t.Helper()
	env := openAtomicEnv(t, 10)
	ctx := context.Background()

	poolRepo := catalog.NewRepository(env.pool)
	if _, err := poolRepo.ReplacePool(ctx, "render", 0, []catalog.PoolEntryInput{
		{ModelID: "atomic-test/model", Enabled: true, Resolutions: []string{"1k"}},
	}); err != nil {
		t.Fatalf("seed atomic routing pool: %v", err)
	}
	poolResolver := routing.NewRoutingResolver(poolRepo)
	poolSaver := func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		if tx == nil {
			return errors.New("routing snapshot saver requires the settlement tx")
		}
		return catalog.SaveRoutingSnapshot(ctx, dbsqlc.New(tx), taskID, snap)
	}

	billingStore := billingpostgres.NewStore(env.pool)
	definition := billing.BillableItemDefinition{
		SKU: "ai-image:render", Namespace: "ai-image", ProductKind: "workflow-feature",
		DisplayName: "Render", PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	env.service = workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.store, TaskBilling: env.store, Quoter: &atomicQuoter{catalog: billingapplication.NewBillableItemCatalog(registry, billingStore)},
		Holder: env.holder, Queue: env.queue,
		Resolver: poolResolver, SnapshotSaver: poolSaver,
	})
	return env
}

func loadAtomicSnapshot(t *testing.T, env *atomicEnv, taskID string) routing.RoutingSnapshot {
	t.Helper()
	snap, err := catalog.LoadRoutingSnapshot(context.Background(), dbsqlc.New(env.pool), taskID)
	if err != nil {
		t.Fatalf("LoadRoutingSnapshot(%s): %v", taskID, err)
	}
	return snap
}

// TestAtomicRoutingSnapshotPinnedInTx proves submit resolves via the shared
// RoutingResolver and pins the snapshot atomically with Hold/task/enqueue:
// the stored fact replays without the live pool.
func TestAtomicRoutingSnapshotPinnedInTx(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	created := env.createPaidTask(t, 1)

	snap := loadAtomicSnapshot(t, env, created.ID)
	if err := snap.Verify(); err != nil {
		t.Fatalf("pinned snapshot must replay without the live pool: %v", err)
	}
	if snap.Service != "render" || snap.RequestedResolution != routing.Resolution1K || snap.PoolVersion != 1 {
		t.Fatalf("pinned pinning = %+v, want render/1k/version 1", snap)
	}
	if len(snap.EligibleModels) != 1 || snap.EligibleModels[0] != "atomic-test/model" {
		t.Fatalf("eligible = %v, want [atomic-test/model]", snap.EligibleModels)
	}
	if snap.SelectedModel != "atomic-test/model" || snap.HasRetryCandidate() {
		t.Fatalf("selected = %q retry = %q, want the single candidate with no retry", snap.SelectedModel, snap.RetryCandidate)
	}
}

// TestAtomicRoutingUnavailableReservesNothing proves an empty eligible set
// converges to the canonical sentinel before Hold, task insert, or enqueue:
// no task row, no hold ledger row, no reserved balance, no enqueue probe.
func TestAtomicRoutingUnavailableReservesNothing(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	ctx := context.Background()
	poolRepo := catalog.NewRepository(env.pool)
	version, err := poolRepo.GetVersion(ctx, "render")
	if err != nil {
		t.Fatalf("pool version: %v", err)
	}
	if _, err := poolRepo.ReplacePool(ctx, "render", version, nil); err != nil {
		t.Fatalf("clear atomic routing pool: %v", err)
	}

	_, err = env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-routing-unavail-"+randomAtomicHex(t, 4))
	if !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err = %v, want ErrServiceResolutionUnavailable", err)
	}
	var unavailable *routing.ResolutionUnavailableError
	if !errors.As(err, &unavailable) || unavailable.Service != "render" || unavailable.Resolution != routing.Resolution1K {
		t.Fatalf("Create fields = %+v, want normalized render/1k", unavailable)
	}
	count, err := env.store.CountTasks(ctx, env.ws)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if env.ledgerCount(t, `wallet_user_id = $1 AND command_type = 'hold'`, env.userID) != 0 {
		t.Fatal("hold ledger row leaked after unavailable Create")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want untouched (1000, 0)", available, reserved)
	}
	var probes int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM settle_enqueue_probe`).Scan(&probes); err != nil {
		t.Fatalf("count enqueue probes: %v", err)
	}
	if probes != 0 {
		t.Fatalf("enqueue probes = %d, want 0", probes)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on unavailable Create")
	}
}

// TestAtomicRoutingSnapshotSurvivesPoolEdits proves later Admin pool edits
// never invalidate an accepted task nor change its candidate set: the
// stored snapshot still replays byte-identical without the live pool.
func TestAtomicRoutingSnapshotSurvivesPoolEdits(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	ctx := context.Background()
	created := env.createPaidTask(t, 1)
	before := loadAtomicSnapshot(t, env, created.ID)

	poolRepo := catalog.NewRepository(env.pool)
	version, err := poolRepo.GetVersion(ctx, "render")
	if err != nil {
		t.Fatalf("pool version: %v", err)
	}
	if _, err := poolRepo.ReplacePool(ctx, "render", version, []catalog.PoolEntryInput{
		{ModelID: "atomic-test/other", Enabled: true, Resolutions: []string{"2k", "4k"}},
	}); err != nil {
		t.Fatalf("edit atomic routing pool: %v", err)
	}

	after := loadAtomicSnapshot(t, env, created.ID)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("stored snapshot changed by the pool edit:\n%+v\n%+v", before, after)
	}
	if err := after.Verify(); err != nil {
		t.Fatalf("Verify() after pool edit = %v; replay must not need the live pool", err)
	}
}
